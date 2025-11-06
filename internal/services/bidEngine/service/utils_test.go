package bidEngine

import (
	"fmt"
	"testing"
)

func TestGetGeoDspPercent(t *testing.T) {
	testMap := map[string]map[string]map[string]float32{
		"ssp1": {
			"US": {
				"dsp1": 0.15,
				"ANY":  0.10,
				"LEFT": 0.05,
			},
			"FR": {
				"dsp1": 0.20,
				"dsp2": 0.25,
			},
			"ANY": {
				"dsp1": 0.08,
				"ANY":  0.03,
			},
		},
		"ssp2": {
			"DE": {
				"ANY": 0.12,
			},
		},
		"ANY": {
			"ANY": {
				"ANY": 0.01,
			},
		},
	}

	defaultPercent := float32(0.02)

	tests := []struct {
		name            string
		ssp             string
		geo             string
		dsp             string
		expectedPercent float32
		description     string
	}{
		// Точные совпадения
		{
			name:            "Exact match all levels",
			ssp:             "ssp1",
			geo:             "US",
			dsp:             "dsp1",
			expectedPercent: 0.15,
			description:     "Точное совпадение SSP→GEO→DSP",
		},
		{
			name:            "Exact match different values",
			ssp:             "ssp1",
			geo:             "FR",
			dsp:             "dsp2",
			expectedPercent: 0.25,
			description:     "Точное совпадение с другими значениями",
		},

		// ANY на DSP уровне
		{
			name:            "ANY on DSP level",
			ssp:             "ssp1",
			geo:             "US",
			dsp:             "dsp999",
			expectedPercent: 0.10,
			description:     "ANY для неизвестного DSP",
		},
		{
			name:            "ANY on DSP with different GEO",
			ssp:             "ssp2",
			geo:             "DE",
			dsp:             "unknown_dsp",
			expectedPercent: 0.12,
			description:     "ANY для DSP в другом GEO",
		},

		// LEFT на DSP уровне
		{
			name:            "LEFT on DSP level",
			ssp:             "ssp1",
			geo:             "US",
			dsp:             "new_dsp",
			expectedPercent: 0.05,
			description:     "LEFT для нового DSP (после ANY)",
		},

		// ANY на GEO уровне
		{
			name:            "ANY on GEO level",
			ssp:             "ssp1",
			geo:             "UnknownCountry",
			dsp:             "dsp1",
			expectedPercent: 0.08,
			description:     "ANY для GEO с конкретным DSP",
		},
		{
			name:            "ANY on GEO and DSP levels",
			ssp:             "ssp1",
			geo:             "UnknownCountry",
			dsp:             "unknown_dsp",
			expectedPercent: 0.03,
			description:     "ANY для GEO и ANY для DSP",
		},

		// ANY на SSP уровне
		{
			name:            "ANY on SSP level",
			ssp:             "unknown_ssp",
			geo:             "US",
			dsp:             "dsp1",
			expectedPercent: 0.08, // Из ssp1→ANY→dsp1, а не ANY→ANY→ANY
			description:     "ANY для SSP с конкретными GEO и DSP",
		},
		{
			name:            "ANY on SSP and GEO levels",
			ssp:             "unknown_ssp",
			geo:             "UnknownCountry",
			dsp:             "dsp1",
			expectedPercent: 0.08, // Приоритет: ssp1→ANY→dsp1
			description:     "ANY для SSP и GEO с конкретным DSP",
		},

		// Полное ANY
		{
			name:            "Full ANY match",
			ssp:             "unknown_ssp",
			geo:             "unknown_geo",
			dsp:             "unknown_dsp",
			expectedPercent: 0.01,
			description:     "Полное ANY на всех уровнях",
		},

		// Процент по умолчанию
		{
			name:            "Default percent when no match",
			ssp:             "non_existent_ssp",
			geo:             "non_existent_geo",
			dsp:             "non_existent_dsp",
			expectedPercent: 0.02,
			description:     "Возврат процента по умолчанию когда нет совпадений",
		},

		// Критичные кейсы с пустыми строками
		{
			name:            "Empty SSP",
			ssp:             "",
			geo:             "US",
			dsp:             "dsp1",
			expectedPercent: 0.08, // ANY для SSP
			description:     "Пустой SSP должен использовать ANY",
		},
		{
			name:            "Empty GEO",
			ssp:             "ssp1",
			geo:             "",
			dsp:             "dsp1",
			expectedPercent: 0.08, // ANY для GEO
			description:     "Пустой GEO должен использовать ANY",
		},
		{
			name:            "Empty DSP",
			ssp:             "ssp1",
			geo:             "US",
			dsp:             "",
			expectedPercent: 0.10, // ANY для DSP
			description:     "Пустой DSP должен использовать ANY",
		},

		// Кейсы с приоритетами
		{
			name:            "Priority exact over ANY",
			ssp:             "ssp1",
			geo:             "US",
			dsp:             "dsp1",
			expectedPercent: 0.15, // Точное совпадение, а не ANY (0.10)
			description:     "Приоритет точного совпадения над ANY",
		},
		{
			name:            "Priority ANY over LEFT",
			ssp:             "ssp1",
			geo:             "US",
			dsp:             "some_dsp",
			expectedPercent: 0.10, // ANY, а не LEFT (0.05)
			description:     "Приоритет ANY над LEFT на DSP уровне",
		},

		// Edge cases
		{
			name:            "Non-existent SSP but ANY exists",
			ssp:             "ssp999",
			geo:             "US",
			dsp:             "dsp1",
			expectedPercent: 0.08,
			description:     "Несуществующий SSP с ANY на GEO уровне",
		},
		{
			name:            "Only default available",
			ssp:             "no_ssp",
			geo:             "no_geo",
			dsp:             "no_dsp",
			expectedPercent: 0.02,
			description:     "Только процент по умолчанию",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetGeoDspPercent(tt.ssp, tt.geo, tt.dsp, testMap, defaultPercent)
			if result != tt.expectedPercent {
				t.Errorf("%s: expected %.3f, got %.3f - %s",
					tt.name, tt.expectedPercent, result, tt.description)
			}
		})
	}
}

