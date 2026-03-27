package adv_service

import (
	"log"
	"math"
	"time"
)

// SlotDuration задаёт длительность одного временного слота.
// Все расчёты оставшихся слотов и целей слота опираются на эту величину.
// В данном случае слот равен 5 минутам.
const SlotDuration = 5 * time.Minute

// Campaign представляет рекламную кампанию со всеми необходимыми данными
// для управления откруткой, равномерностью и участием в аукционе.
type Campaign struct {
	// ID — уникальный идентификатор кампании
	ID string

	// BasePriceCPM — базовая цена за тысячу показов (CPM).
	// Используется для сортировки кампаний по убыванию цены.
	BasePriceCPM float64

	// EvennessBySlotMode — флаг равномерного режима:
	// true — кампания должна тратить бюджет равномерно по слотам,
	//        после выполнения нормы текущего слота она выключается до следующего.
	// false — кампания участвует всегда, пока активна, без ограничений по слотам.
	EvennessBySlotMode bool

	// GoalTotalDollars — общий бюджет кампании в долларах.
	GoalTotalDollars float64

	// CumDoneDollars — сколько долларов уже потрачено кампанией с начала её работы.
	// Это глобальный счётчик, обновляется при каждом показе.
	CumDoneDollars float64

	// SlotDoneDollars — сколько долларов потрачено в текущем слоте.
	// Обнуляется в начале каждого нового слота.
	SlotDoneDollars float64

	// StartTS — время начала кампании.
	StartTS time.Time
	// EndTS — время окончания кампании.
	EndTS time.Time

	// TargetingCheck — функция, которая проверяет, подходит ли запрос под условия кампании
	// (гео, устройство, ОС и т.д.). Если функция не задана (nil), то кампания считается
	// подходящей для любого запроса.
	TargetingCheck func(attrs map[string]interface{}) bool
}

// IsActiveGlobal проверяет, активна ли кампания в глобальном смысле:
// - время сейчас находится в интервале [StartTS, EndTS];
// - общий потраченный бюджет ещё не достиг цели.
func (c *Campaign) IsActiveGlobal(now time.Time) bool {
	if now.Before(c.StartTS) || now.After(c.EndTS) {
		return false
	}
	if c.CumDoneDollars >= c.GoalTotalDollars {
		return false
	}
	return true
}

// SlotsLeft возвращает количество полных слотов, оставшихся до окончания кампании.
// Оставшееся время округляется вверх (math.Ceil), чтобы даже остаток времени,
// меньший длительности слота, считался за один слот.
func (c *Campaign) SlotsLeft(now time.Time) int {
	if now.After(c.EndTS) {
		return 0
	}
	remaining := c.EndTS.Sub(now)
	slots := int(math.Ceil(float64(remaining) / float64(SlotDuration)))
	return slots
}

// SlotTarget вычисляет целевую сумму, которую кампания должна потратить в текущем слоте,
// чтобы к концу своего периода уложиться в бюджет с учётом уже потраченных средств.
// Формула: (оставшийся бюджет) / (количество оставшихся слотов).
// Если кампания уже выполнила план или слотов не осталось, возвращает 0.
func (c *Campaign) SlotTarget(now time.Time) float64 {
	remaining := c.GoalTotalDollars - c.CumDoneDollars
	if remaining <= 0 {
		return 0
	}
	slotsLeft := c.SlotsLeft(now)
	if slotsLeft == 0 {
		return 0
	}
	return remaining / float64(slotsLeft)
}

// ShouldParticipateInSlot определяет, может ли кампания участвовать в текущем слоте.
// Алгоритм:
// 1. Если кампания не активна глобально -> false.
// 2. Если равномерный режим выключен -> true (участвует всегда, пока активна).
// 3. Если равномерный режим включён:
//   - вычисляем целевую сумму слота;
//   - если цель <= 0 -> false;
//   - иначе участвуем только если потрачено в слоте меньше цели.
func (c *Campaign) ShouldParticipateInSlot(now time.Time) bool {
	if !c.IsActiveGlobal(now) {
		return false
	}
	if !c.EvennessBySlotMode {
		// Неравномерный режим: кампания участвует всегда, пока активна.
		return true
	}
	// Равномерный режим: проверяем, не превышена ли норма слота.
	target := c.SlotTarget(now)
	if target <= 0 {
		// Цели нет (например, уже выполнен общий план)
		return false
	}
	return c.SlotDoneDollars < target
}

// RecordImpression учитывает показ рекламы: увеличивает оба счётчика
// (глобальный и слотовый) на стоимость показа.
func (c *Campaign) RecordImpression(costDollars float64) {
	c.CumDoneDollars += costDollars
	c.SlotDoneDollars += costDollars
}

// ResetSlotDone обнуляет счётчик потраченного в слоте.
// Вызывается при переходе к новому слоту.
func (c *Campaign) ResetSlotDone() {
	c.SlotDoneDollars = 0
}

