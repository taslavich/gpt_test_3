package services

import (
	"ai-prelander-builder/backend/internal/config"
	"ai-prelander-builder/backend/internal/models"
	"html/template"
	"os"
	"path/filepath"
)

type RenderInput struct {
	PrelanderID, TemplateName string
	Content                   models.PrelanderContent
	VisualPath, OfferURL      string
}
type templateData struct {
	Content              models.PrelanderContent
	VisualPath, OfferURL string
}

func RenderPrelander(input RenderInput) (string, error) {
	tpl, err := template.ParseFiles(filepath.Join("templates/prelanders", input.TemplateName+".html"))
	if err != nil {
		return "", err
	}
	dir := filepath.Join(config.GeneratedDir, input.PrelanderID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	f, err := os.Create(filepath.Join(dir, "index.html"))
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := tpl.Execute(f, templateData{input.Content, input.VisualPath, input.OfferURL}); err != nil {
		return "", err
	}
	return config.BaseURL + "/preview/" + input.PrelanderID, nil
}