func TestGetGeoDspPercent_EdgeCases(t *testing.T) {
	defaultPercent := float32(0.02)

	// Тест с nil мапой
	t.Run("Nil map returns default", func(t *testing.T) {
		result := GetGeoDspPercent("ssp1", "US", "dsp1", nil, defaultPercent)
		if result != defaultPercent {
			t.Errorf("Expected default percent for nil map, got %.3f", result)
		}
	})

	// Тест с пустой мапой
	t.Run("Empty map returns default", func(t *testing.T) {
		emptyMap := map[string]map[string]map[string]float32{}
		result := GetGeoDspPercent("ssp1", "US", "dsp1", emptyMap, defaultPercent)
		if result != defaultPercent {
			t.Errorf("Expected default percent for empty map, got %.3f", result)
		}
	})

	// Тест с частично заполненной мапой
	t.Run("Partial map with missing levels", func(t *testing.T) {
		partialMap := map[string]map[string]map[string]float32{
			"ssp1": {
				"US": {
					"dsp1": 0.15,
					// Нет ANY или LEFT
				},
			},
		}

		// Точное совпадение должно работать
		result := GetGeoDspPercent("ssp1", "US", "dsp1", partialMap, defaultPercent)
		if result != 0.15 {
			t.Errorf("Expected 0.15 for exact match, got %.3f", result)
		}

		// Неизвестный DSP должен вернуть default
		result = GetGeoDspPercent("ssp1", "US", "unknown_dsp", partialMap, defaultPercent)
		if result != defaultPercent {
			t.Errorf("Expected default for unknown DSP, got %.3f", result)
		}
	})

	// Тест с отрицательными процентами
	t.Run("Negative percentages", func(t *testing.T) {
		negativeMap := map[string]map[string]map[string]float32{
			"ssp1": {
				"US": {
					"dsp1": -0.10, // Отрицательный процент
					"ANY":  -0.05,
				},
			},
		}

		result := GetGeoDspPercent("ssp1", "US", "dsp1", negativeMap, defaultPercent)
		if result != -0.10 {
			t.Errorf("Expected -0.10 for negative percent, got %.3f", result)
		}
	})

	// Тест с очень большими процентами
	t.Run("Large percentages", func(t *testing.T) {
		largeMap := map[string]map[string]map[string]float32{
			"ssp1": {
				"US": {
					"dsp1": 5.0, // 500%
				},
			},
		}

		result := GetGeoDspPercent("ssp1", "US", "dsp1", largeMap, defaultPercent)
		if result != 5.0 {
			t.Errorf("Expected 5.0 for large percent, got %.3f", result)
		}
	})
}

