package utils

import (
	"reflect"
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
)

/*func TestDspMap(t *testing.T) {
	testMap := map[string]map[string]map[string]*types.PercentAndBidfloor{
		"ssp1": {
			"US": {
				"LEFT": {Percent: 0.10, Bidfloor: false},
			},
			"FR": {
				"dsp1": {Percent: 0.20, Bidfloor: false},
				"dsp2": {Percent: 0.25, Bidfloor: false},
			},
			"UA": {
				"dsp1": {Percent: 0.08, Bidfloor: false},
				"LEFT": {Percent: 0.03, Bidfloor: false},
				"ALL":  {Percent: 0.50, Bidfloor: false},
			},
			"ALL": {
				"LEFT": {Percent: 0.03, Bidfloor: false},
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
	}{
		{
			name:            "ALL case",
			ssp:             "ssp1",
			geo:             "UA",
			dsp:             "dsp2",
			expectedPercent: 0.50,
		},
		{
			name:            "LEFT case",
			ssp:             "ssp1",
			geo:             "US",
			dsp:             "dsp1",
			expectedPercent: 0.10,
		},
		{
			name:            "Exact dsp2 case",
			ssp:             "ssp1",
			geo:             "FR",
			dsp:             "dsp2",
			expectedPercent: 0.25,
		},
		{
			name:            "uknown case",
			ssp:             "ssp1",
			geo:             "FR",
			dsp:             "unknown_dsp",
			expectedPercent: 0.02,
		},
		{
			name:            "only LEFT exist",
			ssp:             "ssp1",
			geo:             "KZ",
			dsp:             "new_dsp",
			expectedPercent: 0.03,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPercent(tt.ssp, tt.geo, tt.dsp, testMap, defaultPercent)
			if result.Percent != tt.expectedPercent {
				t.Errorf("%s: expected %.3f, got %.3f",
					tt.name, tt.expectedPercent, result.Percent)
			}
		})
	}
}*/

func TestGeoMap(t *testing.T) {
	testMap := map[string]map[string]map[string]*types.PercentAndBidfloor{
		"ssp1": {
			"US, RU,GZ": {
				"dsp_dao.ad": {Percent: 0.10, Bidfloor: false},
			},
			"FR": {
				"dsp1": {Percent: 0.20, Bidfloor: false},
				"dsp2": {Percent: 0.25, Bidfloor: false},
			},
			"ALL": {
				"dsp_hilltopads.com": {Percent: 0.03, Bidfloor: false},
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
	}{
		{
			name:            "ALL case hilltop",
			ssp:             "ssp1",
			geo:             "US",
			dsp:             "dsp_hilltopads.com",
			expectedPercent: 0.03,
		},
		/*		{
					name:            "LEFT case",
					ssp:             "ssp1",
					geo:             "US",
					dsp:             "dsp1",
					expectedPercent: 0.10,
				},
				{
					name:            "Exact dsp2 case",
					ssp:             "ssp1",
					geo:             "FR",
					dsp:             "dsp2",
					expectedPercent: 0.25,
				},
				{
					name:            "uknown case",
					ssp:             "ssp1",
					geo:             "FR",
					dsp:             "unknown_dsp",
					expectedPercent: 0.02,
				},
				{
					name:            "only LEFT exist",
					ssp:             "ssp1",
					geo:             "KZ",
					dsp:             "new_dsp",
					expectedPercent: 0.03,
				},*/
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetValueFomSspGeoDspMap(tt.ssp, tt.geo, tt.dsp, testMap, &types.PercentAndBidfloor{
				Percent:  defaultPercent,
				Bidfloor: false,
			})
			if result.Percent != tt.expectedPercent {
				t.Errorf("%s: expected %.3f, got %.3f",
					tt.name, tt.expectedPercent, result.Percent)
			}
		})
	}
}

func Test_SetAndConvertNonGoodMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]map[string]map[string]*types.PercentAndBidfloor
		expected map[string]map[string]map[string]*types.PercentAndBidfloor
	}{
		{
			name: "Simple single keys",
			input: map[string]map[string]map[string]*types.PercentAndBidfloor{
				"ssp1": {
					"US": {
						"dsp1": {Percent: 0.15, Bidfloor: false},
					},
				},
			},
			expected: map[string]map[string]map[string]*types.PercentAndBidfloor{
				"ssp1": {
					"US": {
						"dsp1": {Percent: 0.15, Bidfloor: false},
					},
				},
			},
		},
		{
			name: "Multiple SSP keys with comma",
			input: map[string]map[string]map[string]*types.PercentAndBidfloor{
				"ssp1, ssp2": {
					"US": {
						"dsp1": {Percent: 0.15, Bidfloor: false},
					},
				},
			},
			expected: map[string]map[string]map[string]*types.PercentAndBidfloor{
				"ssp1": {
					"US": {
						"dsp1": {Percent: 0.15, Bidfloor: false},
					},
				},
				"ssp2": {
					"US": {
						"dsp1": {Percent: 0.15, Bidfloor: false},
					},
				},
			},
		},
		{
			name: "Multiple GEO keys with comma",
			input: map[string]map[string]map[string]*types.PercentAndBidfloor{
				"ssp1": {
					"US, CA": {
						"dsp1": {Percent: 0.15, Bidfloor: false},
					},
				},
			},
			expected: map[string]map[string]map[string]*types.PercentAndBidfloor{
				"ssp1": {
					"US": {
						"dsp1": {Percent: 0.15, Bidfloor: false},
					},
					"CA": {
						"dsp1": {Percent: 0.15, Bidfloor: false},
					},
				},
			},
		},
		{
			name: "Multiple DSP keys with comma",
			input: map[string]map[string]map[string]*types.PercentAndBidfloor{
				"ssp1": {
					"US": {
						"dsp1, dsp2": {Percent: 0.15, Bidfloor: false},
					},
				},
			},
			expected: map[string]map[string]map[string]*types.PercentAndBidfloor{
				"ssp1": {
					"US": {
						"dsp1": {Percent: 0.15, Bidfloor: false},
						"dsp2": {Percent: 0.15, Bidfloor: false},
					},
				},
			},
		},
		{
			name: "All levels with multiple keys",
			input: map[string]map[string]map[string]*types.PercentAndBidfloor{
				"ssp1, ssp2": {
					"US, CA": {
						"dsp1, dsp2": {Percent: 0.15, Bidfloor: false},
					},
				},
			},
			expected: map[string]map[string]map[string]*types.PercentAndBidfloor{
				"ssp1": {
					"US": {
						"dsp1": {Percent: 0.15, Bidfloor: false},
						"dsp2": {Percent: 0.15, Bidfloor: false},
					},
					"CA": {
						"dsp1": {Percent: 0.15, Bidfloor: false},
						"dsp2": {Percent: 0.15, Bidfloor: false},
					},
				},
				"ssp2": {
					"US": {
						"dsp1": {Percent: 0.15, Bidfloor: false},
						"dsp2": {Percent: 0.15, Bidfloor: false},
					},
					"CA": {
						"dsp1": {Percent: 0.15, Bidfloor: false},
						"dsp2": {Percent: 0.15, Bidfloor: false},
					},
				},
			},
		},
		{
			name: "Keys with spaces and empty elements",
			input: map[string]map[string]map[string]*types.PercentAndBidfloor{
				"ssp1, , ssp2": {
					"US , , CA": {
						"dsp1, , dsp2": {Percent: 0.15, Bidfloor: false},
					},
				},
			},
			expected: map[string]map[string]map[string]*types.PercentAndBidfloor{
				"ssp1": {
					"US": {
						"dsp1": {Percent: 0.15, Bidfloor: false},
						"dsp2": {Percent: 0.15, Bidfloor: false},
					},
					"CA": {
						"dsp1": {Percent: 0.15, Bidfloor: false},
						"dsp2": {Percent: 0.15, Bidfloor: false},
					},
				},
				"ssp2": {
					"US": {
						"dsp1": {Percent: 0.15, Bidfloor: false},
						"dsp2": {Percent: 0.15, Bidfloor: false},
					},
					"CA": {
						"dsp1": {Percent: 0.15, Bidfloor: false},
						"dsp2": {Percent: 0.15, Bidfloor: false},
					},
				},
			},
		},
		{
			name: "Mixed single and multiple keys",
			input: map[string]map[string]map[string]*types.PercentAndBidfloor{
				"ssp1": {
					"US, CA": {
						"dsp1": {Percent: 0.15, Bidfloor: false},
					},
				},
				"ssp2, ssp3": {
					"US": {
						"dsp1, dsp2": {Percent: 0.20, Bidfloor: false},
					},
				},
			},
			expected: map[string]map[string]map[string]*types.PercentAndBidfloor{
				"ssp1": {
					"US": {
						"dsp1": {Percent: 0.15, Bidfloor: false},
					},
					"CA": {
						"dsp1": {Percent: 0.15, Bidfloor: false},
					},
				},
				"ssp2": {
					"US": {
						"dsp1": {Percent: 0.20, Bidfloor: false},
						"dsp2": {Percent: 0.20, Bidfloor: false},
					},
				},
				"ssp3": {
					"US": {
						"dsp1": {Percent: 0.20, Bidfloor: false},
						"dsp2": {Percent: 0.20, Bidfloor: false},
					},
				},
			},
		},
		{
			name: "Special values ANY and LEFT",
			input: map[string]map[string]map[string]*types.PercentAndBidfloor{
				"ANY": {
					"US, CA": {
						"dsp1, LEFT": {Percent: 0.10, Bidfloor: false},
					},
				},
			},
			expected: map[string]map[string]map[string]*types.PercentAndBidfloor{
				"ANY": {
					"US": {
						"dsp1": {Percent: 0.10, Bidfloor: false},
						"LEFT": {Percent: 0.10, Bidfloor: false},
					},
					"CA": {
						"dsp1": {Percent: 0.10, Bidfloor: false},
						"LEFT": {Percent: 0.10, Bidfloor: false},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SetAndConvertNonGoodMap(tt.input)

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Test %s failed:\nExpected: %+v\nGot:      %+v",
					tt.name, tt.expected, result)
			}
		})
	}
}

func TestSplitAndTrimKeys(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"", []string{""}},
		{"key1", []string{"key1"}},
		{"key1,key2", []string{"key1", "key2"}},
		{"key1, key2", []string{"key1", "key2"}},
		{"key1 , key2 , key3", []string{"key1", "key2", "key3"}},
		{"key1, ,key2", []string{"key1", "key2"}},
		{" , key1, , key2, ", []string{"key1", "key2"}},
		{"  key1  ,  key2  ", []string{"key1", "key2"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := splitAndTrimKeys(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("splitAndTrimKeys(%q) = %v, expected %v",
					tt.input, result, tt.expected)
			}
		})
	}
}
