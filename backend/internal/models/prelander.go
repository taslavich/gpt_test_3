package models

type Question struct {
	Text    string
	Answers []string
}

type PrelanderContent struct {
	Headline    string
	Subheadline string
	Body        string
	Questions   []Question
	CTA         string
	Disclaimer  string
	ResultText  string
	Style       string
}

type PrelanderMeta struct {
	PrelanderID  string `json:"prelander_id"`
	GenerationID string `json:"generation_id"`
	Vertical     string `json:"vertical"`
	GEO          string `json:"geo"`
	Language     string `json:"language"`
	Style        string `json:"style"`
	OfferURL     string `json:"offer_url"`
	VisualPath   string `json:"visual_path"`
	PreviewURL   string `json:"preview_url"`
	CreatedAt    string `json:"created_at"`
}

type GenerateResponse struct {
	GenerationID string          `json:"generation_id"`
	Items        []PrelanderMeta `json:"items"`
}
