package auction

/*import (
	"math"
	"testing"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

// setupTestService создаёт сервис с тестовыми кампаниями
func setupTestService(t *testing.T) (*AuctionService, []*Campaign) {
	filterProcessor := filter.NewOptimizedFilterProcessor(filter.NewRuleManager())
	service := NewAuctionService(filterProcessor)

	campaigns := []*Campaign{
		{
			ID:                 "camp_high_cpm",
			BasePriceCPM:       50.0,
			EvennessBySlotMode: false,
			GoalTotalDollars:   1000,
			CumDoneDollars:     0,
			SlotDoneDollars:    0,
			StartTS:            time.Now().Add(-1 * time.Hour),
			EndTS:              time.Now().Add(1 * time.Hour),
			DSPURL:             "dsp1.example.com",
			ActiveIntervals:    nil, // всегда активна
		},
		{
			ID:                 "camp_low_cpm",
			BasePriceCPM:       10.0,
			EvennessBySlotMode: false,
			GoalTotalDollars:   500,
			CumDoneDollars:     0,
			SlotDoneDollars:    0,
			StartTS:            time.Now().Add(-1 * time.Hour),
			EndTS:              time.Now().Add(1 * time.Hour),
			DSPURL:             "dsp1.example.com",
			ActiveIntervals:    nil,
		},
		{
			ID:                 "camp_even",
			BasePriceCPM:       30.0,
			EvennessBySlotMode: true,
			GoalTotalDollars:   1000,
			CumDoneDollars:     500,
			SlotDoneDollars:    0,
			StartTS:            time.Now().Add(-1 * time.Hour),
			EndTS:              time.Now().Add(1 * time.Hour),
			DSPURL:             "dsp2.example.com",
			ActiveIntervals:    nil,
		},
	}

	for _, c := range campaigns {
		service.AddCampaign(c)
	}

	return service, campaigns
}

// TestSelectCampaign_Basic проверяет базовый выбор кампании по цене
func TestSelectCampaign_Basic(t *testing.T) {
	service, campaigns := setupTestService(t)

	req := &ortb_V2_5.BidRequest{
		Id: stringPtr("test_req_1"),
	}
	now := time.Now()

	selected := service.SelectCampaign(req, now)

	if selected == nil {
		t.Fatal("Expected campaign to be selected, got nil")
	}

	if selected.ID != "camp_high_cpm" {
		t.Errorf("Expected camp_high_cpm, got %s", selected.ID)
	}

	if selected.ID == campaigns[1].ID {
		t.Error("Low CPM campaign should not be selected when high CPM is available")
	}
}

// TestSelectCampaign_GlobalInactive проверяет, что неактивные кампании не участвуют
func TestSelectCampaign_GlobalInactive(t *testing.T) {
	service, _ := setupTestService(t)

	expiredCamp := &Campaign{
		ID:                 "camp_expired",
		BasePriceCPM:       100.0,
		EvennessBySlotMode: false,
		GoalTotalDollars:   100,
		CumDoneDollars:     0,
		SlotDoneDollars:    0,
		StartTS:            time.Now().Add(-2 * time.Hour),
		EndTS:              time.Now().Add(-1 * time.Hour),
		DSPURL:             "dsp1.example.com",
		ActiveIntervals:    nil,
	}
	service.AddCampaign(expiredCamp)

	req := &ortb_V2_5.BidRequest{
		Id: stringPtr("test_req_2"),
	}
	now := time.Now()

	selected := service.SelectCampaign(req, now)

	if selected != nil && selected.ID == "camp_expired" {
		t.Error("Expired campaign should not be selected")
	}
}

// TestSelectCampaign_BudgetExhausted проверяет, что кампании с исчерпанным бюджетом не участвуют
func TestSelectCampaign_BudgetExhausted(t *testing.T) {
	service, _ := setupTestService(t)

	exhaustedCamp := &Campaign{
		ID:                 "camp_exhausted",
		BasePriceCPM:       100.0,
		EvennessBySlotMode: false,
		GoalTotalDollars:   100,
		CumDoneDollars:     100,
		SlotDoneDollars:    0,
		StartTS:            time.Now().Add(-1 * time.Hour),
		EndTS:              time.Now().Add(1 * time.Hour),
		DSPURL:             "dsp1.example.com",
		ActiveIntervals:    nil,
	}
	service.AddCampaign(exhaustedCamp)

	req := &ortb_V2_5.BidRequest{
		Id: stringPtr("test_req_3"),
	}
	now := time.Now()

	selected := service.SelectCampaign(req, now)

	if selected != nil && selected.ID == "camp_exhausted" {
		t.Error("Campaign with exhausted budget should not be selected")
	}
}

// TestSelectCampaign_EvennessMode проверяет равномерный режим
func TestSelectCampaign_EvennessMode(t *testing.T) {
	service, _ := setupTestService(t)

	var evenCamp *Campaign
	for _, c := range service.campaigns {
		if c.EvennessBySlotMode {
			evenCamp = c
			break
		}
	}

	if evenCamp == nil {
		t.Fatal("Evenness campaign not found")
	}

	req := &ortb_V2_5.BidRequest{
		Id: stringPtr("test_req_4"),
	}
	now := time.Now()

	slotTarget := evenCamp.SlotTarget(now)
	evenCamp.RecordImpression(slotTarget)

	selected := service.SelectCampaign(req, now)

	if selected != nil && selected.ID == evenCamp.ID {
		t.Error("Evenness campaign should not participate after reaching slot target")
	}
}

// TestSlotTick проверяет обнуление счётчиков при переходе слота
func TestSlotTick(t *testing.T) {
	service, campaigns := setupTestService(t)

	now := time.Now()

	for _, camp := range campaigns {
		camp.RecordImpression(10.0)
		if camp.GetSlotDone() == 0 {
			t.Errorf("Campaign %s slot_done should be > 0 after RecordImpression", camp.ID)
		}
	}

	service.SlotTick(now)

	for _, camp := range campaigns {
		if camp.GetSlotDone() != 0 {
			t.Errorf("Campaign %s slot_done should be 0 after SlotTick, got %.2f", camp.ID, camp.GetSlotDone())
		}
	}
}

// TestSlotTargetCalculation проверяет расчёт цели слота
func TestSlotTargetCalculation(t *testing.T) {
	service, _ := setupTestService(t)

	camp := &Campaign{
		ID:               "test_camp",
		GoalTotalDollars: 1000,
		CumDoneDollars:   200,
		StartTS:          time.Now(),
		EndTS:            time.Now().Add(1 * time.Hour),
		ActiveIntervals:  nil,
	}
	service.AddCampaign(camp)

	now := time.Now()
	slotTarget := camp.SlotTarget(now)

	expected := 800.0 / 12.0

	if math.Abs(slotTarget-expected) > 0.01 {
		t.Errorf("Expected slot target %.2f, got %.2f", expected, slotTarget)
	}
}

// TestCompensationAfterUnderDelivery проверяет компенсацию после недокрута
func TestCompensationAfterUnderDelivery(t *testing.T) {
	service, _ := setupTestService(t)

	camp := &Campaign{
		ID:                 "test_camp",
		BasePriceCPM:       10.0,
		EvennessBySlotMode: true,
		GoalTotalDollars:   1000,
		CumDoneDollars:     0,
		SlotDoneDollars:    0,
		StartTS:            time.Now(),
		EndTS:              time.Now().Add(30 * time.Minute),
		ActiveIntervals:    nil,
	}
	service.AddCampaign(camp)

	now := time.Now()

	camp.RecordImpression(100)

	service.SlotTick(now.Add(5 * time.Minute))
	now = now.Add(5 * time.Minute)

	newTarget := camp.SlotTarget(now)
	expected := 180.0

	if math.Abs(newTarget-expected) > 0.01 {
		t.Errorf("Expected compensation target %.2f, got %.2f", expected, newTarget)
	}
}

// TestCompensationAfterOverDelivery проверяет компенсацию после перекрута
func TestCompensationAfterOverDelivery(t *testing.T) {
	service, _ := setupTestService(t)

	camp := &Campaign{
		ID:                 "test_camp",
		BasePriceCPM:       10.0,
		EvennessBySlotMode: true,
		GoalTotalDollars:   1000,
		CumDoneDollars:     0,
		SlotDoneDollars:    0,
		StartTS:            time.Now(),
		EndTS:              time.Now().Add(30 * time.Minute),
		ActiveIntervals:    nil,
	}
	service.AddCampaign(camp)

	now := time.Now()

	camp.RecordImpression(200)

	service.SlotTick(now.Add(5 * time.Minute))
	now = now.Add(5 * time.Minute)

	newTarget := camp.SlotTarget(now)
	expected := 160.0

	if math.Abs(newTarget-expected) > 0.01 {
		t.Errorf("Expected reduced target %.2f, got %.2f", expected, newTarget)
	}
}

// TestMultipleRequestsInSlot проверяет несколько запросов в одном слоте
func TestMultipleRequestsInSlot(t *testing.T) {
	service, _ := setupTestService(t)

	camp := &Campaign{
		ID:                 "test_camp",
		BasePriceCPM:       10.0,
		EvennessBySlotMode: true,
		GoalTotalDollars:   100,
		CumDoneDollars:     0,
		SlotDoneDollars:    0,
		StartTS:            time.Now(),
		EndTS:              time.Now().Add(5 * time.Minute),
		DSPURL:             "dsp1.example.com",
		ActiveIntervals:    nil,
	}
	service.AddCampaign(camp)

	req := &ortb_V2_5.BidRequest{
		Id: stringPtr("test_req"),
	}
	now := time.Now()

	selected1 := service.SelectCampaign(req, now)
	if selected1 == nil {
		t.Fatal("First request: no campaign selected")
	}

	selected1.RecordImpression(60)

	selected2 := service.SelectCampaign(req, now)
	if selected2 == nil {
		t.Error("Second request: campaign should still be available")
	}

	selected2.RecordImpression(50)

	selected3 := service.SelectCampaign(req, now)
	if selected3 != nil && selected3.ID == camp.ID {
		t.Error("Third request: campaign should not participate after exceeding slot target")
	}
}

// TestPricePriority проверяет приоритет по цене
func TestPricePriority(t *testing.T) {
	service, _ := setupTestService(t)

	camps := []*Campaign{
		{ID: "cpm_100", BasePriceCPM: 100, EvennessBySlotMode: false, GoalTotalDollars: 1000, StartTS: time.Now().Add(-1 * time.Hour), EndTS: time.Now().Add(1 * time.Hour), DSPURL: "dsp1.com", ActiveIntervals: nil},
		{ID: "cpm_200", BasePriceCPM: 200, EvennessBySlotMode: false, GoalTotalDollars: 1000, StartTS: time.Now().Add(-1 * time.Hour), EndTS: time.Now().Add(1 * time.Hour), DSPURL: "dsp1.com", ActiveIntervals: nil},
		{ID: "cpm_150", BasePriceCPM: 150, EvennessBySlotMode: false, GoalTotalDollars: 1000, StartTS: time.Now().Add(-1 * time.Hour), EndTS: time.Now().Add(1 * time.Hour), DSPURL: "dsp1.com", ActiveIntervals: nil},
	}

	for _, c := range camps {
		service.AddCampaign(c)
	}

	req := &ortb_V2_5.BidRequest{
		Id: stringPtr("test_req"),
	}
	now := time.Now()

	selected := service.SelectCampaign(req, now)

	if selected == nil {
		t.Fatal("No campaign selected")
	}

	if selected.ID != "cpm_200" {
		t.Errorf("Expected highest CPM campaign (cpm_200), got %s", selected.ID)
	}
}

// TestFilter_GeoMatch проверяет, что кампания с правилом "язык = ru" проходит запрос с русским языком
func TestFilter_GeoMatch(t *testing.T) {
	ruleManager := filter.NewRuleManager()
	ruleManager.ClearAllDSPRules()
	fp := filter.NewOptimizedFilterProcessor(ruleManager)

	rule := &filter.FilterRule{
		ID:        "lang_ru",
		Field:     filter.FieldDeviceLanguage,
		Condition: filter.ConditionEqual,
		Value:     filter.NewStringCondition(filter.ConditionEqual, "ru"),
	}
	rootNode := &filter.CompiledRuleNode{
		Rule:     rule,
		Operator: "leaf",
	}
	ruleManager.SetDSPRules("dsp1|v25", []*filter.CompiledRuleNode{rootNode}, []*filter.FilterRule{rule})

	camp := &Campaign{
		ID:                 "camp_ru",
		BasePriceCPM:       10.0,
		EvennessBySlotMode: false,
		GoalTotalDollars:   1000,
		StartTS:            time.Now().Add(-1 * time.Hour),
		EndTS:              time.Now().Add(1 * time.Hour),
		DSPURL:             "dsp1",
		ActiveIntervals:    nil,
	}
	service := NewAuctionService(fp)
	service.AddCampaign(camp)

	langRu := "ru"
	req := &ortb_V2_5.BidRequest{
		Id: stringPtr("req1"),
		Device: &ortb_V2_5.Device{
			Language: &langRu,
		},
	}
	now := time.Now()

	selected := service.SelectCampaign(req, now)
	if selected == nil {
		t.Fatal("Expected campaign to be selected, got nil")
	}
	if selected.ID != "camp_ru" {
		t.Errorf("Expected camp_ru, got %s", selected.ID)
	}
}

// TestFilter_GeoMismatch – язык не совпадает (en вместо ru)
func TestFilter_GeoMismatch(t *testing.T) {
	ruleManager := filter.NewRuleManager()
	ruleManager.ClearAllDSPRules()
	fp := filter.NewOptimizedFilterProcessor(ruleManager)

	rule := &filter.FilterRule{
		ID:        "lang_ru",
		Field:     filter.FieldDeviceLanguage,
		Condition: filter.ConditionEqual,
		Value:     filter.NewStringCondition(filter.ConditionEqual, "ru"),
	}
	rootNode := &filter.CompiledRuleNode{Rule: rule, Operator: "leaf"}
	ruleManager.SetDSPRules("dsp1|v25", []*filter.CompiledRuleNode{rootNode}, []*filter.FilterRule{rule})

	camp := &Campaign{
		ID:              "camp_ru",
		DSPURL:          "dsp1",
		StartTS:         time.Now().Add(-1 * time.Hour),
		EndTS:           time.Now().Add(1 * time.Hour),
		ActiveIntervals: nil,
	}
	service := NewAuctionService(fp)
	service.AddCampaign(camp)

	langEn := "en"
	req := &ortb_V2_5.BidRequest{
		Id: stringPtr("req2"),
		Device: &ortb_V2_5.Device{
			Language: &langEn,
		},
	}
	now := time.Now()

	selected := service.SelectCampaign(req, now)
	if selected != nil {
		t.Errorf("Expected no campaign, but got %s", selected.ID)
	}
}

// TestFilter_AndCondition – комбинация (язык=ru AND сайт=example.com)
func TestFilter_AndCondition(t *testing.T) {
	ruleManager := filter.NewRuleManager()
	ruleManager.ClearAllDSPRules()
	fp := filter.NewOptimizedFilterProcessor(ruleManager)

	ruleLang := &filter.FilterRule{
		ID:        "lang_ru",
		Field:     filter.FieldDeviceLanguage,
		Condition: filter.ConditionEqual,
		Value:     filter.NewStringCondition(filter.ConditionEqual, "ru"),
	}
	ruleSite := &filter.FilterRule{
		ID:        "site_example",
		Field:     filter.FieldSiteDomain,
		Condition: filter.ConditionEqual,
		Value:     filter.NewStringCondition(filter.ConditionEqual, "example.com"),
	}
	andNode := &filter.CompiledRuleNode{
		Operator: "and",
		Children: []*filter.CompiledRuleNode{
			{Rule: ruleLang, Operator: "leaf"},
			{Rule: ruleSite, Operator: "leaf"},
		},
	}
	ruleManager.SetDSPRules("dsp1|v25", []*filter.CompiledRuleNode{andNode}, []*filter.FilterRule{ruleLang, ruleSite})

	camp := &Campaign{
		ID:               "camp_and",
		DSPURL:           "dsp1",
		StartTS:          time.Now().Add(-1 * time.Hour),
		EndTS:            time.Now().Add(1 * time.Hour),
		GoalTotalDollars: 1000,
		CumDoneDollars:   0,
		ActiveIntervals:  nil,
	}
	service := NewAuctionService(fp)
	service.AddCampaign(camp)

	langRu := "ru"
	site := &ortb_V2_5.Site{Domain: stringPtr("other.com")}
	req := &ortb_V2_5.BidRequest{
		Id:   stringPtr("req3"),
		Site: site,
		Device: &ortb_V2_5.Device{
			Language: &langRu,
		},
	}
	now := time.Now()

	selected := service.SelectCampaign(req, now)
	if selected != nil {
		t.Errorf("Expected no campaign because site domain mismatch, got %s", selected.ID)
	}

	req.Site.Domain = stringPtr("example.com")
	selected = service.SelectCampaign(req, now)
	if selected == nil {
		t.Fatal("Expected campaign to be selected, got nil")
	}
	if selected.ID != "camp_and" {
		t.Errorf("Expected camp_and, got %s", selected.ID)
	}
}

// TestFilter_NoTargeting – кампания без правил всегда подходит
func TestFilter_NoTargeting(t *testing.T) {
	ruleManager := filter.NewRuleManager()
	ruleManager.ClearAllDSPRules()
	fp := filter.NewOptimizedFilterProcessor(ruleManager)

	camp := &Campaign{
		ID:               "camp_no_rules",
		DSPURL:           "dsp_no_rules",
		StartTS:          time.Now().Add(-1 * time.Hour),
		EndTS:            time.Now().Add(1 * time.Hour),
		GoalTotalDollars: 1000,
		CumDoneDollars:   0,
		ActiveIntervals:  nil,
	}
	service := NewAuctionService(fp)
	service.AddCampaign(camp)

	req := &ortb_V2_5.BidRequest{Id: stringPtr("req4")}
	now := time.Now()
	selected := service.SelectCampaign(req, now)
	if selected == nil {
		t.Fatal("Expected campaign with no rules to be selected, got nil")
	}
	if selected.ID != "camp_no_rules" {
		t.Errorf("Expected camp_no_rules, got %s", selected.ID)
	}
}

// TestFilter_Blacklist – чёрный список (язык != "en")
func TestFilter_Blacklist(t *testing.T) {
	ruleManager := filter.NewRuleManager()
	ruleManager.ClearAllDSPRules()
	fp := filter.NewOptimizedFilterProcessor(ruleManager)

	rule := &filter.FilterRule{
		ID:        "not_en",
		Field:     filter.FieldDeviceLanguage,
		Condition: filter.ConditionNotEqual,
		Value:     filter.NewStringCondition(filter.ConditionNotEqual, "en"),
	}
	rootNode := &filter.CompiledRuleNode{Rule: rule, Operator: "leaf"}
	ruleManager.SetDSPRules("dsp1|v25", []*filter.CompiledRuleNode{rootNode}, []*filter.FilterRule{rule})

	camp := &Campaign{
		ID:               "camp_no_en",
		DSPURL:           "dsp1",
		StartTS:          time.Now().Add(-1 * time.Hour),
		EndTS:            time.Now().Add(1 * time.Hour),
		GoalTotalDollars: 1000,
		CumDoneDollars:   0,
		ActiveIntervals:  nil,
	}
	service := NewAuctionService(fp)
	service.AddCampaign(camp)

	langEn := "en"
	req := &ortb_V2_5.BidRequest{
		Id: stringPtr("req5"),
		Device: &ortb_V2_5.Device{
			Language: &langEn,
		},
	}
	now := time.Now()
	selected := service.SelectCampaign(req, now)
	if selected != nil {
		t.Errorf("Expected campaign to be blocked (language=en), but got %s", selected.ID)
	}

	langRu := "ru"
	req.Device.Language = &langRu
	selected = service.SelectCampaign(req, now)
	if selected == nil {
		t.Fatal("Expected campaign to be allowed (language=ru), got nil")
	}
}

// TestFilter_MultipleCampaignsPriority – выбирается самая дорогая из подходящих по таргетингу
func TestFilter_MultipleCampaignsPriority(t *testing.T) {
	ruleManager := filter.NewRuleManager()
	ruleManager.ClearAllDSPRules()
	fp := filter.NewOptimizedFilterProcessor(ruleManager)

	ruleRu := &filter.FilterRule{
		ID:        "lang_ru",
		Field:     filter.FieldDeviceLanguage,
		Condition: filter.ConditionEqual,
		Value:     filter.NewStringCondition(filter.ConditionEqual, "ru"),
	}
	rootRu := &filter.CompiledRuleNode{Rule: ruleRu, Operator: "leaf"}
	ruleManager.SetDSPRules("dsp_ru|v25", []*filter.CompiledRuleNode{rootRu}, []*filter.FilterRule{ruleRu})

	ruleEn := &filter.FilterRule{
		ID:        "lang_en",
		Field:     filter.FieldDeviceLanguage,
		Condition: filter.ConditionEqual,
		Value:     filter.NewStringCondition(filter.ConditionEqual, "en"),
	}
	rootEn := &filter.CompiledRuleNode{Rule: ruleEn, Operator: "leaf"}
	ruleManager.SetDSPRules("dsp_en|v25", []*filter.CompiledRuleNode{rootEn}, []*filter.FilterRule{ruleEn})

	campRu := &Campaign{
		ID:               "camp_ru",
		BasePriceCPM:     50.0,
		DSPURL:           "dsp_ru",
		StartTS:          time.Now().Add(-1 * time.Hour),
		EndTS:            time.Now().Add(1 * time.Hour),
		GoalTotalDollars: 1000,
		CumDoneDollars:   0,
		ActiveIntervals:  nil,
	}
	campEn := &Campaign{
		ID:               "camp_en",
		BasePriceCPM:     100.0,
		DSPURL:           "dsp_en",
		StartTS:          time.Now().Add(-1 * time.Hour),
		EndTS:            time.Now().Add(1 * time.Hour),
		GoalTotalDollars: 1000,
		CumDoneDollars:   0,
		ActiveIntervals:  nil,
	}
	service := NewAuctionService(fp)
	service.AddCampaign(campRu)
	service.AddCampaign(campEn)

	langRuStr := "ru"
	req := &ortb_V2_5.BidRequest{
		Id: stringPtr("req6"),
		Device: &ortb_V2_5.Device{
			Language: &langRuStr,
		},
	}
	now := time.Now()

	selected := service.SelectCampaign(req, now)
	if selected == nil {
		t.Fatal("Expected a campaign to be selected")
	}
	if selected.ID != "camp_ru" {
		t.Errorf("Expected camp_ru (matching targeting), got %s (CPM=%.2f)", selected.ID, selected.BasePriceCPM)
	}
}

// TestInterval_NoIntervals – кампания без интервалов всегда активна
func TestInterval_NoIntervals(t *testing.T) {
	ruleManager := filter.NewRuleManager()
	ruleManager.ClearAllDSPRules()
	fp := filter.NewOptimizedFilterProcessor(ruleManager)
	service := NewAuctionService(fp)

	camp := &Campaign{
		ID:                 "no_intervals",
		BasePriceCPM:       10.0,
		EvennessBySlotMode: false,
		GoalTotalDollars:   1000,
		CumDoneDollars:     0,
		StartTS:            time.Now().Add(-1 * time.Hour),
		EndTS:              time.Now().Add(1 * time.Hour),
		DSPURL:             "dsp1.com",
		ActiveIntervals:    nil,
	}
	service.AddCampaign(camp)

	req := &ortb_V2_5.BidRequest{Id: stringPtr("req")}
	now := time.Now()

	selected := service.SelectCampaign(req, now)
	if selected == nil || selected.ID != "no_intervals" {
		t.Error("Campaign without intervals should be active")
	}
}

// TestInterval_SingleInside – один интервал, время внутри
func TestInterval_SingleInside(t *testing.T) {
	ruleManager := filter.NewRuleManager()
	ruleManager.ClearAllDSPRules()
	fp := filter.NewOptimizedFilterProcessor(ruleManager)
	service := NewAuctionService(fp)

	start := time.Date(2025, 3, 12, 10, 0, 0, 0, time.UTC)
	end := time.Date(2025, 3, 12, 18, 0, 0, 0, time.UTC)

	camp := &Campaign{
		ID:                 "interval_camp",
		BasePriceCPM:       10.0,
		EvennessBySlotMode: false,
		GoalTotalDollars:   1000,
		CumDoneDollars:     0,
		StartTS:            start.Add(-1 * time.Hour),
		EndTS:              end.Add(1 * time.Hour),
		DSPURL:             "dsp1",
		ActiveIntervals:    []TimeRange{{Start: start, End: end}},
	}
	service.AddCampaign(camp)

	// Время внутри интервала (12 марта 14:00 UTC)
	inside := time.Date(2025, 3, 12, 14, 0, 0, 0, time.UTC)
	req := &ortb_V2_5.BidRequest{Id: stringPtr("req")}
	selected := service.SelectCampaign(req, inside)
	if selected == nil {
		t.Fatal("Expected campaign to be active inside interval, got nil")
	}
	if selected.ID != "interval_camp" {
		t.Errorf("Expected interval_camp, got %s", selected.ID)
	}
}

// TestInterval_SingleOutside – один интервал, время снаружи
func TestInterval_SingleOutside(t *testing.T) {
	ruleManager := filter.NewRuleManager()
	ruleManager.ClearAllDSPRules()
	fp := filter.NewOptimizedFilterProcessor(ruleManager)
	service := NewAuctionService(fp)

	start := time.Date(2025, 3, 12, 10, 0, 0, 0, time.UTC)
	end := time.Date(2025, 3, 12, 18, 0, 0, 0, time.UTC)

	camp := &Campaign{
		ID:                 "interval_camp",
		BasePriceCPM:       10.0,
		EvennessBySlotMode: false,
		GoalTotalDollars:   1000,
		CumDoneDollars:     0,
		StartTS:            start.Add(-1 * time.Hour),
		EndTS:              end.Add(1 * time.Hour),
		DSPURL:             "dsp1",
		ActiveIntervals:    []TimeRange{{Start: start, End: end}},
	}
	service.AddCampaign(camp)

	// Время до интервала (12 марта 09:00 UTC)
	outside := time.Date(2025, 3, 12, 9, 0, 0, 0, time.UTC)
	req := &ortb_V2_5.BidRequest{Id: stringPtr("req")}
	selected := service.SelectCampaign(req, outside)
	if selected != nil {
		t.Errorf("Expected no campaign outside interval, but got %s", selected.ID)
	}

	// Время после интервала (12 марта 19:00 UTC)
	outside = time.Date(2025, 3, 12, 19, 0, 0, 0, time.UTC)
	selected = service.SelectCampaign(req, outside)
	if selected != nil {
		t.Errorf("Expected no campaign after interval, but got %s", selected.ID)
	}
}

// TestInterval_StartEndBoundary – проверка границ: start включительно, end исключительно
func TestInterval_StartEndBoundary(t *testing.T) {
	ruleManager := filter.NewRuleManager()
	ruleManager.ClearAllDSPRules()
	fp := filter.NewOptimizedFilterProcessor(ruleManager)
	service := NewAuctionService(fp)

	start := time.Date(2025, 3, 12, 10, 0, 0, 0, time.UTC)
	end := time.Date(2025, 3, 12, 18, 0, 0, 0, time.UTC)

	camp := &Campaign{
		ID:                 "boundary_camp",
		BasePriceCPM:       10.0,
		EvennessBySlotMode: false,
		GoalTotalDollars:   1000,
		CumDoneDollars:     0,
		StartTS:            start.Add(-1 * time.Hour),
		EndTS:              end.Add(1 * time.Hour),
		DSPURL:             "dsp1",
		ActiveIntervals:    []TimeRange{{Start: start, End: end}},
	}
	service.AddCampaign(camp)
	req := &ortb_V2_5.BidRequest{Id: stringPtr("req")}

	// Точно в момент начала – должно быть активно
	atStart := start
	selected := service.SelectCampaign(req, atStart)
	if selected == nil {
		t.Error("Campaign should be active exactly at start time (inclusive)")
	}

	// За 1 наносекунду до конца – активно
	justBeforeEnd := end.Add(-1 * time.Nanosecond)
	selected = service.SelectCampaign(req, justBeforeEnd)
	if selected == nil {
		t.Error("Campaign should be active just before end time")
	}

	// Точно в момент конца – не активно (end exclusive)
	atEnd := end
	selected = service.SelectCampaign(req, atEnd)
	if selected != nil {
		t.Error("Campaign should NOT be active exactly at end time (exclusive)")
	}
}

// TestInterval_Multiple – несколько интервалов, время внутри одного из них
func TestInterval_Multiple(t *testing.T) {
	ruleManager := filter.NewRuleManager()
	ruleManager.ClearAllDSPRules()
	fp := filter.NewOptimizedFilterProcessor(ruleManager)
	service := NewAuctionService(fp)

	interval1 := TimeRange{
		Start: time.Date(2025, 3, 12, 9, 0, 0, 0, time.UTC),
		End:   time.Date(2025, 3, 12, 12, 0, 0, 0, time.UTC),
	}
	interval2 := TimeRange{
		Start: time.Date(2025, 3, 12, 14, 0, 0, 0, time.UTC),
		End:   time.Date(2025, 3, 12, 18, 0, 0, 0, time.UTC),
	}
	camp := &Campaign{
		ID:                 "multi_camp",
		BasePriceCPM:       10.0,
		EvennessBySlotMode: false,
		GoalTotalDollars:   1000,
		CumDoneDollars:     0,
		StartTS:            time.Date(2025, 3, 12, 0, 0, 0, 0, time.UTC),
		EndTS:              time.Date(2025, 3, 12, 23, 59, 59, 0, time.UTC),
		DSPURL:             "dsp1",
		ActiveIntervals:    []TimeRange{interval1, interval2},
	}
	service.AddCampaign(camp)
	req := &ortb_V2_5.BidRequest{Id: stringPtr("req")}

	// Время в первом интервале (10:00)
	inside1 := time.Date(2025, 3, 12, 10, 0, 0, 0, time.UTC)
	selected := service.SelectCampaign(req, inside1)
	if selected == nil {
		t.Fatal("Expected campaign active in first interval")
	}
	// Время во втором интервале (15:00)
	inside2 := time.Date(2025, 3, 12, 15, 0, 0, 0, time.UTC)
	selected = service.SelectCampaign(req, inside2)
	if selected == nil {
		t.Fatal("Expected campaign active in second interval")
	}
	// Время между интервалами (13:00)
	between := time.Date(2025, 3, 12, 13, 0, 0, 0, time.UTC)
	selected = service.SelectCampaign(req, between)
	if selected != nil {
		t.Error("Campaign should not be active between intervals")
	}
}

// TestInterval_CombinedWithGlobalBounds – интервалы пересекаются с глобальными StartTS/EndTS
func TestInterval_CombinedWithGlobalBounds(t *testing.T) {
	ruleManager := filter.NewRuleManager()
	ruleManager.ClearAllDSPRules()
	fp := filter.NewOptimizedFilterProcessor(ruleManager)
	service := NewAuctionService(fp)

	globalStart := time.Date(2025, 3, 12, 8, 0, 0, 0, time.UTC)
	globalEnd := time.Date(2025, 3, 12, 20, 0, 0, 0, time.UTC)

	interval := TimeRange{
		Start: time.Date(2025, 3, 12, 10, 0, 0, 0, time.UTC),
		End:   time.Date(2025, 3, 12, 18, 0, 0, 0, time.UTC),
	}
	camp := &Campaign{
		ID:                 "combined",
		BasePriceCPM:       10.0,
		EvennessBySlotMode: false,
		GoalTotalDollars:   1000,
		CumDoneDollars:     0,
		StartTS:            globalStart,
		EndTS:              globalEnd,
		DSPURL:             "dsp1",
		ActiveIntervals:    []TimeRange{interval},
	}
	service.AddCampaign(camp)
	req := &ortb_V2_5.BidRequest{Id: stringPtr("req")}

	// Время внутри интервала и внутри глобальных границ – активно
	valid := time.Date(2025, 3, 12, 12, 0, 0, 0, time.UTC)
	selected := service.SelectCampaign(req, valid)
	if selected == nil {
		t.Error("Should be active when inside both interval and global bounds")
	}

	// Время внутри интервала, но до глобального StartTS – не активно
	beforeGlobal := time.Date(2025, 3, 12, 9, 0, 0, 0, time.UTC)
	selected = service.SelectCampaign(req, beforeGlobal)
	if selected != nil {
		t.Error("Should be inactive because before global StartTS")
	}

	// Время внутри интервала, но после глобального EndTS – не активно
	afterGlobal := time.Date(2025, 3, 12, 19, 0, 0, 0, time.UTC)
	selected = service.SelectCampaign(req, afterGlobal)
	if selected != nil {
		t.Error("Should be inactive because after global EndTS")
	}
}

// TestInterval_WithBudgetAndSlot – проверка, что интервалы работают вместе с бюджетом и равномерностью
func TestInterval_WithBudgetAndSlot(t *testing.T) {
	ruleManager := filter.NewRuleManager()
	ruleManager.ClearAllDSPRules()
	fp := filter.NewOptimizedFilterProcessor(ruleManager)
	service := NewAuctionService(fp)

	start := time.Date(2025, 3, 12, 10, 0, 0, 0, time.UTC)
	end := time.Date(2025, 3, 12, 10, 10, 0, 0, time.UTC) // 10 минут = 2 слота

	camp := &Campaign{
		ID:                 "interval_budget",
		BasePriceCPM:       10.0,
		EvennessBySlotMode: true,
		GoalTotalDollars:   100,
		CumDoneDollars:     0,
		SlotDoneDollars:    0,
		StartTS:            start,
		EndTS:              end,
		DSPURL:             "dsp1",
		ActiveIntervals:    []TimeRange{{Start: start, End: end}},
	}
	service.AddCampaign(camp)
	req := &ortb_V2_5.BidRequest{Id: stringPtr("req")}

	// Время внутри интервала, первый слот
	now := start.Add(1 * time.Minute) // 10:01
	selected := service.SelectCampaign(req, now)
	if selected == nil {
		t.Fatal("Campaign should be active inside interval")
	}
	selected.RecordImpression(60) // тратим 60

	// Второй запрос в том же слоте (цель слота = 100/2 = 50, потрачено 60 -> лимит)
	selected2 := service.SelectCampaign(req, now)
	if selected2 != nil {
		t.Error("Should be inactive because slot target exceeded")
	}

	// Переход на следующий слот (10:05)
	service.SlotTick(now.Add(5 * time.Minute))
	now = now.Add(5 * time.Minute)
	selected3 := service.SelectCampaign(req, now)
	if selected3 == nil {
		t.Error("Should be active in second slot because interval still active")
	}
}

// Helper
func stringPtr(s string) *string {
	return &s
}
*/
