package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	openai "github.com/sashabaranov/go-openai"
)

type config struct {
	EvolutionAPIURL   string
	EvolutionAPIKey   string
	EvolutionInstance string
	OpenAIAPIKey      string
	OpenAIVoice       string
}

type evolutionClient struct {
	baseURL    string
	apiKey     string
	instance   string
	httpClient *http.Client
}

type webhookPayload struct {
	Message webhookMessage `json:"message"`
}

type webhookMessage struct {
	From  string        `json:"from"`
	Type  string        `json:"type"`
	Body  string        `json:"body"`
	Audio *webhookAudio `json:"audio,omitempty"`
}

type webhookAudio struct {
	URL string `json:"url"`
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("config: .env not found, relying on environment variables (%v)", err)
	}

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	openaiClient := openai.NewClient(cfg.OpenAIAPIKey)
	evoClient := newEvolutionClient(cfg)

	http.HandleFunc("/webhook", webhookHandler(openaiClient, evoClient, cfg))

	addr := ":8080"
	log.Printf("server listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func loadConfig() (*config, error) {
	cfg := &config{
		EvolutionAPIURL:   strings.TrimSuffix(os.Getenv("EVOLUTION_API_URL"), "/"),
		EvolutionAPIKey:   os.Getenv("EVOLUTION_API_KEY"),
		EvolutionInstance: os.Getenv("EVOLUTION_INSTANCE"),
		OpenAIAPIKey:      os.Getenv("OPENAI_API_KEY"),
		OpenAIVoice:       os.Getenv("OPENAI_VOICE"),
	}

	if cfg.EvolutionAPIURL == "" || cfg.EvolutionAPIKey == "" || cfg.EvolutionInstance == "" || cfg.OpenAIAPIKey == "" {
		return nil, errors.New("missing required environment variables")
	}

	if cfg.OpenAIVoice == "" {
		cfg.OpenAIVoice = "alloy"
	}

	return cfg, nil
}

func newEvolutionClient(cfg *config) *evolutionClient {
	return &evolutionClient{
		baseURL:    cfg.EvolutionAPIURL,
		apiKey:     cfg.EvolutionAPIKey,
		instance:   cfg.EvolutionInstance,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func webhookHandler(oa *openai.Client, evo *evolutionClient, cfg *config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		defer r.Body.Close()

		var payload webhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			log.Printf("webhook decode error: %v", err)
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		if err := handleIncomingMessage(ctx, oa, evo, cfg, payload.Message); err != nil {
			log.Printf("webhook handling error: %v", err)
			http.Error(w, "failed to process message", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func handleIncomingMessage(ctx context.Context, oa *openai.Client, evo *evolutionClient, cfg *config, msg webhookMessage) error {
	if msg.From == "" {
		return errors.New("missing sender id")
	}

	userInput := strings.TrimSpace(msg.Body)

	if msg.Type == "audio" || msg.Type == "ptt" {
		if msg.Audio == nil || msg.Audio.URL == "" {
			return errors.New("audio message missing url")
		}

		transcribed, err := transcribeAudio(ctx, oa, msg.Audio.URL)
		if err != nil {
			return fmt.Errorf("transcribe audio: %w", err)
		}
		userInput = transcribed
	}

	if userInput == "" {
		return errors.New("empty message content")
	}

	reply, err := buildChatReply(ctx, oa, userInput)
	if err != nil {
		return fmt.Errorf("chat completion: %w", err)
	}

	if err := evo.sendTextMessage(ctx, msg.From, reply); err != nil {
		return fmt.Errorf("send text: %w", err)
	}

	audioData, err := synthesizeAudio(ctx, oa, cfg.OpenAIVoice, reply)
	if err != nil {
		log.Printf("audio synthesis failed: %v", err)
		return nil
	}

	if err := evo.sendAudioMessage(ctx, msg.From, audioData); err != nil {
		log.Printf("send audio failed: %v", err)
	}

	return nil
}

func buildChatReply(ctx context.Context, oa *openai.Client, prompt string) (string, error) {
	resp, err := oa.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT4oMini,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: "Você é um assistente do WhatsApp. Responda de forma breve e amigável.",
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
	})
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("chat completion returned no choices")
	}

	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

func transcribeAudio(ctx context.Context, oa *openai.Client, audioURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, audioURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("download audio: status %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	audioReq := openai.AudioRequest{
		Model:    openai.Whisper1,
		FilePath: "audio.ogg",
		Reader:   bytes.NewReader(data),
		Format:   openai.AudioResponseFormatText,
	}

	transcription, err := oa.CreateTranscription(ctx, audioReq)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(transcription.Text), nil
}

func synthesizeAudio(ctx context.Context, oa *openai.Client, voice, text string) ([]byte, error) {
	req := openai.CreateSpeechRequest{
		Model:          openai.TTSModel1,
		Voice:          mapSpeechVoice(voice),
		Input:          text,
		ResponseFormat: openai.SpeechResponseFormatMp3,
	}

	resp, err := oa.CreateSpeech(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Close()

	audio, err := io.ReadAll(resp)
	if err != nil {
		return nil, err
	}

	return audio, nil
}

func mapSpeechVoice(voice string) openai.SpeechVoice {
	switch strings.ToLower(voice) {
	case "alloy", "":
		return openai.VoiceAlloy
	case "echo":
		return openai.VoiceEcho
	case "fable":
		return openai.VoiceFable
	case "onyx":
		return openai.VoiceOnyx
	case "nova":
		return openai.VoiceNova
	case "shimmer":
		return openai.VoiceShimmer
	default:
		return openai.VoiceAlloy
	}
}

func (e *evolutionClient) sendTextMessage(ctx context.Context, to, message string) error {
	payload := map[string]string{
		"number": to,
		"text":   message,
	}

	return e.postJSON(ctx, fmt.Sprintf("%s/message/sendText/%s", e.baseURL, e.instance), payload)
}

func (e *evolutionClient) sendAudioMessage(ctx context.Context, to string, audio []byte) error {
	base64Audio := base64.StdEncoding.EncodeToString(audio)

	payload := map[string]string{
		"number":  to,
		"base64":  base64Audio,
		"mime":    "audio/mpeg",
		"caption": "Resposta em áudio",
	}

	return e.postJSON(ctx, fmt.Sprintf("%s/message/sendAudio/%s", e.baseURL, e.instance), payload)
}

func (e *evolutionClient) postJSON(ctx context.Context, url string, body any) error {
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, buf)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", e.apiKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("evolution API error: %s - %s", resp.Status, strings.TrimSpace(string(responseBody)))
	}

	return nil
}
