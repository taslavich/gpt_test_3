package bidEngine

import (
	"encoding/json"
	"fmt"
	"os"
)

var SspGeoPercents map[string]map[string]map[string]float32
var SspGeoDspPercentsFilePath string

func InitSspGeoPercentsLogic(filename string) error {
	SspGeoDspPercentsFilePath = filename
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	err = json.Unmarshal(data, &SspGeoPercents)
	if err != nil {
		return fmt.Errorf("failed to parse JSON from %s: %w", filename, err)
	}

	return nil
}
