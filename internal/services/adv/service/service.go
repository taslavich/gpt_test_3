package auction

import (
	"log"
	"math"
	"sync"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

// SlotDuration задаёт длительность одного временного слота (5 минут)
const SlotDuration = 5 * time.Minute

// TimeRange представляет интервал времени с точными датой и временем (UTC)
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Campaign представляет рекламную кампанию
type Campaign struct {
	// Основные поля
	ID                 string
	BasePriceCPM       float64
	EvennessBySlotMode bool
	GoalTotalDollars   float64
	CumDoneDollars     float64
	SlotDoneDollars    float64
	StartTS            time.Time
	EndTS              time.Time

	// Связь с фильтрами
	DSPURL string // URL DSP, которому принадлежит кампания

	// Активные интервалы (nil или пустой = всегда активна)
	ActiveIntervals []TimeRange

	// Защита от конкурентного доступа
	mu sync.RWMutex
}

// IsActiveInIntervals проверяет, попадает ли текущее время в один из заданных интервалов
func (c *Campaign) IsActiveInIntervals(now time.Time) bool {
	if len(c.ActiveIntervals) == 0 {
		return true
	}
	for _, interval := range c.ActiveIntervals {
		if (now.Equal(interval.Start) || now.After(interval.Start)) &&
			(now.Equal(interval.End) || now.Before(interval.End)) {
			return true
		}
	}
	return false
}

// IsActiveGlobal проверяет, активна ли кампания глобально
func (c *Campaign) IsActiveGlobal(now time.Time) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if now.Before(c.StartTS) || now.After(c.EndTS) {
		return false
	}
	if !c.IsActiveInIntervals(now) {
		return false
	}
	return c.CumDoneDollars < c.GoalTotalDollars
}

// SlotsLeft возвращает количество полных слотов до окончания кампании
func (c *Campaign) SlotsLeft(now time.Time) int {
	if now.After(c.EndTS) {
		return 0
	}
	remaining := c.EndTS.Sub(now)
	return int(math.Ceil(float64(remaining) / float64(SlotDuration)))
}

// SlotTarget вычисляет целевую сумму на текущий слот
func (c *Campaign) SlotTarget(now time.Time) float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

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

// ShouldParticipateInSlot определяет, может ли кампания участвовать в слоте
func (c *Campaign) ShouldParticipateInSlot(now time.Time) bool {
	if !c.IsActiveGlobal(now) {
		return false
	}
	if !c.EvennessBySlotMode {
		return true
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	target := c.SlotTarget(now)
	if target <= 0 {
		return false
	}
	return c.SlotDoneDollars < target
}

// RecordImpression учитывает показ
func (c *Campaign) RecordImpression(costDollars float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.CumDoneDollars += costDollars
	c.SlotDoneDollars += costDollars
}

// ResetSlotDone обнуляет счётчик слота
func (c *Campaign) ResetSlotDone() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.SlotDoneDollars = 0
}

// GetCumDone возвращает потраченный бюджет (для тестов)
func (c *Campaign) GetCumDone() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.CumDoneDollars
}

// GetSlotDone возвращает потраченное в слоте (для тестов)
func (c *Campaign) GetSlotDone() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.SlotDoneDollars
}

// AuctionService - сервис аукциона
type AuctionService struct {
	campaigns       map[string]*Campaign
	filterProcessor *filter.OptimizedFilterProcessor
	mu              sync.RWMutex
}

// NewAuctionService создаёт новый сервис аукциона
func NewAuctionService(filterProcessor *filter.OptimizedFilterProcessor) *AuctionService {
	return &AuctionService{
		campaigns:       make(map[string]*Campaign),
		filterProcessor: filterProcessor,
	}
}

// AddCampaign добавляет кампанию в сервис
func (s *AuctionService) AddCampaign(campaign *Campaign) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.campaigns[campaign.ID] = campaign
}

// GetCampaign возвращает кампанию по ID
func (s *AuctionService) GetCampaign(id string) *Campaign {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.campaigns[id]
}

// SelectCampaign выбирает кампанию для BidRequest
func (s *AuctionService) SelectCampaign(req *ortb_V2_5.BidRequest, now time.Time) *Campaign {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Шаг 1: Фильтрация по участию в слоте
	var eligible []*Campaign
	for _, campaign := range s.campaigns {
		if campaign.ShouldParticipateInSlot(now) {
			eligible = append(eligible, campaign)
		}
	}

	if len(eligible) == 0 {
		return nil
	}

	// Шаг 2: Применяем фильтры и сортируем по цене
	// Сначала фильтруем по правилам DSP
	var filtered []*Campaign
	for _, campaign := range eligible {
		// Применяем фильтры для DSP
		filterResult := s.filterProcessor.ProcessRequestForDSPV25(campaign.DSPURL, req)
		if filterResult.Allowed {
			filtered = append(filtered, campaign)
		}
	}

	if len(filtered) == 0 {
		return nil
	}

	// Шаг 3: Сортируем по убыванию CPM
	for i := 0; i < len(filtered)-1; i++ {
		for j := i + 1; j < len(filtered); j++ {
			if filtered[i].BasePriceCPM < filtered[j].BasePriceCPM {
				filtered[i], filtered[j] = filtered[j], filtered[i]
			}
		}
	}

	// Шаг 4: Логируем и возвращаем победителя
	selected := filtered[0]
	log.Printf("[Auction] Selected campaign: ID=%s, CPM=%.2f, DSP=%s, slot_done=%.2f, slot_target=%.2f",
		selected.ID, selected.BasePriceCPM, selected.DSPURL,
		selected.GetSlotDone(), selected.SlotTarget(now))

	return selected
}

// SlotTick обрабатывает переход к новому слоту
func (s *AuctionService) SlotTick(now time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	slotStart := now.Truncate(SlotDuration)
	log.Printf("[Auction] New slot started at %v", slotStart)

	for _, campaign := range s.campaigns {
		campaign.ResetSlotDone()
	}
}

// GetActiveCampaignsCount возвращает количество активных кампаний (для тестов)
func (s *AuctionService) GetActiveCampaignsCount(now time.Time) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, campaign := range s.campaigns {
		if campaign.IsActiveGlobal(now) {
			count++
		}
	}
	return count
}