// SelectCampaign — главная функция выбора кампании для текущего запроса.
// Она реализует логику аукциона с учётом равномерности и таргетинга.
// Последовательность:
//  1. Фильтрация: оставляем только те кампании, которые могут участвовать в слоте
//     (ShouldParticipateInSlot == true).
//  2. Если подходящих нет, возвращаем nil.
//  3. Сортируем отфильтрованные кампании по убыванию BasePriceCPM (самые дорогие вперёд).
//  4. Идём по отсортированному списку и проверяем таргетинг:
//     - если у кампании нет функции таргетинга (nil) или она возвращает true,
//     то выбираем эту кампанию, логируем решение и возвращаем её.
//  5. Если ни одна не прошла таргетинг, возвращаем nil.
func SelectCampaign(campaigns []*Campaign, now time.Time, requestAttrs map[string]interface{}) *Campaign {
	// Шаг 1: фильтрация по участию в слоте
	var eligible []*Campaign
	for _, c := range campaigns {
		if c.ShouldParticipateInSlot(now) {
			eligible = append(eligible, c)
		}
	}
	if len(eligible) == 0 {
		return nil
	}

	// Шаг 2: сортировка по убыванию CPM (самые дорогие первыми)
	// Используем простую пузырьковую сортировку (для демонстрации).
	// В реальном проекте лучше использовать sort.Slice.
	for i := 0; i < len(eligible)-1; i++ {
		for j := i + 1; j < len(eligible); j++ {
			if eligible[i].BasePriceCPM < eligible[j].BasePriceCPM {
				eligible[i], eligible[j] = eligible[j], eligible[i]
			}
		}
	}

	// Шаг 3: проверка таргетинга и выбор первой подходящей
	for _, c := range eligible {
		if c.TargetingCheck == nil || c.TargetingCheck(requestAttrs) {
			// Логируем выбор кампании для отладки и мониторинга
			log.Printf(
				"Campaign selected: ID=%s, price=%.2f, evenness=%v, slot_done=%.2f, slot_target=%.2f, cum_done=%.2f",
				c.ID, c.BasePriceCPM, c.EvennessBySlotMode,
				c.SlotDoneDollars, c.SlotTarget(now), c.CumDoneDollars,
			)
			return c
		}
	}
	return nil
}

// SlotTick обрабатывает переход к новому временному слоту.
// Вызывается каждые 5 минут (например, по таймеру).
// Действия:
//   - определяет начало текущего слота как now.Truncate(SlotDuration) — это гарантирует,
//     что все серверы (при синхронизированном времени NTP) будут использовать одинаковые
//     границы слотов.
//   - логирует начало слота;
//   - обнуляет слотовые счётчики (SlotDoneDollars) у всех кампаний.
func SlotTick(campaigns []*Campaign, now time.Time) {
	// Округляем текущее время вниз до ближайшего кратного SlotDuration.
	// Например, если now = 14:02:15, то slotStart = 14:00:00.
	slotStart := now.Truncate(SlotDuration)
	log.Printf("New slot started at %v", slotStart)

	// Обнуляем счётчики потраченного в слоте для всех кампаний.
	for _, c := range campaigns {
		c.ResetSlotDone()
	}
}

// Пример использования (main) демонстрирует базовый сценарий работы системы.
// В реальном приложении кампании загружаются из БД, запросы приходят через HTTP/gRPC,
// а переход по слотам управляется внешним планировщиком (например, time.Ticker).
func main() {
	// Создаём тестовые кампании
	camp1 := &Campaign{
		ID:                 "camp_1",
		BasePriceCPM:       10.0,
		EvennessBySlotMode: true, // равномерный режим
		GoalTotalDollars:   1000,
		CumDoneDollars:     200,
		SlotDoneDollars:    0,
		StartTS:            time.Now().Add(-time.Hour),
		EndTS:              time.Now().Add(time.Hour),
		TargetingCheck: func(attrs map[string]interface{}) bool {
			// Разрешена только для страны "RU"
			country, ok := attrs["country"].(string)
			return ok && country == "RU"
		},
	}
	camp2 := &Campaign{
		ID:                 "camp_2",
		BasePriceCPM:       20.0,
		EvennessBySlotMode: false, // неравномерный режим
		GoalTotalDollars:   500,
		CumDoneDollars:     100,
		SlotDoneDollars:    0,
		StartTS:            time.Now().Add(-30 * time.Minute),
		EndTS:              time.Now().Add(2 * time.Hour),
		TargetingCheck: func(attrs map[string]interface{}) bool {
			// Разрешена всем
			return true
		},
	}
	campaigns := []*Campaign{camp1, camp2}

	now := time.Now()

	// Эмулируем начало нового слота (в реальности вызывается по таймеру)
	SlotTick(campaigns, now)

	// Эмулируем несколько входящих запросов с разными атрибутами
	requests := []map[string]interface{}{
		{"country": "RU"}, // запрос из России
		{"country": "US"}, // запрос из США
		{"country": "RU"}, // ещё один из России
	}

	for i, req := range requests {
		selected := SelectCampaign(campaigns, now, req)
		if selected == nil {
			log.Printf("Request %d: no campaign selected", i)
			continue
		}
		// Имитация показа и списания денег
		cost := 0.01 // стоимость одного показа (упрощённо)
		selected.RecordImpression(cost)
		log.Printf("Request %d: shown by campaign %s, cost %.4f", i, selected.ID, cost)
	}

	// Далее в реальном приложении цикл продолжится: таймер будет вызывать SlotTick
	// каждые 5 минут, а обработчики запросов — SelectCampaign.
}
