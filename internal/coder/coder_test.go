package coder

import (
	"log"
	"strings"
	"testing"
)

// Тесты
func TestAdmToAdidRoundTrip(t *testing.T) {
	testCases := []struct {
		name string
		adm  string
	}{
		{
			name: "simple_popunder",
			adm:  `<script>window.open('https://example.com', '_blank');</script>`,
		},
		{
			name: "popunder_with_params",
			adm:  `<script>window.open('https://example.com?source=rtb&campaign=123', '_blank');</script>`,
		},
		{
			name: "html_banner",
			adm:  `<div style="width:300px;height:250px;background:#f00;"><a href="https://example.com/click">Click me!</a></div>`,
		},
		{
			name: "empty_string",
			adm:  "",
		},
		{
			name: "special_chars",
			adm:  `<script>alert("Hello 'World' \"Test\" & <tags>");</script>`,
		},
		{
			name: "unicode_content",
			adm:  `<div>Привет мир! 🚀 中文测试</div>`,
		},
		{
			name: "long_content",
			adm:  strings.Repeat(`<div>This is a very long ad content that should be compressed well. </div>`, 50),
		},
		{
			name: "json_like",
			adm:  `{"url":"https://example.com","width":300,"height":250}`,
		},
		{
			name: "with_newlines",
			adm: `<script>
    window.open('https://example.com', '_blank');
    console.log('Popup opened');
</script>`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Преобразуем adm → adid
			adid := AdmToAdidCompact(tc.adm)

			// Проверяем что adid начинается с префикса
			if !strings.HasPrefix(adid, "ad_") {
				t.Errorf("adid should start with 'ad_' prefix, got: %s", adid)
			}

			// Преобразуем обратно adid → adm
			recoveredAdm, err := AdidToAdmCompact(adid)
			if err != nil {
				t.Fatalf("Failed to convert adid back to adm: %v", err)
			}

			// Проверяем что получили исходный adm
			if recoveredAdm != tc.adm {
				t.Errorf("Round-trip failed!\nOriginal: %q\nRecovered: %q", tc.adm, recoveredAdm)
			}

			t.Logf("Success: %d chars → %d chars (adid)", len(tc.adm), len(adid))
		})
	}
}

func TestAdidToAdmErrorCases(t *testing.T) {
	errorCases := []struct {
		name        string
		adid        string
		shouldError bool
	}{
		{
			name:        "missing_prefix",
			adid:        "invalid_adid_without_prefix",
			shouldError: true,
		},
		{
			name:        "empty_string",
			adid:        "",
			shouldError: true,
		},
		{
			name:        "only_prefix",
			adid:        "ad_",
			shouldError: true,
		},
		{
			name:        "invalid_base64",
			adid:        "ad_invalid!!!base64@@@",
			shouldError: true,
		},
		{
			name:        "corrupted_data",
			adid:        "ad_abc123", // случайные данные
			shouldError: true,
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := AdidToAdmCompact(tc.adid)

			if tc.shouldError {
				if err == nil {
					t.Errorf("Expected error for adid %q but got result: %q", tc.adid, result)
				} else {
					t.Logf("Correctly returned error for %q: %v", tc.adid, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for adid %q but got: %v", tc.adid, err)
				} else {
					t.Logf("Correctly processed valid adid: %q -> %q", tc.adid, result)
				}
			}
		})
	}
}

// Тест специально для валидных adid
func TestAdidToAdmValidCases(t *testing.T) {
	validCases := []struct {
		name string
		adm  string
	}{
		{
			name: "simple_adm",
			adm:  "test",
		},
		{
			name: "popunder",
			adm:  `<script>window.open('https://example.com', '_blank');</script>`,
		},
	}

	for _, tc := range validCases {
		t.Run(tc.name, func(t *testing.T) {
			// Создаем валидный adid из adm
			adid := AdmToAdidCompact(tc.adm)

			// Преобразуем обратно
			recoveredAdm, err := AdidToAdmCompact(adid)
			if err != nil {
				t.Fatalf("Failed to convert valid adid back to adm: %v", err)
			}

			// Проверяем что получили исходный adm
			if recoveredAdm != tc.adm {
				t.Errorf("Round-trip failed for valid adid!\nOriginal: %q\nRecovered: %q", tc.adm, recoveredAdm)
			}

			t.Logf("Valid adid test passed: %q -> %q", tc.adm, adid)
		})
	}
}

