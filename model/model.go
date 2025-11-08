package model

type Config struct {
	EvolutionAPIURL   string
	EvolutionAPIKey   string
	EvolutionInstance string
	OpenAIAPIKey      string
	OpenAIVoice       string
	OpenAIModel       string
}

type WebhookPayload struct {
	Message WebhookMessage `json:"message"`
}

type WebhookMessage struct {
	From  string        `json:"from"`
	Type  string        `json:"type"`
	Body  string        `json:"body"`
	Audio *WebhookAudio `json:"audio,omitempty"`
}

type WebhookAudio struct {
	URL string `json:"url"`
}