func TestGetGeoDspPercent_PriorityOrder(t *testing.T) {
	// Тест для проверки правильного порядка приоритетов
	priorityMap := map[string]map[string]map[string]float32{
		"ssp1": {
			"US": {
				"dsp1": 0.15, // 1. Точное совпадение
				"ANY":  0.10, // 2. ANY для DSP
				"LEFT": 0.05, // 3. LEFT для DSP
			},
			"ANY": {
				"dsp1": 0.08, // 4. ANY для GEO
				"ANY":  0.03, // 5. ANY для GEO+DSP
			},
		},
		"ANY": {
			"US": {
				"dsp1": 0.07, // 6. ANY для SSP
			},
			"ANY": {
				"dsp1": 0.06, // 7. ANY для SSP+GEO
				"ANY":  0.01, // 8. Полное ANY
			},
		},
	}

	defaultPercent := float32(0.02)

	tests := []struct {
		ssp             string
		geo             string
		dsp             string
		expectedPercent float32
		expectedOrder   int
	}{
		{"ssp1", "US", "dsp1", 0.15, 1},                // Точное совпадение
		{"ssp1", "US", "other_dsp", 0.10, 2},           // ANY для DSP
		{"ssp1", "US", "new_dsp", 0.05, 3},             // LEFT для DSP
		{"ssp1", "OtherGeo", "dsp1", 0.08, 4},          // ANY для GEO
		{"ssp1", "OtherGeo", "other_dsp", 0.03, 5},     // ANY для GEO+DSP
		{"OtherSSP", "US", "dsp1", 0.07, 6},            // ANY для SSP
		{"OtherSSP", "OtherGeo", "dsp1", 0.06, 7},      // ANY для SSP+GEO
		{"OtherSSP", "OtherGeo", "other_dsp", 0.01, 8}, // Полное ANY
		{"NoMatch", "NoMatch", "NoMatch", 0.02, 9},     // По умолчанию
	}

	for i, tt := range tests {
		result := GetGeoDspPercent(tt.ssp, tt.geo, tt.dsp, priorityMap, defaultPercent)
		if result != tt.expectedPercent {
			t.Errorf("Priority test %d (order %d): expected %.3f, got %.3f for %s/%s/%s",
				i+1, tt.expectedOrder, tt.expectedPercent, result, tt.ssp, tt.geo, tt.dsp)
		}
	}
}

