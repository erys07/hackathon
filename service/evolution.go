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

func (e *EvolutionClient) GetQRCode(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/instance/qrcode/%s", e.baseURL, e.instance), nil)
	if err != nil {
		return err
	}

	req.Header.Set("apikey", e.apiKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	qrCode, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	log.Printf("QR Code response: %s - %s", resp.Status, string(qrCode))

	return nil
}

func (e *EvolutionClient) ConnectInstance(ctx context.Context) error {
	// Primeiro, tentar desconectar para garantir um estado limpo
	disconnectReq, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/instance/logout/%s", e.baseURL, e.instance), nil)
	if err != nil {
		return err
	}
	disconnectReq.Header.Set("apikey", e.apiKey)
	_, _ = e.httpClient.Do(disconnectReq) // Ignoramos erros aqui

	time.Sleep(2 * time.Second) // Aguarda um pouco para garantir que a desconexão foi processada

	// Tenta obter um novo QR code
	if err := e.GetQRCode(ctx); err != nil {
		log.Printf("Failed to get QR code: %v", err)
	}

	// Agora tenta conectar
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/instance/connect/%s", e.baseURL, e.instance), nil)
	if err != nil {
		return err
	}

	req.Header.Set("apikey", e.apiKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	log.Printf("Instance connect response: %s - %s", resp.Status, strings.TrimSpace(string(responseBody)))

	if resp.StatusCode >= 300 {
		return fmt.Errorf("evolution API error: %s - %s", resp.Status, strings.TrimSpace(string(responseBody)))
	}

	return nil
}

func (e *EvolutionClient) CheckInstanceStatus(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/instance/connectionState/%s", e.baseURL, e.instance), nil)
	if err != nil {
		return err
	}

	req.Header.Set("apikey", e.apiKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	var statusResp struct {
		Instance struct {
			InstanceName string `json:"instanceName"`
			State       string `json:"state"`
		} `json:"instance"`
	}
	if err := json.Unmarshal(responseBody, &statusResp); err != nil {
		log.Printf("Failed to parse status response: %v", err)
		return err
	}

	log.Printf("Instance status: %s - State: %s", resp.Status, statusResp.Instance.State)

	// Verificar o status específico do WhatsApp
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/instance/status/%s", e.baseURL, e.instance), nil)
	if err != nil {
		return err
	}
	req.Header.Set("apikey", e.apiKey)

	resp, err = e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	whatsappStatus, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	log.Printf("WhatsApp connection status: %s - %s", resp.Status, string(whatsappStatus))

	if !strings.Contains(strings.ToLower(string(whatsappStatus)), "connected") {
		log.Printf("WhatsApp not connected, attempting to reconnect...")
		return e.ConnectInstance(ctx)
	}

	return nil
}

func (e *EvolutionClient) SendAudioMessage(ctx context.Context, to string, audio []byte) error {
	// Primeiro, verifica o status da instância
	if err := e.CheckInstanceStatus(ctx); err != nil {
		return fmt.Errorf("failed to check instance status: %v", err)
	}

	base64Audio := base64.StdEncoding.EncodeToString(audio)
	sample := base64Audio
	if len(sample) > 64 {
		sample = fmt.Sprintf("%s...%s", sample[:32], sample[len(sample)-16:])
	}

	log.Printf("SendAudioMessage: mediatype=audio to=%s base64_len=%d sample=%s", to, len(base64Audio), sample)

	payload := map[string]any{
		"number":    to,
		"mediatype": "audio",
		"media":     base64Audio,
	}

	payloadJSON, _ := json.Marshal(payload)
	log.Printf("SendAudioMessage: payload=%s", truncate(string(payloadJSON), 256))

	err := e.postJSON(ctx, fmt.Sprintf("%s/message/sendMedia/%s", e.baseURL, e.instance), payload)
	if err != nil {
		log.Printf("SendAudioMessage: error=%v", err)
		return err
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

	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	log.Printf("Evolution API response: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(responseBody)))

	if resp.StatusCode >= 300 {
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
