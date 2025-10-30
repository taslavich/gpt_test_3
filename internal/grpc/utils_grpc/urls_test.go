package utils

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"testing"
)

func TestWrapURLForAdmUsesBase64URL(t *testing.T) {
	host := CallbackHost{Scheme: "https", Host: "twinbidexchange.com", Port: 8086}
	original := "https://example.com/click?ad=1&user=2"
	encoded := base64.RawURLEncoding.EncodeToString([]byte(original))

	got := WrapURL(host, original, "global-1", ADM)
	want := fmt.Sprintf("https://twinbidexchange.com:8086/%s?id=%s&url=%s", ADM, "global-1", encoded)

	if got != want {
		t.Fatalf("WrapURL() = %s, want %s", got, want)
	}

	decoded, err := DecodeWrappedURL(ADM, encoded)
	if err != nil {
		t.Fatalf("DecodeWrappedURL() unexpected error: %v", err)
	}

	if decoded != original {
		t.Fatalf("DecodeWrappedURL() = %s, want %s", decoded, original)
	}
}

func TestWrapURLDefaultPortSkipped(t *testing.T) {
	host := CallbackHost{Scheme: "https", Host: "secure.example.com", Port: 443}
	original := "https://example.com"

	got := WrapURL(host, original, "gid", NURL)
	want := fmt.Sprintf("https://secure.example.com/%s?id=%s&url=%s", NURL, "gid", url.QueryEscape(original))

	if got != want {
		t.Fatalf("WrapURL() = %s, want %s", got, want)
	}

	decoded, err := DecodeWrappedURL(NURL, url.QueryEscape(original))
	if err != nil {
		t.Fatalf("DecodeWrappedURL() unexpected error: %v", err)
	}

	if decoded != original {
		t.Fatalf("DecodeWrappedURL() = %s, want %s", decoded, original)
	}
}

func TestWrapURLFallbackToEncodedURL(t *testing.T) {
	host := CallbackHost{}
	original := "https://example.com"

	got := WrapURL(host, original, "gid", BURL)
	want := original

	if got != want {
		t.Fatalf("WrapURL() = %s, want %s", got, want)
	}
}