func TestGetGeoDspPercent_MinimalMaps(t *testing.T) {
	defaultPercent := float32(0.02)

	// 1. Тесты с минимальными мапами
	t.Run("Only exact matches no wildcards", func(t *testing.T) {
		minimalMap := map[string]map[string]map[string]float32{
			"ssp1": {
				"US": {"dsp1": 0.15},
			},
			"ssp2": {
				"FR": {"dsp2": 0.20},
			},
		}

		tests := []struct {
			ssp             string
			geo             string
			dsp             string
			expectedPercent float32
		}{
			{"ssp1", "US", "dsp1", 0.15},
			{"ssp1", "US", "dsp2", defaultPercent}, // Нет wildcards
			{"ssp3", "US", "dsp1", defaultPercent},
		}

		for _, tt := range tests {
			result := GetGeoDspPercent(tt.ssp, tt.geo, tt.dsp, minimalMap, defaultPercent)
			if result != tt.expectedPercent {
				t.Errorf("Minimal map %s/%s/%s: expected %.3f, got %.3f",
					tt.ssp, tt.geo, tt.dsp, tt.expectedPercent, result)
			}
		}
	})

	// 2. Тесты только с ANY на всех уровнях
	t.Run("Only ANY wildcards", func(t *testing.T) {
		anyOnlyMap := map[string]map[string]map[string]float32{
			"ANY": {
				"ANY": {
					"ANY": 0.10,
				},
			},
		}

		tests := []struct {
			ssp             string
			geo             string
			dsp             string
			expectedPercent float32
		}{
			{"any_ssp", "any_geo", "any_dsp", 0.10},
			{"ssp1", "US", "dsp1", 0.10},
			{"", "", "", 0.10}, // Все пустые
		}

		for _, tt := range tests {
			result := GetGeoDspPercent(tt.ssp, tt.geo, tt.dsp, anyOnlyMap, defaultPercent)
			if result != tt.expectedPercent {
				t.Errorf("ANY only %s/%s/%s: expected %.3f, got %.3f",
					tt.ssp, tt.geo, tt.dsp, tt.expectedPercent, result)
			}
		}
	})

	// 3. Тесты только с LEFT на DSP уровне
	t.Run("Only LEFT on DSP level", func(t *testing.T) {
		leftOnlyMap := map[string]map[string]map[string]float32{
			"ssp1": {
				"US": {
					"LEFT": 0.05,
				},
			},
		}

		tests := []struct {
			ssp             string
			geo             string
			dsp             string
			expectedPercent float32
		}{
			{"ssp1", "US", "new_dsp1", 0.05},
			{"ssp1", "US", "new_dsp2", 0.05},
			{"ssp1", "US", "dsp1", 0.05},           // Любой DSP попадает под LEFT
			{"ssp1", "FR", "dsp1", defaultPercent}, // Другое GEO
		}

		for _, tt := range tests {
			result := GetGeoDspPercent(tt.ssp, tt.geo, tt.dsp, leftOnlyMap, defaultPercent)
			if result != tt.expectedPercent {
				t.Errorf("LEFT only %s/%s/%s: expected %.3f, got %.3f",
					tt.ssp, tt.geo, tt.dsp, tt.expectedPercent, result)
			}
		}
	})
}

