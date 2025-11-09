package service

import (
	"bytes"
	"context"
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
	payload := map[string]any{
		"number": to,
		"text":   message,
	}

	return e.postJSON(ctx, fmt.Sprintf("%s/message/sendText/%s", e.baseURL, e.instance), payload)
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

	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	log.Printf("Evolution API response: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(responseBody)))

	if resp.StatusCode >= 300 {
		return fmt.Errorf("evolution API error: %s - %s", resp.Status, strings.TrimSpace(string(responseBody)))
	}

	return nil
}
