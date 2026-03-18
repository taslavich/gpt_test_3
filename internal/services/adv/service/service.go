package adv_service

import (
	"log"
	"math"
	"time"
)

// SlotDuration задаёт длительность слота (5 минут)
const SlotDuration = 5 * time.Minute

// Campaign представляет рекламную кампанию
type Campaign struct {
	ID                 string
	BasePriceCPM       float64                                 // базовая цена CPM
	EvennessBySlotMode bool                                    // режим равномерности по слотам
	GoalTotalDollars   float64                                 // общая цель в долларах
	CumDoneDollars     float64                                 // сколько уже потрачено всего
	SlotDoneDollars    float64                                 // сколько потрачено в текущем слоте
	StartTS            time.Time                               // начало показа кампании
	EndTS              time.Time                               // окончание показа кампании
	TargetingCheck     func(attrs map[string]interface{}) bool // функция проверки таргетинга
	// можно добавить другие поля
}

// IsActiveGlobal проверяет, не закончилась ли кампания по бюджету или времени
func (c *Campaign) IsActiveGlobal(now time.Time) bool {
	if now.Before(c.StartTS) || now.After(c.EndTS) {
		return false
	}
	if c.CumDoneDollars >= c.GoalTotalDollars {
		return false
	}
	return true
}

// SlotsLeft возвращает количество полных слотов, оставшихся до конца кампании
func (c *Campaign) SlotsLeft(now time.Time) int {
	if now.After(c.EndTS) {
		return 0
	}
	remaining := c.EndTS.Sub(now)
	slots := int(math.Ceil(float64(remaining) / float64(SlotDuration)))
	return slots
}

// SlotTarget возвращает целевую сумму на текущий слот с учётом компенсации
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

// ShouldParticipateInSlot определяет, может ли кампания участвовать в текущем слоте
func (c *Campaign) ShouldParticipateInSlot(now time.Time) bool {
	if !c.IsActiveGlobal(now) {
		return false
	}
	if !c.EvennessBySlotMode {
		// в не‑равномерном режиме кампания участвует всегда, пока активна
		return true
	}
	// равномерный режим: проверяем, не выполнена ли уже норма слота
	target := c.SlotTarget(now)
	if target <= 0 {
		// цели нет – возможно, кампания уже выполнила общий план
		return false
	}
	return c.SlotDoneDollars < target
}

// RecordImpression учитывает показ и его стоимость
func (c *Campaign) RecordImpression(costDollars float64) {
	c.CumDoneDollars += costDollars
	c.SlotDoneDollars += costDollars
}

// ResetSlotDone обнуляет счётчик слота (должно вызываться при переходе к новому слоту)
func (c *Campaign) ResetSlotDone() {
	c.SlotDoneDollars = 0
}

// SelectCampaign выбирает подходящую кампанию для текущего запроса
func SelectCampaign(campaigns []*Campaign, now time.Time, requestAttrs map[string]interface{}) *Campaign {
	// сначала фильтруем кампании, которые могут участвовать в слоте
	var eligible []*Campaign
	for _, c := range campaigns {
		if c.ShouldParticipateInSlot(now) {
			eligible = append(eligible, c)
		}
	}
	if len(eligible) == 0 {
		return nil
	}

	// сортируем по убыванию BasePriceCPM (самые дорогие вперёд)
	// для простоты используем пузырьковую сортировку (можно заменить на sort.Slice)
	for i := 0; i < len(eligible)-1; i++ {
		for j := i + 1; j < len(eligible); j++ {
			if eligible[i].BasePriceCPM < eligible[j].BasePriceCPM {
				eligible[i], eligible[j] = eligible[j], eligible[i]
			}
		}
	}

	// проходим по отсортированному списку, применяем таргетинг
	for _, c := range eligible {
		if c.TargetingCheck == nil || c.TargetingCheck(requestAttrs) {
			// логируем решение (можно вынести в отдельную функцию)
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

// SlotTick обрабатывает переход к новому слоту: обнуляет слот‑счётчики у всех кампаний
func SlotTick(campaigns []*Campaign, now time.Time) {
	// Определяем начало текущего слота (округляем вниз)
	slotStart := now.Truncate(SlotDuration)
	log.Printf("New slot started at %v", slotStart)

	for _, c := range campaigns {
		c.ResetSlotDone()
	}
}

// Пример использования
func main() {
	// создадим несколько тестовых кампаний
	camp1 := &Campaign{
		ID:                 "camp_1",
		BasePriceCPM:       10.0,
		EvennessBySlotMode: true,
		GoalTotalDollars:   1000,
		CumDoneDollars:     200,
		SlotDoneDollars:    0,
		StartTS:            time.Now().Add(-time.Hour),
		EndTS:              time.Now().Add(time.Hour),
		TargetingCheck: func(attrs map[string]interface{}) bool {
			// пример: разрешаем только для страны "RU"
			country, ok := attrs["country"].(string)
			return ok && country == "RU"
		},
	}
	camp2 := &Campaign{
		ID:                 "camp_2",
		BasePriceCPM:       20.0,
		EvennessBySlotMode: false,
		GoalTotalDollars:   500,
		CumDoneDollars:     100,
		SlotDoneDollars:    0,
		StartTS:            time.Now().Add(-30 * time.Minute),
		EndTS:              time.Now().Add(2 * time.Hour),
		TargetingCheck: func(attrs map[string]interface{}) bool {
			// разрешена везде
			return true
		},
	}
	campaigns := []*Campaign{camp1, camp2}

	now := time.Now()

	// эмулируем начало слота
	SlotTick(campaigns, now)

	// приходит несколько запросов
	requests := []map[string]interface{}{
		{"country": "RU"},
		{"country": "US"},
		{"country": "RU"},
	}

	for i, req := range requests {
		selected := SelectCampaign(campaigns, now, req)
		if selected == nil {
			log.Printf("Request %d: no campaign selected", i)
			continue
		}
		// имитируем показ и списание денег
		cost := 0.01 // стоимость одного показа в долларах (упрощённо)
		selected.RecordImpression(cost)
		log.Printf("Request %d: shown by campaign %s, cost %.4f", i, selected.ID, cost)
	}

	// через некоторое время можно снова вызвать SlotTick для перехода к следующему слоту
}
