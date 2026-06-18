package services

import "ai-prelander-builder/backend/internal/models"

type copySet struct {
	Headline, Subheadline, Body, CTA, ResultText string
	Questions                                    []models.Question
}

var en = map[string]copySet{
	"dating":      {"People near you are looking for a chat", "Answer a few questions to continue", "This short preview helps match the next page to your preferences.", "Continue", "Profiles can be shown after confirmation.", []models.Question{{"Are you over 18?", []string{"Yes", "No"}}, {"Are you single?", []string{"Yes", "Prefer not to say"}}, {"Do you want to see profiles nearby?", []string{"Yes", "Continue"}}}},
	"sweepstakes": {"You may be eligible for a reward", "Answer 3 questions to continue", "Availability can vary by location and partner rules.", "Check availability", "Your answers help check available options.", []models.Question{{"Are you located in your country?", []string{"Yes", "No"}}, {"Have you participated before?", []string{"No", "Yes"}}, {"Choose your preferred reward", []string{"Gift card", "Device", "Deals"}}}},
	"crypto":      {"Check your potential result", "Answer a few questions to calculate your estimate", "This is an educational estimate only and not financial advice.", "Continue", "Your estimate is ready to review on the next page.", []models.Question{{"What is your experience level?", []string{"Beginner", "Intermediate", "Advanced"}}, {"What monthly amount are you interested in?", []string{"Low", "Medium", "High"}}, {"Choose your risk level", []string{"Lower", "Balanced", "Higher"}}}},
	"nutra":       {"Check your personal recommendation", "Answer a few questions", "Get a general wellness-oriented recommendation without medical guarantees.", "Get recommendation", "A recommendation can be prepared based on your answers.", []models.Question{{"What is your main goal?", []string{"Energy", "Routine", "Wellness"}}, {"How old are you?", []string{"18-34", "35-54", "55+"}}, {"How fast do you want to see changes?", []string{"Gradual", "Soon", "No rush"}}}},
	"utilities":   {"Your device may need optimization", "Run a quick check to continue", "A quick helper can review common performance preferences.", "Start check", "The quick check is ready to continue.", []models.Question{{"What device are you using?", []string{"Android", "iOS", "Desktop"}}, {"Is your device running slowly?", []string{"Sometimes", "Often", "Not sure"}}, {"Do you want to continue?", []string{"Yes", "Later"}}}},
}

var localized = map[string]map[string]string{
	"de": {"Continue": "Weiter", "Check availability": "Verfügbarkeit prüfen", "Get recommendation": "Empfehlung erhalten", "Start check": "Prüfung starten", "Answer a few questions": "Beantworte ein paar Fragen"},
	"fr": {"Continue": "Continuer", "Check availability": "Vérifier la disponibilité", "Get recommendation": "Obtenir une recommandation", "Start check": "Démarrer la vérification"},
	"es": {"Continue": "Continuar", "Check availability": "Comprobar disponibilidad", "Get recommendation": "Obtener recomendación", "Start check": "Iniciar revisión"},
	"it": {"Continue": "Continua", "Check availability": "Verifica disponibilità", "Get recommendation": "Ricevi consiglio", "Start check": "Avvia controllo"},
	"pt": {"Continue": "Continuar", "Check availability": "Verificar disponibilidade", "Get recommendation": "Obter recomendação", "Start check": "Iniciar verificação"},
	"ru": {"Continue": "Продолжить", "Check availability": "Проверить доступность", "Get recommendation": "Получить рекомендацию", "Start check": "Начать проверку", "Answer a few questions": "Ответьте на несколько вопросов"},
}

func GenerateContent(vertical, geo, language, style string) models.PrelanderContent {
	c, ok := en[vertical]
	if !ok {
		c = en["dating"]
	}
	if dict := localized[language]; dict != nil {
		if v := dict[c.CTA]; v != "" {
			c.CTA = v
		}
		if v := dict[c.Subheadline]; v != "" {
			c.Subheadline = v
		}
	}
	return models.PrelanderContent{Headline: c.Headline, Subheadline: c.Subheadline, Body: c.Body + " GEO: " + geo + ".", Questions: c.Questions, CTA: c.CTA, Disclaimer: "Advertising preview. Results and availability are not guaranteed. This page is not financial or medical advice.", ResultText: c.ResultText, Style: style}
}
