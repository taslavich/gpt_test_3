package utils

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
)

const (
	NURL = "nurl"
	BURL = "burl"
	ADM  = "adm"
)

func WrapURL(hostname, originalURL, globalId, admOrnurlOrBurl string) string {
	encodeUrl := base64.URLEncoding.EncodeToString([]byte(originalURL))
	return fmt.Sprintf("https://%s/%s?id=%s&url=%s",
		hostname, admOrnurlOrBurl, globalId, encodeUrl)
}

func CompressString(s string) string {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write([]byte(s))
	gz.Close()
	return base64.URLEncoding.EncodeToString(buf.Bytes())
}

func DecompressString(s string) (string, error) {
	data, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	defer gz.Close()
	result, _ := io.ReadAll(gz)
	return string(result), nil
}
