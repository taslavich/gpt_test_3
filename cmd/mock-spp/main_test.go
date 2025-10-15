package loadtest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

// Конфигурация теста
const (
	sppAdapterURL = "https://twinbidexchange.com/bidRequest/bid_v_2_5"
	threads       = 100             // Количество параллельных горутин
	targetRPS     = 10000           // Целевая нагрузка (RPS)
	testDuration  = 2 * time.Second // Длительность теста
)

var (
	globalIDCounter uint64
)

// Конфигурация диагностики
var (
	enableDiagnostics = true              // Включить сбор диагностики
	diagDuration      = 70 * time.Second  // Длительность диагностики (немного больше теста)
	diagOutputDir     = "./loadtest-diag" // Каталог для диагностических логов
	processPattern    = ""                //"rtb_bid-engine|rtb_orchestrator|rtb_router|rtb_spp-adapter|rtb_kafka-loader|rtb_clickhouse-loader|rtb_mock-dsp-1|rtb_mock-dsp-2|rtb_mock-dsp-3|rtb_redis|rtb_kafka|rtb_nginx-gateway" // Шаблон для поиска процессов RTB
)

type streamSpec struct {
	name     string
	args     []string
	filename string
}

type snapshotSpec struct {
	name     string
	args     []string
	interval time.Duration
	filename string
}

// Генератор тестовых BidRequest для нового формата
func generateBidRequest() map[string]interface{} {
	bidFloor := float32(0.5)
	w := int32(300)
	h := int32(250)
	country := "US"

	id := atomic.AddUint64(&globalIDCounter, 1)

	// Создаем ORTB 2.5 BidRequest
	bidRequest := &ortb_V2_5.BidRequest{
		Id: stringPtr(fmt.Sprintf("req-%d", id)),
		At: int32Ptr(1),
		Imp: []*ortb_V2_5.Imp{
			{
				Id:       stringPtr("imp-1"),
				BidFloor: &bidFloor,
				Banner: &ortb_V2_5.Banner{
					W: &w,
					H: &h,
				},
			},
		},
		Device: &ortb_V2_5.Device{
			Ip: stringPtr(generateRandomIP()),
			Geo: &ortb_V2_5.Geo{
				Country: &country,
			},
		},
	}

	// Оборачиваем в нужный формат
	return map[string]interface{}{
		"bid_request": bidRequest,
		"ssp_domain":  "example.com",
	}
}

// Вспомогательные функции
func stringPtr(s string) *string { return &s }
func int32Ptr(i int32) *int32    { return &i }

func generateRandomIP() string {
	// Простая генерация приватного IP, быстро меняющаяся
	n := atomic.AddUint64(&globalIDCounter, 1)
	return fmt.Sprintf("10.%d.%d.%d", byte(n>>16), byte(n>>8), byte(n))
}

// Тест производительности (rate-based)
func TestLoadRTBSystem(t *testing.T) {
	fmt.Printf("Starting load test: threads=%d targetRPS=%d duration=%v\n", threads, targetRPS, testDuration)

	// распределяем RPS по воркерам, учитывая остаток
	perWorker := targetRPS / threads
	remainder := targetRPS % threads

	// буфер результатов — targetRPS * duration (максимум). Берём минимум с разумным лимитом.
	maxResults := targetRPS * int(testDuration.Seconds())
	if maxResults < 1 {
		maxResults = targetRPS
	}
	if maxResults > 5_000_000 {
		maxResults = 5_000_000 // защита от OOM
	}

	results := make(chan *testResult, maxResults)
	var wg sync.WaitGroup

	startTime := time.Now()
	stopCh := make(chan struct{})

	// Запускаем воркеры
	for i := 0; i < threads; i++ {
		rps := perWorker
		if i < remainder {
			rps++
		}
		wg.Add(1)
		go workerRate(i, rps, results, &wg, stopCh)
	}

	// Останавливаем через duration
	time.AfterFunc(testDuration, func() {
		close(stopCh)
	})

	// Закрываем results после завершения воркеров
	go func() {
		wg.Wait()
		close(results)
	}()

	// Сбор результатов
	var resultsSlice []*testResult
	for result := range results {
		resultsSlice = append(resultsSlice, result)
	}

	totalTime := time.Since(startTime)
	analyzeResults(resultsSlice, totalTime)

	// Ждем завершения диагностики
	if enableDiagnostics {
		fmt.Println("⏳ Ожидание завершения диагностики...")
		time.Sleep(5 * time.Second)
		fmt.Printf("📊 Диагностические логи сохранены в: %s\n", diagOutputDir)
	}
}

// Воркер с таргетом rps
func workerRate(id, rps int, results chan<- *testResult, wg *sync.WaitGroup, stopCh <-chan struct{}) {
	defer wg.Done()
	if rps <= 0 {
		return
	}

	interval := time.Second / time.Duration(rps)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			start := time.Now()
			bidRequest := generateBidRequest()
			result := sendBidRequestWithClient(bidRequest, start, client)
			// non-blocking send to avoid deadlock if results channel full
			select {
			case results <- result:
			default:
			}
		}
	}
}

