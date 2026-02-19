package filter

/*
import (
	"os"
	"testing"

	ortb_V2_5 "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func TestDSPV25Rules(t *testing.T) {
	// Создаем временный файл с правилами
	rulesContent := `{
		"version": "1.0",
		"dsps": {
			"dspB": {
				"v25": [
					{
						"and": [
							{
								"or": [
									{ "field": "country", "condition": "equal", "value_type": "string", "value": "US" },
									{
										"and": [
											{ "field": "banner.w", "condition": "exists", "value_type": "int", "value": "" },
											{ "field": "banner.h", "condition": "exists", "value_type": "int", "value": "" }
										]
									},
									{
										"and": [
											{ "field": "native.request", "condition": "exists", "value_type": "string", "value": "" },
											{ "field": "native.ver", "condition": "exists", "value_type": "string", "value": "" }
										]
									}
								]
							},
							{
								"or": [
									{ "field": "tmax", "condition": "less_equal", "value_type": "int", "value": 120 },
									{ "field": "bidfloor", "condition": "greater_than", "value_type": "float", "value": 0.1 }
								]
							},
							{
								"and": [
									{ "field": "site.page", "condition": "exists", "value_type": "string", "value": "" },
									{ "field": "device.ua", "condition": "exists", "value_type": "string", "value": "" },
									{ "field": "user.id", "condition": "exists", "value_type": "string", "value": "" }
								]
							}
						]
					},
					{ "field": "banner.w", "condition": "between", "value_type": "int", "value": [300, 800] },
					{ "field": "bidfloor", "condition": "not_between", "value_type": "float", "value": [6.0, 8.0] }
				]
			}
		}
	}`

	tmpFile, err := os.CreateTemp("", "dsp_rules_*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(rulesContent); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// Инициализация менеджера правил и загрузчика
	ruleManager := NewRuleManager()
	loader := NewFileRuleLoader(ruleManager, tmpFile.Name(), tmpFile.Name(), "", "")

	err = loader.LoadDSPRules()
	if err != nil {
		t.Fatalf("Failed to load DSP rules: %v", err)
	}

	processor := NewOptimizedFilterProcessor(ruleManager)

	tests := []struct {
		name     string
		dspURL   string
		request  *ortb_V2_5.BidRequest
		expected bool
	}{
		{
			name:   "Positive case - should pass all rules",
			dspURL: "dspB",
			request: &ortb_V2_5.BidRequest{
				Device: &ortb_V2_5.Device{
					Geo: &ortb_V2_5.Geo{
						Country: stringPtr("US"),
					},
					Ua: stringPtr("Mozilla/5.0"),
				},
				Site: &ortb_V2_5.Site{
					Page: stringPtr("https://example.com"),
				},
				User: &ortb_V2_5.User{
					Id: stringPtr("user123"),
				},
				Tmax: intPtr(100),
				Imp: []*ortb_V2_5.Imp{
					{
						Banner: &ortb_V2_5.Banner{
							W: intPtr(500),
							H: intPtr(600),
						},
						BidFloor: floatPtr(0.5),
					},
				},
			},
			expected: true,
		},
		{
			name:   "Negative case - country not US and no banner dimensions",
			dspURL: "dspB",
			request: &ortb_V2_5.BidRequest{
				Device: &ortb_V2_5.Device{
					Geo: &ortb_V2_5.Geo{
						Country: stringPtr("CA"),
					},
					Ua: stringPtr("Mozilla/5.0"),
				},
				Site: &ortb_V2_5.Site{
					Page: stringPtr("https://example.com"),
				},
				User: &ortb_V2_5.User{
					Id: stringPtr("user123"),
				},
				Tmax: intPtr(150), // больше 120
				Imp: []*ortb_V2_5.Imp{
					{
						BidFloor: floatPtr(0.05), // меньше 0.1
					},
				},
			},
			expected: false,
		},
		{
			name:   "Negative case - banner width out of range",
			dspURL: "dspB",
			request: &ortb_V2_5.BidRequest{
				Device: &ortb_V2_5.Device{
					Geo: &ortb_V2_5.Geo{
						Country: stringPtr("US"),
					},
					Ua: stringPtr("Mozilla/5.0"),
				},
				Site: &ortb_V2_5.Site{
					Page: stringPtr("https://example.com"),
				},
				User: &ortb_V2_5.User{
					Id: stringPtr("user123"),
				},
				Tmax: intPtr(100),
				Imp: []*ortb_V2_5.Imp{
					{
						Banner: &ortb_V2_5.Banner{
							W: intPtr(200), // вне диапазона 300-800
							H: intPtr(600),
						},
						BidFloor: floatPtr(0.5),
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.ProcessRequestForDSPV25(tt.dspURL, tt.request)
			if result.Allowed != tt.expected {
				t.Errorf("Expected allowed: %v, got: %v", tt.expected, result.Allowed)
			}
		})
	}
}

// Вспомогательные функции для создания указателей
func stringPtr(s string) *string  { return &s }
func intPtr(i int32) *int32       { return &i }
func floatPtr(f float32) *float32 { return &f }
*/
