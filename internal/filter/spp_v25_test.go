package filter

import (
	"os"
	"testing"

	ortb_V2_5 "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func TestSPPV25Rules(t *testing.T) {
	// Создаем временный файл с правилами SPP
	rulesContent := `{
		"version": "1.0",
		"spps": {
			"sppY": {
				"v25": [
					{
						"and": [
							{ "field": "bid.price", "condition": "greater_equal", "value_type": "float", "value": 0.5 },
							{
								"or": [
									{
										"and": [
											{ "field": "bid.w", "condition": "greater_than", "value_type": "int", "value": 0 },
											{ "field": "bid.h", "condition": "greater_than", "value_type": "int", "value": 0 }
										]
									},
									{
										"or": [
											{ "field": "bid.adomain", "condition": "exists", "value_type": "string", "value": "" },
											{ "field": "bid.crid", "condition": "exists", "value_type": "string", "value": "" }
										]
									}
								]
							},
							{ "field": "bidresponse.cur", "condition": "exists", "value_type": "string", "value": "" }
						]
					},
					{ "field": "bid.price", "condition": "not_between", "value_type": "float", "value": [5.0, 8.0] },
					{
						"and": [
							{ "field": "bid.w", "condition": "between", "value_type": "int", "value": [100, 1200] },
							{ "field": "bid.h", "condition": "between", "value_type": "int", "value": [100, 1200] }
						]
					}
				]
			}
		}
	}`

	// Создаем временный файл
	tmpFile, err := os.CreateTemp("", "spp_rules_*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Записываем правила в файл
	if _, err := tmpFile.WriteString(rulesContent); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// Инициализация менеджера правил и загрузчика
	ruleManager := NewRuleManager()
	loader := NewFileRuleLoader(ruleManager, "", "", tmpFile.Name(), tmpFile.Name())

	err = loader.LoadSPPRules()
	if err != nil {
		t.Fatalf("Failed to load SPP rules: %v", err)
	}

	processor := NewOptimizedFilterProcessor(ruleManager)

	// Тестовый случай для отладки
	debugResponse := &ortb_V2_5.BidResponse{
		Cur: stringPtr("USD"),
		Seatbid: &ortb_V2_5.SeatBid{
			Bid: []*ortb_V2_5.Bid{
				{
					Price:   floatPtr(4.5),
					W:       intPtr(800),
					H:       intPtr(600),
					Adomain: []string{"example.com"},
					Crid:    stringPtr("creative123"),
					Nurl:    stringPtr("https://tracker.example/win"),  // <— ДОБАВИТЬ
					Burl:    stringPtr("https://tracker.example/bill"), // <— ДОБАВИТЬ
				},
			},
		},
	}

	// Отладочная информация
	t.Log("=== DEBUG INFO ===")
	t.Logf("BidResponse.Cur: %s", *debugResponse.Cur)
	t.Logf("Bid.Price: %.2f", *debugResponse.Seatbid.Bid[0].Price)
	t.Logf("Bid.W: %d", *debugResponse.Seatbid.Bid[0].W)
	t.Logf("Bid.H: %d", *debugResponse.Seatbid.Bid[0].H)
	t.Logf("Bid.Adomain: %v", debugResponse.Seatbid.Bid[0].Adomain)
	t.Logf("Bid.Crid: %s", *debugResponse.Seatbid.Bid[0].Crid)

	// Проверяем экстракцию значений
	extractor := processor.v25RespExtractor
	priceValue := extractor.ExtractFieldValue(FieldBidPrice, debugResponse)
	widthValue := extractor.ExtractFieldValue(FieldBidWidth, debugResponse)
	heightValue := extractor.ExtractFieldValue(FieldBidHeight, debugResponse)
	adomainValue := extractor.ExtractFieldValue(FieldBidAdomain, debugResponse)
	cridValue := extractor.ExtractFieldValue(FieldBidCRID, debugResponse)
	curValue := extractor.ExtractFieldValue(FieldBidResponseCur, debugResponse)

	t.Log("=== EXTRACTED VALUES ===")
	t.Logf("BidPrice: %+v", priceValue)
	t.Logf("BidWidth: %+v", widthValue)
	t.Logf("BidHeight: %+v", heightValue)
	t.Logf("BidAdomain: %+v", adomainValue)
	t.Logf("BidCRID: %+v", cridValue)
	t.Logf("BidResponseCur: %+v", curValue)

	tests := []struct {
		name     string
		sppURL   string
		response *ortb_V2_5.BidResponse
		expected bool
	}{
		{
			name:     "Positive case - should pass all SPP rules",
			sppURL:   "sppY",
			response: debugResponse,
			expected: true,
		},
		{
			name:   "Negative case - bid price too low",
			sppURL: "sppY",
			response: &ortb_V2_5.BidResponse{
				Cur: stringPtr("USD"),
				Seatbid: &ortb_V2_5.SeatBid{
					Bid: []*ortb_V2_5.Bid{
						{
							Price: floatPtr(0.3),
							W:     intPtr(800),
							H:     intPtr(600),
							Nurl:  stringPtr("https://tracker.example/win"),  // <— ДОБАВИТЬ
							Burl:  stringPtr("https://tracker.example/bill"), // <— ДОБАВИТЬ
						},
					},
				},
			},
			expected: false,
		},
		{
			name:   "Negative case - bid dimensions out of range",
			sppURL: "sppY",
			response: &ortb_V2_5.BidResponse{
				Cur: stringPtr("USD"),
				Seatbid: &ortb_V2_5.SeatBid{
					Bid: []*ortb_V2_5.Bid{
						{
							Price: floatPtr(4.5),
							W:     intPtr(50),
							H:     intPtr(1300),
							Nurl:  stringPtr("https://tracker.example/win"),  // <— ДОБАВИТЬ
							Burl:  stringPtr("https://tracker.example/bill"), // <— ДОБАВИТЬ
						},
					},
				},
			},
			expected: false,
		},
		{
			name:   "Negative case - bid price in excluded range",
			sppURL: "sppY",
			response: &ortb_V2_5.BidResponse{
				Cur: stringPtr("USD"),
				Seatbid: &ortb_V2_5.SeatBid{
					Bid: []*ortb_V2_5.Bid{
						{
							Price:   floatPtr(6.5),
							W:       intPtr(800),
							H:       intPtr(600),
							Adomain: []string{"example.com"},
							Crid:    stringPtr("creative123"),
							Nurl:    stringPtr("https://tracker.example/win"),  // <— ДОБАВИТЬ
							Burl:    stringPtr("https://tracker.example/bill"), // <— ДОБАВИТЬ
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.ProcessResponseForSPPV25(tt.sppURL, tt.response)
			if result.Allowed != tt.expected {
				t.Errorf("Test '%s': Expected allowed: %v, got: %v", tt.name, tt.expected, result.Allowed)

				// Дополнительная отладочная информация для неудачного теста
				if tt.name == "Positive case - should pass all SPP rules" {
					t.Log("=== FAILED POSITIVE CASE DEBUG ===")
					// Проверяем каждое поле отдельно
					extractor := processor.v25RespExtractor

					fieldsToCheck := []FieldType{
						FieldBidPrice,
						FieldBidWidth,
						FieldBidHeight,
						FieldBidAdomain,
						FieldBidCRID,
						FieldBidResponseCur,
					}

					for _, field := range fieldsToCheck {
						value := extractor.ExtractFieldValue(field, tt.response)
						t.Logf("Field %s: %+v", field, value)
					}
				}
			}
		})
	}
}

func TestExtractors(t *testing.T) {
	processor := NewOptimizedFilterProcessor(NewRuleManager())

	// Тестируем SPP экстрактор
	response := &ortb_V2_5.BidResponse{
		Cur: stringPtr("USD"),
		Seatbid: &ortb_V2_5.SeatBid{
			Bid: []*ortb_V2_5.Bid{
				{
					Price:   floatPtr(4.5),
					W:       intPtr(800),
					H:       intPtr(600),
					Adomain: []string{"example.com"},
					Crid:    stringPtr("creative123"),
					Nurl:    stringPtr("https://tracker.example/win"),  // <— ДОБАВИТЬ
					Burl:    stringPtr("https://tracker.example/bill"), // <— ДОБАВИТЬ
				},
			},
		},
	}

	extractor := processor.v25RespExtractor

	tests := []struct {
		field    FieldType
		expected interface{}
	}{
		{FieldBidPrice, float64(4.5)},
		{FieldBidWidth, 800},
		{FieldBidHeight, 600},
		{FieldBidAdomain, "example.com"},
		{FieldBidCRID, "creative123"},
		{FieldBidResponseCur, "USD"},
	}

	for _, tt := range tests {
		value := extractor.ExtractFieldValue(tt.field, response)
		t.Logf("Field %s: %+v (expected: %v)", tt.field, value, tt.expected)
	}
}
