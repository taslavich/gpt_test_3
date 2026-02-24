package geoBadIp

import (
	"encoding/json"
	"fmt"
	"os"
)

func NewGeoToLang(filename string) (GeoToLang, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	var tmp GeoToLang

	err = json.Unmarshal(data, &tmp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON from %s: %w", filename, err)
	}

	return tmp, nil
}

type GeoToLang = map[string]string
