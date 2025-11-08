package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	openai "github.com/sashabaranov/go-openai"

	"hackathon/model"
)

func WebhookHandler(oa *openai.Client, evo *EvolutionClient, cfg *model.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("webhook read body error: %v", err)
			http.Error(w, "failed to read payload", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var payload model.WebhookPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			log.Printf("webhook decode error: %v", err)
			log.Printf("webhook raw payload: %s", string(body))
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		if err := handleIncomingMessage(ctx, oa, evo, cfg, payload.Data, string(body)); err != nil {
			log.Printf("webhook handling error: %v", err)
			http.Error(w, "failed to process message", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func handleIncomingMessage(ctx context.Context, oa *openai.Client, evo *EvolutionClient, cfg *model.Config, data model.WebhookData, rawBody string) error {
	msg := data.Message

	sender := resolveSender(data)
	if sender == "" {
		log.Printf("handleIncomingMessage: missing sender id in payload: %s", rawBody)
		return errors.New("missing sender id")
	}

	userInput := extractMessageBody(msg)

	messageType := strings.ToLower(strings.TrimSpace(msg.Type))
	if messageType == "" {
		messageType = strings.ToLower(strings.TrimSpace(data.MessageType))
	}

	if messageType == "audio" || messageType == "ptt" {
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

	if err := evo.SendTextMessage(ctx, sender, reply); err != nil {
		return fmt.Errorf("send text: %w", err)
	}

	audioData, err := synthesizeAudio(ctx, oa, cfg.OpenAIVoice, reply)
	if err != nil {
		log.Printf("audio synthesis failed: %v", err)
		return nil
	}

	if err := evo.SendAudioMessage(ctx, sender, audioData); err != nil {
		log.Printf("send audio failed: %v", err)
	}

	return nil
}

func extractMessageBody(msg model.WebhookMessage) string {
	candidates := []string{
		msg.Body,
		msg.Conversation,
		msg.Text,
		msg.ExtendedText,
	}

	for _, candidate := range candidates {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			return trimmed
		}
	}

	return ""
}

func resolveSender(data model.WebhookData) string {
	candidates := []string{
		data.Message.From,
		data.Sender,
		data.Message.RemoteJID,
		data.Message.ChatID,
		data.Key.RemoteJID,
		data.RemoteJID,
		data.ChatID,
	}

	for _, candidate := range candidates {
		if normalized := normalizeRemoteID(candidate); normalized != "" {
			return normalized
		}
	}

	return ""
}

func normalizeRemoteID(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return ""
	}

	if strings.Contains(trimmed, "@") {
		parts := strings.Split(trimmed, "@")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	return trimmed
}

func buildChatReply(ctx context.Context, oa *openai.Client, prompt string) (string, error) {
	resp, err := oa.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT4o,
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
