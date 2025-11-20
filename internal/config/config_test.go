package config

/*
func TestMapStringToString_SetValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected MapStringToString
	}{
		{
			name:  "URL with equals in value",
			input: "kadam.net=http://pop-48702.daortb.com/api/rtb-pops/item?sourceId=59738&api-key=xvKZ-_oewvADCb2RR0W6bgp_EdLEKCLj,clickadilla.com=http://pop.zog.link/bid-request?token=h6dKfdh544FHD83",
			expected: MapStringToString{
				"kadam.net":       "http://pop-48702.daortb.com/api/rtb-pops/item?sourceId=59738&api-key=xvKZ-_oewvADCb2RR0W6bgp_EdLEKCLj",
				"clickadilla.com": "http://pop.zog.link/bid-request?token=h6dKfdh544FHD83",
			},
		},
		{
			name:  "Multiple pairs with equals in values",
			input: "key1=value1=with=equals,key2=http://example.com?param1=val1&param2=val2,key3=simple",
			expected: MapStringToString{
				"key1": "value1=with=equals",
				"key2": "http://example.com?param1=val1&param2=val2",
				"key3": "simple",
			},
		},
		{
			name:     "Empty input",
			input:    "",
			expected: MapStringToString{},
		},
		{
			name:  "Invalid pair without equals",
			input: "key1=value1,key2withoutEquals,key3=value3",
			expected: MapStringToString{
				"key1": "value1",
				"key3": "value3",
			},
		},
		{
			name:  "With spaces",
			input: " key1 = value1=with=equals , key2 = http://example.com?p1=v1 ",
			expected: MapStringToString{
				"key1": "value1=with=equals",
				"key2": "http://example.com?p1=v1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m MapStringToString
			err := m.SetValue(tt.input)
			if err != nil {
				t.Errorf("SetValue() error = %v", err)
				return
			}
			if !reflect.DeepEqual(m, tt.expected) {
				t.Errorf("SetValue() got = %v, want %v", m, tt.expected)
			}
		})
	}
}

func TestMapStringToStringSlice_SetValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected MapStringToStringSlice
	}{
		{
			name:  "Multiple URLs with equals in values",
			input: "kadam.net=http://pop-48702.daortb.com/api/rtb-pops/item?sourceId=59738&api-key=xvKZ-_oewvADCb2RR0W6bgp_EdLEKCLj|http://backup.com?param=value,clickadilla.com=http://pop.zog.link/bid-request?token=h6dKfdh544FHD83",
			expected: MapStringToStringSlice{
				"kadam.net": {
					"http://pop-48702.daortb.com/api/rtb-pops/item?sourceId=59738&api-key=xvKZ-_oewvADCb2RR0W6bgp_EdLEKCLj",
					"http://backup.com?param=value",
				},
				"clickadilla.com": {
					"http://pop.zog.link/bid-request?token=h6dKfdh544FHD83",
				},
			},
		},
		{
			name:  "URLs with multiple equals and pipes",
			input: "key1=http://example.com?p1=v1&p2=v2|https://backup.com?a=1&b=2,key2=singleurl.com?test=value",
			expected: MapStringToStringSlice{
				"key1": {
					"http://example.com?p1=v1&p2=v2",
					"https://backup.com?a=1&b=2",
				},
				"key2": {
					"singleurl.com?test=value",
				},
			},
		},
		{
			name:     "Empty input",
			input:    "",
			expected: MapStringToStringSlice{},
		},
		{
			name:  "Invalid pair without equals",
			input: "key1=url1|url2,invalidPair,key3=url3",
			expected: MapStringToStringSlice{
				"key1": {"url1", "url2"},
				"key3": {"url3"},
			},
		},
		{
			name:  "With spaces around",
			input: " key1 = http://example.com?p=v | https://backup.com , key2 = single ",
			expected: MapStringToStringSlice{
				"key1": {
					"http://example.com?p=v",
					"https://backup.com",
				},
				"key2": {"single"},
			},
		},
		{
			name:  "Single URL without pipe",
			input: "domain.com=http://single.url.com?param=value",
			expected: MapStringToStringSlice{
				"domain.com": {
					"http://single.url.com?param=value",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m MapStringToStringSlice
			err := m.SetValue(tt.input)
			if err != nil {
				t.Errorf("SetValue() error = %v", err)
				return
			}
			if !reflect.DeepEqual(m, tt.expected) {
				t.Errorf("SetValue() got = %v, want %v", m, tt.expected)
			}
		})
	}
}
*/