func TestGetGeoDspPercent_ConflictAndNested(t *testing.T) {
	defaultPercent := float32(0.02)

	// 4. Тесты с конфликтующими wildcards
	t.Run("Conflicting wildcards same level", func(t *testing.T) {
		conflictMap := map[string]map[string]map[string]float32{
			"ssp1": {
				"US": {
					"dsp1": 0.15,
					"ANY":  0.10,
					"LEFT": 0.05,
				},
			},
		}

		tests := []struct {
			ssp             string
			geo             string
			dsp             string
			expectedPercent float32
		}{
			{"ssp1", "US", "dsp1", 0.15}, // Приоритет точному
			{"ssp1", "US", "dsp2", 0.10}, // ANY перед LEFT
			{"ssp1", "US", "dsp3", 0.10}, // ANY все равно приоритетнее
		}

		for _, tt := range tests {
			result := GetGeoDspPercent(tt.ssp, tt.geo, tt.dsp, conflictMap, defaultPercent)
			if result != tt.expectedPercent {
				t.Errorf("Conflict %s/%s/%s: expected %.3f, got %.3f",
					tt.ssp, tt.geo, tt.dsp, tt.expectedPercent, result)
			}
		}
	})

	// 5. Тесты с вложенными ANY
	t.Run("Nested ANY combinations", func(t *testing.T) {
		nestedAnyMap := map[string]map[string]map[string]float32{
			"ssp1": {
				"US":  {"ANY": 0.10},
				"ANY": {"dsp1": 0.08},
			},
			"ANY": {
				"US": {"ANY": 0.05},
			},
		}

		tests := []struct {
			ssp             string
			geo             string
			dsp             string
			expectedPercent float32
		}{
			{"ssp1", "US", "dsp2", 0.10}, // ssp1→US→ANY
			{"ssp1", "FR", "dsp1", 0.08}, // ssp1→ANY→dsp1
			{"ssp2", "US", "dsp2", 0.05}, // ANY→US→ANY
			{"ssp2", "FR", "dsp1", defaultPercent},
		}

		for _, tt := range tests {
			result := GetGeoDspPercent(tt.ssp, tt.geo, tt.dsp, nestedAnyMap, defaultPercent)
			if result != tt.expectedPercent {
				t.Errorf("Nested ANY %s/%s/%s: expected %.3f, got %.3f",
					tt.ssp, tt.geo, tt.dsp, tt.expectedPercent, result)
			}
		}
	})
}

func TestGetGeoDspPercent_SpecialValues(t *testing.T) {
	defaultPercent := float32(0.02)

	// 6. Тесты с специальными значениями
	t.Run("Zero and extreme values", func(t *testing.T) {
		specialMap := map[string]map[string]map[string]float32{
			"ssp1": {
				"US": {
					"dsp1": 0.0,   // Нулевой процент
					"dsp2": -0.10, // Отрицательный
					"dsp3": 1.5,   // >100%
					"dsp4": 100.0, // 10000%
				},
			},
		}

		tests := []struct {
			ssp             string
			geo             string
			dsp             string
			expectedPercent float32
		}{
			{"ssp1", "US", "dsp1", 0.0},
			{"ssp1", "US", "dsp2", -0.10},
			{"ssp1", "US", "dsp3", 1.5},
			{"ssp1", "US", "dsp4", 100.0},
		}

		for _, tt := range tests {
			result := GetGeoDspPercent(tt.ssp, tt.geo, tt.dsp, specialMap, defaultPercent)
			if result != tt.expectedPercent {
				t.Errorf("Special values %s/%s/%s: expected %.3f, got %.3f",
					tt.ssp, tt.geo, tt.dsp, tt.expectedPercent, result)
			}
		}
	})

	// 7. Тесты с case sensitivity
	t.Run("Case sensitivity", func(t *testing.T) {
		caseMap := map[string]map[string]map[string]float32{
			"SSP1": {"US": {"DSP1": 0.15}},
			"ssp1": {"us": {"dsp1": 0.10}},
		}

		tests := []struct {
			ssp             string
			geo             string
			dsp             string
			expectedPercent float32
		}{
			{"SSP1", "US", "DSP1", 0.15},
			{"ssp1", "us", "dsp1", 0.10},
			{"Ssp1", "Us", "Dsp1", defaultPercent}, // Разный case
		}

		for _, tt := range tests {
			result := GetGeoDspPercent(tt.ssp, tt.geo, tt.dsp, caseMap, defaultPercent)
			if result != tt.expectedPercent {
				t.Errorf("Case sensitivity %s/%s/%s: expected %.3f, got %.3f",
					tt.ssp, tt.geo, tt.dsp, tt.expectedPercent, result)
			}
		}
	})
}

