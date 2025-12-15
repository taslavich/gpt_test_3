package utils

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"

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

// OptimizeAll - вызывает все оптимизации для сервиса
// serviceType: "router", "orchestrator", "bidengine", "ssp", "clickhouse"
func OptimizeAll(serviceType string) error {
	log.Printf("⚡ Оптимизация для %s (CPU: %d ядер)", serviceType, runtime.NumCPU())

	optimizeGoRuntime(serviceType)
	if err := setSystemLimits(); err != nil {
		return fmt.Errorf("Cannot setSystemLimits %v", err)
	}
	if err := optimizeNetworkSettings(serviceType); err != nil {
		return fmt.Errorf("Cannot optimizeNetworkSettings %v", err)
	}

	log.Printf("✅ Оптимизации применены для %s", serviceType)

	return nil
}

func optimizeGoRuntime(serviceType string) {
	cores := runtime.NumCPU()

	// Разные настройки для разных сервисов
	switch serviceType {
	case "rout":
		// Роутер - максимальная производительность
		debug.SetMaxThreads(50000)
		runtime.GOMAXPROCS(cores)
		debug.SetGCPercent(30)              // Агрессивный GC для low latency
		debug.SetMaxStack(16 * 1024 * 1024) // 256MB

	case "orch", "eng":
		// ГРПЦ сервисы - баланс latency/throughput
		debug.SetMaxThreads(50000)
		runtime.GOMAXPROCS(cores)
		debug.SetGCPercent(20)
		debug.SetMaxStack(128 * 1024 * 1024)

	case "ssp":
		// HTTP сервис - больше горутин
		debug.SetMaxThreads(100000)
		runtime.GOMAXPROCS(cores)
		debug.SetGCPercent(30) // Менее агрессивный
		debug.SetMaxStack(64 * 1024 * 1024)

	default:
		// Остальные
		debug.SetMaxThreads(30000)
		runtime.GOMAXPROCS(cores)
		debug.SetGCPercent(30)
		debug.SetMaxStack(64 * 1024 * 1024)
	}

	log.Printf("  GOMAXPROCS: %d, GC: %d%%, MaxStack: %dMB",
		runtime.GOMAXPROCS(0), debug.SetGCPercent(0), debug.SetMaxStack(0)/(1024*1024))
}

func setSystemLimits() error {
	// Только если root
	if os.Geteuid() != 0 {
		return fmt.Errorf("Без root прав, системные лимиты не изменены")
	}

	// Дескрипторы файлов
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err == nil {
		rLimit.Cur = 500000
		rLimit.Max = 500000
		syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit)
		log.Printf("  Файловые дескрипторы: %d", rLimit.Cur)
	}

	return nil
}

func optimizeNetworkSettings(serviceType string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("Без root прав, системные лимиты не изменены")
	}

	// Общие настройки для всех
	if err := sysctlWrite("net.core.rmem_max", "16777216"); err != nil {
		return fmt.Errorf("Cannot sysctlWrite %v", err)
	}
	if err := sysctlWrite("net.core.wmem_max", "16777216"); err != nil {
		return fmt.Errorf("Cannot sysctlWrite %v", err)
	}

	// Специфичные для роутера (много исходящих соединений)
	if serviceType == "router" {
		if err := sysctlWrite("net.ipv4.ip_local_port_range", "1024 65535"); err != nil {
			return fmt.Errorf("Cannot sysctlWrite %v", err)
		}
		if err := sysctlWrite("net.ipv4.tcp_fin_timeout", "5"); err != nil {
			return fmt.Errorf("Cannot sysctlWrite %v", err)
		}
		if err := sysctlWrite("net.ipv4.tcp_tw_reuse", "1"); err != nil {
			return fmt.Errorf("Cannot sysctlWrite %v", err)
		}
		if err := sysctlWrite("net.core.somaxconn", "65535"); err != nil {
			return fmt.Errorf("Cannot sysctlWrite %v", err)
		}
	}

	return nil
}

func sysctlWrite(key, value string) error {
	cmd := exec.Command("sysctl", "-w", key+"="+value)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Cannot Run %v", err)
	}

	return nil
}
