package services

import (
	"ai-prelander-builder/backend/internal/config"
	"ai-prelander-builder/backend/internal/models"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func ReadAllPrelanders() ([]models.PrelanderMeta, error) {
	if err := os.MkdirAll(filepath.Dir(config.DataFile), 0755); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(config.DataFile)
	if errors.Is(err, os.ErrNotExist) || len(b) == 0 {
		return []models.PrelanderMeta{}, nil
	}
	if err != nil {
		return nil, err
	}
	var items []models.PrelanderMeta
	if err := json.Unmarshal(b, &items); err != nil {
		return []models.PrelanderMeta{}, nil
	}
	return items, nil
}
func AppendPrelanders(newItems []models.PrelanderMeta) error {
	items, err := ReadAllPrelanders()
	if err != nil {
		return err
	}
	items = append(newItems, items...)
	b, _ := json.MarshalIndent(items, "", "  ")
	return os.WriteFile(config.DataFile, b, 0644)
}
