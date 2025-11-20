package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	ALL = "ALL"
)

func IsInStringSlice(variable string, list []string) bool {
	for _, target := range list {
		if variable == target {
			return true
		}
	}

	return false
}

func InitSspGeoDspMap[T *types.PercentAndBidfloor | bool](filename string) (map[string]map[string]map[string]T, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	tmp := make(map[string]map[string]map[string]T)

	err = json.Unmarshal(data, &tmp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON from %s: %w", filename, err)
	}

	return SetAndConvertNonGoodMap(tmp), nil
}

func SetAndConvertNonGoodMap[T *types.PercentAndBidfloor | bool](tmp map[string]map[string]map[string]T) map[string]map[string]map[string]T {
	out := make(map[string]map[string]map[string]T)
	for sspKey, geoMap := range tmp {
		sspKeys := splitAndTrimKeys(sspKey)

		for _, singleSspKey := range sspKeys {
			if out[singleSspKey] == nil {
				out[singleSspKey] = make(map[string]map[string]T)
			}

			for geoKey, dspMap := range geoMap {
				geoKeys := splitAndTrimKeys(geoKey)

				for _, singleGeoKey := range geoKeys {
					if out[singleSspKey][singleGeoKey] == nil {
						out[singleSspKey][singleGeoKey] = make(map[string]T)
					}

					for dspKey, value := range dspMap {
						dspKeys := splitAndTrimKeys(dspKey)

						for _, singleDspKey := range dspKeys {
							out[singleSspKey][singleGeoKey][singleDspKey] = value
						}
					}
				}
			}
		}
	}

	return out
}

func splitAndTrimKeys(key string) []string {
	if key == "" {
		return []string{""}
	}

	parts := strings.Split(key, ",")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	if len(result) == 0 {
		return []string{""}
	}

	return result
}

func GetValueFomSspGeoDspMap[T *types.PercentAndBidfloor | bool](ssp, geo, dsp string, valueMap map[string]map[string]map[string]T, uknownValue T) T {
	if valueMap == nil {
		return uknownValue
	}

	sspAll := []string{ssp, ALL}
	geoAll := []string{geo, ALL}
	dspAll := []string{dsp, ALL}

	for i := range sspAll {
		if geoMap, ok := valueMap[sspAll[i]]; ok {
			for j := range geoAll {
				if dspMap, ok := geoMap[geoAll[j]]; ok {
					for k := range geoAll {
						if mainObj, ok := dspMap[dspAll[k]]; ok {
							return mainObj
						}
					}
				}
			}
		}
	}

	return uknownValue
}

func RewriteSspGeoDspFile[T *types.PercentAndBidfloor | bool](JsonData, percentFilename string) (map[string]map[string]map[string]T, error) {
	var inputMap map[string]map[string]map[string]T
	err := json.Unmarshal([]byte(JsonData), &inputMap)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid JSON format: %v", err)
	}

	fileData, err := json.MarshalIndent(inputMap, "", "  ")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal data for file: %v", err)
	}

	err = os.WriteFile(percentFilename, fileData, 0644)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to write file %s: %v", percentFilename, err)
	}

	return SetAndConvertNonGoodMap(inputMap), nil
}

func RewriteSspGeoDspFileNextVer[
	T *types.PercentAndBidfloor |
		bool,
](
	inputMap map[string]map[string]map[string]T,
	percentFilename string,
) (
	map[string]map[string]map[string]T,
	error,
) {
	fileData, err := json.MarshalIndent(inputMap, "", "  ")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal data for file: %v", err)
	}

	err = os.WriteFile(percentFilename, fileData, 0644)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to write file %s: %v", percentFilename, err)
	}

	return SetAndConvertNonGoodMap(inputMap), nil
}