// Отправка BidRequest с переиспользуемым клиентом
func sendBidRequestWithClient(bidRequest map[string]interface{}, startTime time.Time, client *http.Client) *testResult {
	jsonData, err := json.Marshal(bidRequest)
	if err != nil {
		return &testResult{
			success:   false,
			latency:   time.Since(startTime),
			error:     err.Error(),
			timestamp: startTime,
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", sppAdapterURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return &testResult{
			success:   false,
			latency:   time.Since(startTime),
			error:     err.Error(),
			timestamp: startTime,
		}
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return &testResult{
			success:   false,
			latency:   time.Since(startTime),
			error:     err.Error(),
			timestamp: startTime,
		}
	}
	defer resp.Body.Close()

	latency := time.Since(startTime)

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		return &testResult{
			success:   true,
			latency:   latency,
			error:     "",
			timestamp: startTime,
		}
	}

	return &testResult{
		success:   false,
		latency:   latency,
		error:     fmt.Sprintf("HTTP %d", resp.StatusCode),
		timestamp: startTime,
	}
}

// Результат теста
type testResult struct {
	success   bool
	latency   time.Duration
	error     string
	timestamp time.Time
}

// Анализ результатов
func analyzeResults(results []*testResult, totalTime time.Duration) {
	var totalLatency time.Duration
	var successCount int
	var errorCount int

	latencies := make([]time.Duration, 0, len(results))

	for _, result := range results {
		latencies = append(latencies, result.latency)
		totalLatency += result.latency

		if result.success {
			successCount++
		} else {
			errorCount++
			log.Printf("Error: %s (latency: %v)", result.error, result.latency)
		}
	}

	totalRequests := len(results)
	if totalRequests == 0 {
		fmt.Println("No requests recorded")
		return
	}

	rps := float64(totalRequests) / totalTime.Seconds()
	avgLatency := totalLatency / time.Duration(totalRequests)

	// Percentiles via sort.Slice
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p := func(q float64) time.Duration {
		if len(latencies) == 0 {
			return 0
		}
		idx := int(float64(len(latencies)) * q)
		if idx < 0 {
			idx = 0
		}
		if idx >= len(latencies) {
			idx = len(latencies) - 1
		}
		return latencies[idx]
	}

	p50 := p(0.50)
	p95 := p(0.95)
	p99 := p(0.99)

	fmt.Printf("\n=== LOAD TEST RESULTS ===\n")
	fmt.Printf("Total Requests: %d\n", totalRequests)
	fmt.Printf("Successful: %d (%.2f%%)\n", successCount, float64(successCount)/float64(totalRequests)*100)
	fmt.Printf("Errors: %d (%.2f%%)\n", errorCount, float64(errorCount)/float64(totalRequests)*100)
	fmt.Printf("RPS: %.2f\n", rps)
	fmt.Printf("Test Duration: %v\n", totalTime)
	fmt.Printf("Average Latency: %v\n", avgLatency)
	fmt.Printf("P50 Latency: %v\n", p50)
	fmt.Printf("P95 Latency: %v\n", p95)
	fmt.Printf("P99 Latency: %v\n", p99)
	fmt.Printf("=========================\n")

	// Проверка требований 5k RPS — оставил для обратной совместимости
	if rps >= 5000 {
		fmt.Println("✅ Требование 5k RPS ВЫПОЛНЕНО")
	} else {
		fmt.Printf("❌ Требование 5k RPS НЕ ВЫПОЛНЕНО (текущее: %.2f)\n", rps)
	}

	if avgLatency < 100*time.Millisecond {
		fmt.Println("✅ Latency requirement MET")
	} else {
		fmt.Printf("⚠️  Latency is high: %v\n", avgLatency)
	}
}

// ============================
// ДИАГНОСТИЧЕСКАЯ СИСТЕМА
// ============================

func runDiagnostics() {
	if err := os.MkdirAll(diagOutputDir, 0o755); err != nil {
		log.Printf("❌ Ошибка создания каталога диагностики: %v", err)
		return
	}

	pidList := strings.Join(findPIDs(processPattern), ",")
	ctx, cancel := context.WithTimeout(context.Background(), diagDuration)
	defer cancel()

	var wg sync.WaitGroup

	streamCmds := []streamSpec{
		{"top", buildTopArgs(pidList), "top.log"},
		{"pidstat", buildPidstatArgs(pidList), "pidstat.log"},
		{"mpstat", []string{"-P", "ALL", "2"}, "mpstat.log"},
		{"vmstat", []string{"2"}, "vmstat.log"},
		{"iostat", []string{"-xz", "2"}, "iostat.log"},
		{"sar", []string{"-n", "DEV", "2"}, "sar-dev.log"},
		{"sar", []string{"-n", "TCP,ETCP", "2"}, "sar-tcp.log"},
	}

	for _, spec := range streamCmds {
		wg.Add(1)
		go runStream(ctx, &wg, spec, diagOutputDir)
	}

	snapshotCmds := []snapshotSpec{
		{"free", []string{"-m"}, 10 * time.Second, "free.log"},
		{"ss", []string{"-s"}, 10 * time.Second, "ss-summary.log"},
		{"ss", []string{"-tan", "state", "time-wait"}, 10 * time.Second, "ss-timewait.log"},
	}

	for _, spec := range snapshotCmds {
		wg.Add(1)
		go runSnapshots(ctx, &wg, spec, diagOutputDir)
	}

	// Записываем информацию о тесте
	writeTestInfo(diagOutputDir)

	wg.Wait()
	fmt.Println("✅ Диагностика завершена")
}

