package bidEngine

import (
	"encoding/json"
	"fmt"
	"os"
)

const (
	LEFT = "LEFT"
	ANY = "ANY"
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

func GetGeoDspPercent(ssp, geo, dsp string, percentsMap map[string]map[string]map[string]float32, defaultPercent float32) float32 {
	if percentsMap == nil {
		return defaultPercent
	}

	// 1. Ищем SSP уровень
	if sspMap, sspExists := percentsMap[ANY]
	sspMap, sspExists := percentsMap[ssp]
	if !sspExists {
		// Пробуем ANY для SSP
		sspMap, sspExists = percentsMap["ANY"]
		if !sspExists {
			return defaultPercent
		}
	}

	// 2. Ищем GEO уровень
	geoMap, geoExists := sspMap[geo]
	if !geoExists {
		// Пробуем ANY для GEO
		geoMap, geoExists = sspMap["ANY"]
		if !geoExists {
			return defaultPercent
		}
	}

	// 3. Ищем DSP уровень
	percent, dspExists := geoMap[dsp]
	if !dspExists {
		// Пробуем ANY для DSP
		percent, dspExists = geoMap["ANY"]
		if !dspExists {
			// Пробуем LEFT для DSP
			percent, dspExists = geoMap["LEFT"]
			if !dspExists {
				return defaultPercent
			}
		}
	}

	return percent
}
