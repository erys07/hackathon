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
	Event       string      `json:"event"`
	Instance    string      `json:"instance"`
	Data        WebhookData `json:"data"`
	Destination string      `json:"destination"`
	DateTime    string      `json:"date_time"`
}

type WebhookData struct {
	Sender      string         `json:"sender"`
	RemoteJID   string         `json:"remoteJid"`
	ChatID      string         `json:"chatId"`
	Message     WebhookMessage `json:"message"`
	MessageType string         `json:"messageType"`
	Key         WebhookKey     `json:"key"`
}

type WebhookKey struct {
	RemoteJID string `json:"remoteJid"`
	FromMe    bool   `json:"fromMe"`
	ID        string `json:"id"`
}

type WebhookMessage struct {
	From          string        `json:"from"`
	ChatID        string        `json:"chatId"`
	RemoteJID     string        `json:"remoteJid"`
	Type          string        `json:"type"`
	Body          string        `json:"body"`
	Conversation  string        `json:"conversation"`
	ExtendedText  string        `json:"extendedText"`
	Text          string        `json:"text"`
	Audio         *WebhookAudio `json:"audio,omitempty"`
	MessageSender string        `json:"sender,omitempty"`
}

type WebhookAudio struct {
	URL string `json:"url"`
}