func findPIDs(pattern string) []string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil
	}
	out, err := exec.Command("pgrep", "-f", pattern).Output()
	if err != nil {
		log.Printf("⚠️ pgrep не нашёл процессов для %q: %v", pattern, err)
		return nil
	}
	return strings.Fields(string(out))
}

func buildTopArgs(pidList string) []string {
	args := []string{"-b", "-H", "-d", "2"}
	if pidList != "" {
		args = append(args, "-p", pidList)
	}
	return args
}

func buildPidstatArgs(pidList string) []string {
	args := []string{"-u", "-r", "-d", "2"}
	if pidList != "" {
		args = append(args, "-p", pidList)
	}
	return args
}

func runStream(ctx context.Context, wg *sync.WaitGroup, spec streamSpec, dir string) {
	defer wg.Done()

	path := filepath.Join(dir, spec.filename)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Printf("❌ Не удалось открыть %s: %v", path, err)
		return
	}
	defer file.Close()

	// Записываем заголовок
	fmt.Fprintf(file, "=== %s started at %s ===\n", spec.name, time.Now().Format(time.RFC3339))
	fmt.Fprintf(file, "Command: %s %s\n\n", spec.name, strings.Join(spec.args, " "))

	cmd := exec.CommandContext(ctx, spec.name, spec.args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("❌ %s stdout: %v", spec.name, err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("❌ %s stderr: %v", spec.name, err)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("❌ %s start: %v", spec.name, err)
		return
	}

	var copierWG sync.WaitGroup
	copierWG.Add(2)
	go copyStream(&copierWG, file, stdout)
	go copyStream(&copierWG, file, stderr)

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := cmd.Wait(); err != nil && ctx.Err() == nil {
			log.Printf("⚠️ %s завершилась с ошибкой: %v", spec.name, err)
		}
	}()

	select {
	case <-ctx.Done():
		_ = cmd.Process.Signal(os.Interrupt)
		<-done
	case <-done:
	}
	copierWG.Wait()
	fmt.Fprintf(file, "\n=== %s finished at %s ===\n\n", spec.name, time.Now().Format(time.RFC3339))
}

func runSnapshots(ctx context.Context, wg *sync.WaitGroup, spec snapshotSpec, dir string) {
	defer wg.Done()

	path := filepath.Join(dir, spec.filename)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Printf("❌ Не удалось открыть %s: %v", path, err)
		return
	}
	defer file.Close()

	// Записываем заголовок
	fmt.Fprintf(file, "=== %s snapshots started at %s ===\n", spec.name, time.Now().Format(time.RFC3339))
	fmt.Fprintf(file, "Command: %s %s\n", spec.name, strings.Join(spec.args, " "))
	fmt.Fprintf(file, "Interval: %v\n\n", spec.interval)

	ticker := time.NewTicker(spec.interval)
	defer ticker.Stop()

	writeSnapshot(ctx, file, spec)
	for {
		select {
		case <-ctx.Done():
			fmt.Fprintf(file, "\n=== %s snapshots finished at %s ===\n\n", spec.name, time.Now().Format(time.RFC3339))
			return
		case <-ticker.C:
			writeSnapshot(ctx, file, spec)
		}
	}
}

func writeSnapshot(ctx context.Context, file *os.File, spec snapshotSpec) {
	fmt.Fprintf(file, "\n--- %s ---\n", time.Now().Format(time.RFC3339))
	cmd := exec.CommandContext(ctx, spec.name, spec.args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(file, "❌ ошибка: %v\n", err)
	}
	file.Write(output)
}

func copyStream(wg *sync.WaitGroup, dst *os.File, src io.Reader) {
	defer wg.Done()
	scanner := bufio.NewScanner(src)
	for scanner.Scan() {
		fmt.Fprintln(dst, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		log.Printf("⚠️ ошибка чтения потока: %v", err)
	}
}

func writeTestInfo(dir string) {
	path := filepath.Join(dir, "test_info.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Printf("❌ Не удалось записать информацию о тесте: %v", err)
		return
	}
	defer file.Close()

	fmt.Fprintf(file, "=== Load Test Information ===\n")
	fmt.Fprintf(file, "Start Time: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(file, "Target URL: %s\n", sppAdapterURL)
	fmt.Fprintf(file, "Threads: %d\n", threads)
	fmt.Fprintf(file, "Target RPS: %d\n", targetRPS)
	fmt.Fprintf(file, "Test Duration: %v\n", testDuration)
	fmt.Fprintf(file, "Process Pattern: %s\n", processPattern)
	fmt.Fprintf(file, "=============================\n\n")
}
