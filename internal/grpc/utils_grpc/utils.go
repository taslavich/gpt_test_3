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

func OptimizeAll(serviceType string) error {
	log.Printf("⚡ Оптимизация для %s (CPU: %d ядер, Go: %s)",
		serviceType, runtime.NumCPU(), runtime.Version())

	optimizeGoRuntime(serviceType)

	// Системные оптимизации только если root
	if os.Geteuid() == 0 {
		if err := setSystemLimits(); err != nil {
			log.Printf("⚠️  Системные лимиты не изменены: %v", err)
		}
		if err := optimizeNetworkSettings(serviceType); err != nil {
			log.Printf("⚠️  Сетевые настройки не изменены: %v", err)
		}
	} else {
		log.Printf("⚠️  Требуются root права для системных оптимизаций")
	}

	log.Printf("✅ Оптимизации применены для %s", serviceType)
	return nil
}
func optimizeGoRuntime(serviceType string) {
	cores := runtime.NumCPU()

	// 1. Устанавливаем GOMAXPROCS - безопасно и полезно
	runtime.GOMAXPROCS(cores)

	// 2. Установка MaxThreads - ВАЖНО: НЕ вызываем SetMaxThreads(0) для получения значения!
	// В Go 1.24 нужно устанавливать только положительные значения
	// или не трогать вообще (runtime сам управляет потоками)
	switch serviceType {
	case "router":
		// Для роутера с 64 ядрами устанавливаем разумное значение
		debug.SetMaxThreads(10000)          // Безопасное значение для высоконагрузочного сервиса
		debug.SetGCPercent(200)             // Реже GC для лучшей производительности
		debug.SetMaxStack(32 * 1024 * 1024) // 32MB

	case "orchestrator", "bidengine":
		debug.SetMaxThreads(5000)
		debug.SetGCPercent(150)
		debug.SetMaxStack(16 * 1024 * 1024)

	case "ssp":
		debug.SetMaxThreads(8000)
		debug.SetGCPercent(200)
		debug.SetMaxStack(8 * 1024 * 1024)

	default:
		debug.SetMaxThreads(2000)
		debug.SetGCPercent(100)
		debug.SetMaxStack(8 * 1024 * 1024)
	}

	// 3. Отключаем ненужное профилирование в продакшене
	runtime.MemProfileRate = 0
	runtime.SetBlockProfileRate(0)
	runtime.SetMutexProfileFraction(0)

	// 4. Логируем - БЕЗ вызова SetMaxThreads(0) для получения значений!
	log.Printf("  GOMAXPROCS: %d, GC: %d%%, MaxStack: %dMB",
		runtime.GOMAXPROCS(0),
		debug.SetGCPercent(-1), // Это еще работает
		debug.SetMaxStack(0)/(1024*1024))
	// НЕЛЬЗЯ: debug.SetMaxThreads(0) - это вызывает панику в Go 1.24
}

func setSystemLimits() error {
	// Увеличиваем лимиты для высоконагруженного сервера
	var rLimit syscall.Rlimit

	// Файловые дескрипторы
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err == nil {
		oldCur := rLimit.Cur
		// Для 64 ядерного сервера
		rLimit.Cur = 500000
		rLimit.Max = 500000
		if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
			log.Printf("  Не удалось установить RLIMIT_NOFILE: %v", err)
			// Пробуем стандартное значение
			rLimit.Cur = 100000
			rLimit.Max = 100000
			syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit)
		}
		log.Printf("  Файловые дескрипторы: %d -> %d", oldCur, rLimit.Cur)
	}

	return nil
}

func optimizeNetworkSettings(serviceType string) error {
	// Сетевые оптимизации для высоких RPS
	// Используем безопасный подход - только основные параметры
	settings := []struct {
		key      string
		value    string
		required bool // Обязательный ли параметр
	}{
		// Только самые важные и безопасные параметры
		{"net.core.somaxconn", "65535", true},
		{"net.core.netdev_max_backlog", "100000", false},
		{"net.ipv4.tcp_max_syn_backlog", "3240000", false},
	}

	// Специфичные настройки для роутера (только безопасные)
	if serviceType == "router" {
		routerSettings := []struct {
			key      string
			value    string
			required bool
		}{
			{"net.ipv4.ip_local_port_range", "1024 65000", true},
			{"net.ipv4.tcp_tw_reuse", "1", false},
			{"net.ipv4.tcp_fin_timeout", "10", false},
		}
		settings = append(settings, routerSettings...)
	}

	successCount := 0
	for _, setting := range settings {
		if err := safeSysctlWrite(setting.key, setting.value); err != nil {
			if setting.required {
				log.Printf("  ⚠️ Не удалось установить обязательный параметр %s: %v", setting.key, err)
				// Для обязательных параметров можно вернуть ошибку
				// return fmt.Errorf("не удалось установить %s: %v", setting.key, err)
			} else {
				log.Printf("  ⚠️ Не удалось установить параметр %s: %v", setting.key, err)
			}
		} else {
			successCount++
		}
	}

	log.Printf("  Установлено %d/%d сетевых параметров", successCount, len(settings))
	return nil
}

func safeSysctlWrite(key, value string) error {
	// Пробуем установить без предварительной проверки
	cmd := exec.Command("sysctl", "-w", key+"="+value)
	output, err := cmd.CombinedOutput()

	if err != nil {
		// Проверяем, может быть параметр уже установлен или не поддерживается
		outputStr := string(output)
		if strings.Contains(outputStr, "permission denied") {
			return fmt.Errorf("нет прав для установки %s", key)
		} else if strings.Contains(outputStr, "No such file or directory") {
			return fmt.Errorf("параметр %s не поддерживается ядром", key)
		} else if strings.Contains(outputStr, "Invalid argument") {
			return fmt.Errorf("недопустимое значение для %s", key)
		}
		return fmt.Errorf("%s: %v", strings.TrimSpace(outputStr), err)
	}

	log.Printf("    %s = %s", key, value)
	return nil
}
