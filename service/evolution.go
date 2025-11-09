package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"hackathon/model"
)

type EvolutionClient struct {
	baseURL    string
	apiKey     string
	instance   string
	httpClient *http.Client
}

func NewEvolutionClient(cfg *model.Config) *EvolutionClient {
	return &EvolutionClient{
		baseURL:    cfg.EvolutionAPIURL,
		apiKey:     cfg.EvolutionAPIKey,
		instance:   cfg.EvolutionInstance,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (e *EvolutionClient) SendTextMessage(ctx context.Context, to, message string) error {
	payload := map[string]string{
		"number": to,
		"text":   message,
	}

	return e.postJSON(ctx, fmt.Sprintf("%s/message/sendText/%s", e.baseURL, e.instance), payload)
}

func (e *EvolutionClient) SendAudioMessage(ctx context.Context, to string, audio []byte) error {
	base64Audio := base64.StdEncoding.EncodeToString(audio)
	sample := base64Audio
	if len(sample) > 64 {
		sample = fmt.Sprintf("%s...%s", sample[:32], sample[len(sample)-16:])
	}

	log.Printf("SendAudioMessage: mediatype=audio to=%s base64_len=%d sample=%s", to, len(base64Audio), sample)

	payloads := []map[string]any{
		{
			"number":    to,
			"mediatype": "audio",
			"data":      base64Audio,
		},
		{
			"number":    to,
			"mediatype": "audio",
			"file":      base64Audio,
		},
		{
			"number":    to,
			"mediatype": "audio",
			"media":     base64Audio,
		},
		{
			"number":    to,
			"mediatype": "audio",
			"mediaData": map[string]any{
				"base64": base64Audio,
			},
		},
		{
			"number": to,
			"mediaMessage": map[string]any{
				"mediaType": "audio",
				"media":     base64Audio,
			},
		},
	}

	for idx, payload := range payloads {
		payloadJSON, _ := json.Marshal(payload)
		log.Printf("SendAudioMessage: attempt %d payload keys=%v body=%s", idx+1, mapKeys(payload), truncate(string(payloadJSON), 256))

		err := e.postJSON(ctx, fmt.Sprintf("%s/message/sendMedia/%s", e.baseURL, e.instance), payload)
		if err == nil {
			return nil
		}

		errMsg := err.Error()
		log.Printf("SendAudioMessage: attempt %d error=%v", idx+1, errMsg)

		if !strings.Contains(strings.ToLower(errMsg), "owned media must be a url or base64") || idx == len(payloads)-1 {
			return err
		}

		log.Printf("SendAudioMessage: attempt %d failed with owned media error, trying next payload shape", idx+1)
	}

	return nil
}

func (e *EvolutionClient) postJSON(ctx context.Context, url string, body any) error {
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

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}

	return fmt.Sprintf("%s...%s", s[:max/2], s[len(s)-max/2:])
}
