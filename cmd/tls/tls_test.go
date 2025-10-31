package loadtest

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Конфигурация теста
const (
	targetURL     = "https://twinbidexchange.com/adm?id=c709afe1-bea9-4132-aee13&url=https%3A%2F%2Fkts.vasstycom.com%2Fin%2F266gRU_w0WXx8uICUafxnCKMHrYdx"
	threads       = 100             // Количество параллельных горутин
	totalRequests = 2000            // Общее количество запросов
	timeout       = 5 * time.Second // Таймаут на запрос
)

var (
	successCount    uint64
	errorCount      uint64
	sentCount       uint64 // Счетчик отправленных запросов
	receivedCount   uint64 // Счетчик полученных ответов
	noResponseCount uint64 // Счетчик запросов без ответа
)

// Тест для GET запросов
func TestLoadGETRequests(t *testing.T) {
	fmt.Printf("Starting load test: threads=%d totalRequests=%d\n", threads, totalRequests)

	startTime := time.Now()

	// Распределяем запросы по воркерам
	requestsPerWorker := totalRequests / threads
	remainder := totalRequests % threads

	var wg sync.WaitGroup
	results := make(chan *requestResult, totalRequests)

	// Запускаем воркеры
	for i := 0; i < threads; i++ {
		requests := requestsPerWorker
		if i < remainder {
			requests++
		}
		wg.Add(1)
		go worker(i, requests, results, &wg)
	}

	// Ждем завершения всех воркеров
	wg.Wait()
	close(results)

	// Собираем результаты
	for result := range results {
		atomic.AddUint64(&receivedCount, 1)
		if result.success {
			atomic.AddUint64(&successCount, 1)
		} else {
			atomic.AddUint64(&errorCount, 1)
		}
	}

	totalTime := time.Since(startTime)
	analyzeResults(totalTime)
}

// Результат запроса
type requestResult struct {
	success bool
	error   string
}

// Воркер для отправки GET запросов
func worker(id, requests int, results chan<- *requestResult, wg *sync.WaitGroup) {
	defer wg.Done()

	client := &http.Client{
		Timeout: timeout,
	}

	for i := 0; i < requests; i++ {
		// Увеличиваем счетчик отправленных запросов
		atomic.AddUint64(&sentCount, 1)

		result := sendGETRequest(client)
		results <- result
	}
}

// Отправка GET запроса
func sendGETRequest(client *http.Client) *requestResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return &requestResult{
			success: false,
			error:   fmt.Sprintf("Error creating request: %v", err),
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return &requestResult{
			success: false,
			error:   fmt.Sprintf("Error sending request: %v", err),
		}
	}
	defer resp.Body.Close()

	// Считаем успешными статусы 2xx и 3xx
	success := resp.StatusCode >= 200 && resp.StatusCode < 400
	if !success {
		return &requestResult{
			success: false,
			error:   fmt.Sprintf("HTTP %d", resp.StatusCode),
		}
	}

	return &requestResult{
		success: true,
		error:   "",
	}
}

// Анализ результатов
func analyzeResults(totalTime time.Duration) {
	success := atomic.LoadUint64(&successCount)
	errors := atomic.LoadUint64(&errorCount)
	sent := atomic.LoadUint64(&sentCount)
	received := atomic.LoadUint64(&receivedCount)

	// Запросы без ответа = отправлено - получено ответов
	noResponse := sent - received

	if sent == 0 {
		fmt.Println("No requests sent")
		return
	}

	rps := float64(sent) / totalTime.Seconds()
	successRate := float64(success) / float64(received) * 100
	responseRate := float64(received) / float64(sent) * 100

	fmt.Printf("\n=== LOAD TEST RESULTS ===\n")
	fmt.Printf("Requests SENT: %d\n", sent)
	fmt.Printf("Responses RECEIVED: %d\n", received)
	fmt.Printf("No Response: %d (%.2f%%)\n", noResponse, float64(noResponse)/float64(sent)*100)
	fmt.Printf("Response Rate: %.2f%%\n", responseRate)
	fmt.Printf("Successful: %d (%.2f%% of received)\n", success, successRate)
	fmt.Printf("Errors: %d (%.2f%% of received)\n", errors, float64(errors)/float64(received)*100)
	fmt.Printf("RPS: %.2f\n", rps)
	fmt.Printf("Test Duration: %v\n", totalTime)
	fmt.Printf("=========================\n")

	// Анализ потерь
	if noResponse > 0 {
		fmt.Printf("⚠️  %d requests got no response (network/timeout issues)\n", noResponse)
	}

	if responseRate >= 99.0 {
		fmt.Println("✅ Excellent response rate")
	} else if responseRate >= 95.0 {
		fmt.Println("✅ Good response rate")
	} else {
		fmt.Printf("❌ Poor response rate: %.2f%%\n", responseRate)
	}

	// Проверка эффективности
	if successRate >= 95.0 && responseRate >= 98.0 {
		fmt.Println("✅ Test PASSED - High success and response rate")
	} else {
		fmt.Printf("❌ Test FAILED - Success rate: %.2f%%, Response rate: %.2f%%\n", successRate, responseRate)
	}
}
