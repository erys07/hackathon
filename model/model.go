package model

import "encoding/json"

type Config struct {
	EvolutionAPIURL   string
	EvolutionAPIKey   string
	EvolutionInstance string
	OpenAIAPIKey      string
	OpenAIVoice       string
	OpenAIModel       string
	RedisAddr         string
	RedisPassword     string
	RedisDB           int
}

type WebhookPayload struct {
	Event       string          `json:"event"`
	Instance    string          `json:"instance"`
	Data        json.RawMessage `json:"data"`
	Destination string          `json:"destination"`
	DateTime    string          `json:"date_time"`
	Sender      string          `json:"sender"`
	ServerURL   string          `json:"server_url"`
	APIKey      string          `json:"apikey"`
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
	From                       string                      `json:"from"`
	ChatID                     string                      `json:"chatId"`
	RemoteJID                  string                      `json:"remoteJid"`
	Type                       string                      `json:"type"`
	Body                       string                      `json:"body"`
	Conversation               string                      `json:"conversation"`
	ExtendedText               string                      `json:"extendedText"`
	Text                       string                      `json:"text"`
	Audio                      *WebhookAudio               `json:"audio,omitempty"`
	MessageSender              string                      `json:"sender,omitempty"`
	ExtendedTextMessage        *ExtendedTextMessage        `json:"extendedTextMessage,omitempty"`
	ButtonsResponseMessage     *ButtonsResponseMessage     `json:"buttonsResponseMessage,omitempty"`
	InteractiveResponseMessage *InteractiveResponseMessage `json:"interactiveResponseMessage,omitempty"`
}

type WebhookAudio struct {
	URL string `json:"url"`
}

type ExtendedTextMessage struct {
	Text string `json:"text"`
}

type ButtonsResponseMessage struct {
	SelectedButtonID    string `json:"selectedButtonId"`
	SelectedDisplayText string `json:"selectedDisplayText"`
}

type InteractiveResponseMessage struct {
	Body *InteractiveBody `json:"body"`
}

type InteractiveBody struct {
	Text string `json:"text"`
}

type MessagesUpsertData struct {
	Messages   []MessagesUpsertEntry `json:"messages"`
	Type       string                `json:"type"`
	InstanceID string                `json:"instanceId"`
}

type MessagesUpsertEntry struct {
	Key              WebhookKey     `json:"key"`
	Message          WebhookMessage `json:"message"`
	MessageType      string         `json:"messageType"`
	MessageTimestamp int64          `json:"messageTimestamp"`
	PushName         string         `json:"pushName"`
}
