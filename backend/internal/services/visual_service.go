package services

import (
	"fmt"
	"html"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func safeName(name string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, name)
}

func HandleUploadedVisual(file multipart.File, header *multipart.FileHeader) (string, error) {
	defer file.Close()
	if err := os.MkdirAll("static/uploads", 0755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%d_%s", time.Now().UnixNano(), safeName(header.Filename))
	path := filepath.Join("static/uploads", name)
	out, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		return "", err
	}
	return "/static/uploads/" + name, nil
}

func GenerateVisual(vertical, geo, language, style string) (string, error) {
	if err := os.MkdirAll("static/assets/generated_visuals", 0755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s_%s_%s_%d.svg", safeName(vertical), safeName(geo), safeName(style), time.Now().UnixNano())
	path := filepath.Join("static/assets/generated_visuals", name)
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="900" height="1200" viewBox="0 0 900 1200"><defs><linearGradient id="g" x1="0" x2="1" y1="0" y2="1"><stop stop-color="#7c3aed"/><stop offset="1" stop-color="#06b6d4"/></linearGradient></defs><rect width="900" height="1200" fill="url(#g)"/><circle cx="710" cy="190" r="170" fill="rgba(255,255,255,.18)"/><circle cx="160" cy="980" r="220" fill="rgba(0,0,0,.16)"/><text x="50%%" y="45%%" text-anchor="middle" fill="white" font-size="78" font-family="Arial" font-weight="700">%s</text><text x="50%%" y="53%%" text-anchor="middle" fill="white" font-size="42" font-family="Arial">%s · %s · %s</text></svg>`, html.EscapeString(strings.ToUpper(vertical)), html.EscapeString(geo), html.EscapeString(language), html.EscapeString(style))
	return "/static/assets/generated_visuals/" + name, os.WriteFile(path, []byte(svg), 0644)
}
