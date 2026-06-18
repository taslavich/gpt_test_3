package handlers

import (
	"ai-prelander-builder/backend/internal/models"
	"ai-prelander-builder/backend/internal/services"
	"ai-prelander-builder/backend/internal/validation"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

var styles = []string{"quiz", "urgency", "native_article", "minimal_confirm", "calculator"}

func id(prefix string) string {
	b := make([]byte, 5)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

func GeneratePrelanders(c *gin.Context) {
	vertical, geo, language := c.PostForm("vertical"), c.PostForm("geo"), c.PostForm("language")
	offerURL, visualMode := c.PostForm("offer_url"), c.PostForm("visual_mode")
	count, err := strconv.Atoi(c.DefaultPostForm("variants_count", "5"))
	if err != nil {
		count = 5
	}
	fileHeader, _ := c.FormFile("uploaded_visual")
	if err := validation.GenerateRequest(vertical, geo, language, offerURL, visualMode, count, fileHeader); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	generationID := id("gen")
	items := make([]models.PrelanderMeta, 0, count)
	for i := 0; i < count; i++ {
		style := styles[i%len(styles)]
		var visualPath string
		if visualMode == "upload" {
			f, err := fileHeader.Open()
			if err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			visualPath, err = services.HandleUploadedVisual(f, fileHeader)
			if err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
		} else {
			visualPath, err = services.GenerateVisual(vertical, geo, language, style)
			if err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
		}
		plID := id("pl")
		content := services.GenerateContent(vertical, geo, language, style)
		previewURL, err := services.RenderPrelander(services.RenderInput{PrelanderID: plID, TemplateName: style, Content: content, VisualPath: visualPath, OfferURL: offerURL})
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		items = append(items, models.PrelanderMeta{PrelanderID: plID, GenerationID: generationID, Vertical: vertical, GEO: geo, Language: language, Style: style, OfferURL: offerURL, VisualPath: visualPath, PreviewURL: previewURL, CreatedAt: time.Now().UTC().Format(time.RFC3339)})
	}
	if err := services.AppendPrelanders(items); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.GenerateResponse{GenerationID: generationID, Items: items})
}

func ListPrelanders(c *gin.Context) {
	items, err := services.ReadAllPrelanders()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, items)
}
func Preview(c *gin.Context) { c.File("generated/" + c.Param("prelander_id") + "/index.html") }