func TestCompressionEfficiency(t *testing.T) {
	// Тест на эффективность сжатия
	longAdm := strings.Repeat(`<div><img src="https://example.com/banner.jpg" alt="Advertisement"><script>trackImpression();</script></div>`, 100)

	originalSize := len(longAdm)
	adid := AdmToAdidCompact(longAdm)
	compressedSize := len(adid)

	compressionRatio := float64(compressedSize) / float64(originalSize) * 100

	t.Logf("Compression efficiency: %d → %d bytes (%.1f%%)",
		originalSize, compressedSize, compressionRatio)

	// Восстанавливаем и проверяем
	recovered, err := AdidToAdmCompact(adid)
	if err != nil {
		t.Fatalf("Failed to recover long content: %v", err)
	}

	if recovered != longAdm {
		t.Error("Long content recovery failed")
	}
}

// Бенчмарк тест для производительности
func BenchmarkAdmToAdid(b *testing.B) {
	testAdm := `<script>window.open('https://example.com/popunder?campaign=test&source=rtb', '_blank');</script>`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		AdmToAdidCompact(testAdm)
	}
}

func BenchmarkAdidToAdm(b *testing.B) {
	testAdm := `<script>window.open('https://example.com/popunder', '_blank');</script>`
	adid := AdmToAdidCompact(testAdm)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		AdidToAdmCompact(adid)
	}
}

// Дополнительный тест на пограничные случаи
func TestEdgeCases(t *testing.T) {
	// Многократное преобразование туда-обратно
	original := `<div>Test content</div>`

	for i := 0; i < 10; i++ {
		adid := AdmToAdidCompact(original)
		recovered, err := AdidToAdmCompact(adid)
		if err != nil {
			t.Fatalf("Iteration %d failed: %v", i, err)
		}
		if recovered != original {
			t.Fatalf("Iteration %d: content changed", i)
		}
	}

	t.Log("Multiple round-trips successful")
}

// Тест на очень длинный контент
func TestVeryLongContent(t *testing.T) {
	// Создаем очень длинный adm (100KB+)
	var longBuilder strings.Builder
	longBuilder.WriteString("<script>")
	for i := 0; i < 10000; i++ {
		longBuilder.WriteString(`window.open('https://example.com/page/`)
		longBuilder.WriteString(string(rune('a' + i%26)))
		longBuilder.WriteString("', '_blank');")
	}
	longBuilder.WriteString("</script>")

	longAdm := longBuilder.String()

	adid := AdmToAdidCompact(longAdm)
	recovered, err := AdidToAdmCompact(adid)

	if err != nil {
		t.Fatalf("Failed with very long content: %v", err)
	}

	if recovered != longAdm {
		t.Error("Very long content recovery failed")
	}

	t.Logf("Very long content: %d → %d bytes", len(longAdm), len(adid))
}

/*func TestMain(t *testing.T) {
	adm := "https://u-48702.daleelerah.info/api/rtb-pops/go?id=30931019512640203&sig=4d01045f858a4fb48e486f9151159b&u=aHR0cHM6Ly9kYWNsbGFkcy5jb20vZ2V0Lz9zcG90X2lkPTE0Mjc0NDMmY2F0PTI1JnN1YmlkPTE5NjI4MzAyNDcmdXRtX3NvdXJjZT17c291cmNlX2lkfSZ0Yl91cmw9aHR0cHMlM0ElMkYlMkZkYWxlZWxlcmFoLmluZm8lMkZwb3AtZ28lMkY1NDcwNw%3D%3D"
	adid := AdmToAdidCompact(adm)
	log.Print(adid)

	testAdm, err := AdidToAdmCompact(adid)
	if err != nil {
		log.Fatalf("PIZDEC")
	}

	if strings.EqualFold(adm, testAdm) {
		log.Printf("YES")
	}
}*/

func TestMain(t *testing.T) {

	adid := "1910097"
	testAdm, err := AdidToAdmCompact(adid)
	if err != nil {
		log.Fatalf("PIZDEC")
	}

	log.Print(testAdm)
}
