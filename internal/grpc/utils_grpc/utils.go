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

	// Автоматически определяем оптимальное количество потоков
	runtime.GOMAXPROCS(cores)

	// Разные настройки для разных сервисов
	switch serviceType {
	case "router":
		// Роутер - низкая задержка, высокая параллельность
		// Для Go 1.24+ SetMaxThreads устарел, используем осторожно
		debug.SetMaxThreads(cores * 100)    // 32 ядра * 100 = 3200 потоков
		debug.SetGCPercent(100)             // Менее агрессивный GC
		debug.SetMaxStack(64 * 1024 * 1024) // 64MB макс стек

		// Оптимизации для роутера
		runtime.MemProfileRate = 4096      // Реже профилирование памяти
		runtime.SetBlockProfileRate(0)     // Отключаем профилирование блокировок
		runtime.SetMutexProfileFraction(0) // Отключаем профилирование мьютексов

	case "orchestrator", "bidengine":
		// gRPC сервисы - баланс latency/throughput
		debug.SetMaxThreads(cores * 50)
		debug.SetGCPercent(80)
		debug.SetMaxStack(32 * 1024 * 1024)

	case "ssp":
		// HTTP сервис - больше горутин
		debug.SetMaxThreads(cores * 200)
		debug.SetGCPercent(120)
		debug.SetMaxStack(16 * 1024 * 1024)

	default:
		// Консервативные настройки по умолчанию
		debug.SetMaxThreads(cores * 25)
		debug.SetGCPercent(100)
		debug.SetMaxStack(16 * 1024 * 1024)
	}

	log.Printf("  GOMAXPROCS: %d, MaxThreads: %d, GC: %d%%, MaxStack: %dMB",
		runtime.GOMAXPROCS(0),
		debug.SetMaxThreads(0), // Получаем текущее значение
		debug.SetGCPercent(-1), // Получаем текущее значение
		debug.SetMaxStack(0)/(1024*1024))
}

func setSystemLimits() error {
	// Увеличиваем лимиты для высоконагруженного сервера
	var rLimit syscall.Rlimit

	// Файловые дескрипторы (важно для роутера с множеством соединений)
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err == nil {
		oldCur := rLimit.Cur
		rLimit.Cur = 1000000 // 1 млн дескрипторов
		rLimit.Max = 1000000
		if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
			log.Printf("  Не удалось установить RLIMIT_NOFILE: %v", err)
			// Пробуем меньшее значение
			rLimit.Cur = 500000
			rLimit.Max = 500000
			syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit)
		}
		log.Printf("  Файловые дескрипторы: %d -> %d", oldCur, rLimit.Cur)
	}

	return nil
}

func optimizeNetworkSettings(serviceType string) error {
	// Сетевые оптимизации для высоких RPS
	settings := []struct {
		key   string
		value string
	}{
		// Общие настройки TCP
		{"net.core.rmem_max", "67108864"}, // 64MB
		{"net.core.wmem_max", "67108864"},
		{"net.core.rmem_default", "4194304"}, // 4MB
		{"net.core.wmem_default", "4194304"},
		{"net.core.optmem_max", "4194304"},
		{"net.core.somaxconn", "65535"},
		{"net.core.netdev_max_backlog", "500000"},

		// TCP оптимизации
		{"net.ipv4.tcp_rmem", "4096 87380 67108864"},
		{"net.ipv4.tcp_wmem", "4096 65536 67108864"},
		{"net.ipv4.tcp_mem", "8388608 12582912 16777216"},
		{"net.ipv4.tcp_window_scaling", "1"},
		{"net.ipv4.tcp_timestamps", "1"},
		{"net.ipv4.tcp_sack", "1"},
		{"net.ipv4.tcp_fack", "1"},
		{"net.ipv4.tcp_syncookies", "1"},
		{"net.ipv4.tcp_max_syn_backlog", "3240000"},
		{"net.ipv4.tcp_synack_retries", "2"},
		{"net.ipv4.tcp_syn_retries", "2"},
		{"net.ipv4.tcp_retries2", "3"},
	}

	// Специфичные настройки для роутера
	if serviceType == "router" {
		routerSettings := []struct {
			key   string
			value string
		}{
			{"net.ipv4.ip_local_port_range", "1024 65000"},
			{"net.ipv4.tcp_fin_timeout", "10"},
			{"net.ipv4.tcp_tw_reuse", "1"},
			{"net.ipv4.tcp_tw_recycle", "0"}, // Выключено в новых ядрах
			{"net.ipv4.tcp_max_tw_buckets", "2000000"},
			{"net.ipv4.tcp_keepalive_time", "300"},
			{"net.ipv4.tcp_keepalive_probes", "3"},
			{"net.ipv4.tcp_keepalive_intvl", "30"},
			{"net.ipv4.tcp_slow_start_after_idle", "0"}, // Важно для постоянной нагрузки
		}
		settings = append(settings, routerSettings...)
	}

	for _, setting := range settings {
		if err := sysctlWrite(setting.key, setting.value); err != nil {
			log.Printf("  ⚠️  Не удалось установить %s=%s: %v",
				setting.key, setting.value, err)
		}
	}

	log.Printf("  Сетевые настройки оптимизированы для %s", serviceType)
	return nil
}

func sysctlWrite(key, value string) error {
	// Проверяем существование параметра
	cmd := exec.Command("sysctl", "-n", key)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("параметр %s не существует", key)
	}

	// Устанавливаем значение
	cmd = exec.Command("sysctl", "-w", key+"="+value)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("не удалось установить %s: %v, вывод: %s",
			key, err, string(output))
	}

	log.Printf("    %s = %s", key, value)
	return nil
}
