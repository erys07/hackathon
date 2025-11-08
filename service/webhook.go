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

		defer r.Body.Close()

		var payload model.WebhookPayload
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

func handleIncomingMessage(ctx context.Context, oa *openai.Client, evo *EvolutionClient, cfg *model.Config, msg model.WebhookMessage) error {
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

	if err := evo.SendTextMessage(ctx, msg.From, reply); err != nil {
		return fmt.Errorf("send text: %w", err)
	}

	audioData, err := synthesizeAudio(ctx, oa, cfg.OpenAIVoice, reply)
	if err != nil {
		log.Printf("audio synthesis failed: %v", err)
		return nil
	}

	if err := evo.SendAudioMessage(ctx, msg.From, audioData); err != nil {
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