func TestGetGeoDspPercent_SpecialCharacters(t *testing.T) {
	defaultPercent := float32(0.02)

	// 9. Тесты с пробелами и спецсимволами
	t.Run("Special characters in keys", func(t *testing.T) {
		specialCharMap := map[string]map[string]map[string]float32{
			"ssp-1": {"US-NY": {"dsp_1": 0.15}},
			"ssp.2": {"FR/Paris": {"dsp-2": 0.20}},
			"ssp 3": {"GB London": {"dsp 3": 0.25}},
		}

		tests := []struct {
			ssp             string
			geo             string
			dsp             string
			expectedPercent float32
		}{
			{"ssp-1", "US-NY", "dsp_1", 0.15},
			{"ssp.2", "FR/Paris", "dsp-2", 0.20},
			{"ssp 3", "GB London", "dsp 3", 0.25},
		}

		for _, tt := range tests {
			result := GetGeoDspPercent(tt.ssp, tt.geo, tt.dsp, specialCharMap, defaultPercent)
			if result != tt.expectedPercent {
				t.Errorf("Special chars %s/%s/%s: expected %.3f, got %.3f",
					tt.ssp, tt.geo, tt.dsp, tt.expectedPercent, result)
			}
		}
	})
}

func TestGetGeoDspPercent_OverlappingRules(t *testing.T) {
	defaultPercent := float32(0.02)

	// 11. Тесты с перекрывающимися правилами
	t.Run("Overlapping rules", func(t *testing.T) {
		overlapMap := map[string]map[string]map[string]float32{
			"ssp1": {
				"US":  {"ANY": 0.10},
				"ANY": {"ANY": 0.05},
			},
			"ANY": {
				"US":  {"ANY": 0.08},
				"ANY": {"ANY": 0.03},
			},
		}

		tests := []struct {
			ssp             string
			geo             string
			dsp             string
			expectedPercent float32
		}{
			{"ssp1", "US", "dsp1", 0.10}, // ssp1→US→ANY
			{"ssp1", "FR", "dsp1", 0.05}, // ssp1→ANY→ANY
			{"ssp2", "US", "dsp1", 0.08}, // ANY→US→ANY
			{"ssp2", "FR", "dsp1", 0.03}, // ANY→ANY→ANY
		}

		for _, tt := range tests {
			result := GetGeoDspPercent(tt.ssp, tt.geo, tt.dsp, overlapMap, defaultPercent)
			if result != tt.expectedPercent {
				t.Errorf("Overlap %s/%s/%s: expected %.3f, got %.3f",
					tt.ssp, tt.geo, tt.dsp, tt.expectedPercent, result)
			}
		}
	})
}

func TestGetGeoDspPercent_Performance(t *testing.T) {
	defaultPercent := float32(0.02)

	// 10. Тесты производительности с большими мапами
	t.Run("Large map performance", func(t *testing.T) {
		largeMap := map[string]map[string]map[string]float32{}

		// Создаем большую мапу с 50 SSP
		for i := 1; i <= 50; i++ {
			sspKey := fmt.Sprintf("ssp%d", i)
			largeMap[sspKey] = map[string]map[string]float32{
				"US": {
					"dsp1": float32(0.15 + 0.01*float32(i)), // Уникальные значения
					"ANY":  float32(0.10 + 0.01*float32(i)),
				},
			}
		}
		largeMap["ANY"] = map[string]map[string]float32{
			"ANY": {"ANY": 0.01},
		}

		tests := []struct {
			ssp             string
			geo             string
			dsp             string
			expectedPercent float32
		}{
			{"ssp50", "US", "dsp1", 0.65},           // 0.15 + 0.01*50
			{"unknown", "unknown", "unknown", 0.01}, // Должен быстро найти ANY
		}

		for _, tt := range tests {
			result := GetGeoDspPercent(tt.ssp, tt.geo, tt.dsp, largeMap, defaultPercent)
			if result != tt.expectedPercent {
				t.Errorf("Large map %s/%s/%s: expected %.3f, got %.3f",
					tt.ssp, tt.geo, tt.dsp, tt.expectedPercent, result)
			}
		}
	})
}
