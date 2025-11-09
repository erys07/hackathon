package service

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	openai "github.com/sashabaranov/go-openai"

	"hackathon/model"
)

func WebhookHandler(oa *openai.Client, evo *EvolutionClient, store *ConversationStore, cfg *model.Config) http.HandlerFunc {
	if evo == nil {
		panic("WebhookHandler requires EvolutionClient")
	}

	if oa == nil {
		log.Print("WebhookHandler: openai client is nil, responses will be Echo mode")
	}

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

		log.Printf("webhook request: method=%s path=%s remote=%s", r.Method, r.URL.Path, r.RemoteAddr)
		log.Printf("webhook payload raw: %s", string(body))

		var payload model.WebhookPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			log.Printf("webhook decode error: %v", err)
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}

		ctx := r.Context()

		switch payload.Event {
		case "messages.upsert":
			if err := handleMessagesUpsert(ctx, payload, oa, evo, store, cfg); err != nil {
				log.Printf("handle messages.upsert error: %v", err)
			}
		case "message_create":
			var data model.WebhookData
			if err := json.Unmarshal(payload.Data, &data); err != nil {
				log.Printf("webhook message_create decode error: %v", err)
				break
			}

			if err := processWebhookMessage(ctx, oa, evo, store, cfg, payload.Sender, data.Message, data.Key); err != nil {
				log.Printf("handle message_create error: %v", err)
			}
		default:
			log.Printf("webhook ignoring event: %s", payload.Event)
		}

		w.WriteHeader(http.StatusOK)
	}
}

func handleMessagesUpsert(ctx context.Context, payload model.WebhookPayload, oa *openai.Client, evo *EvolutionClient, store *ConversationStore, cfg *model.Config) error {
	var container struct {
		Messages []model.MessagesUpsertEntry `json:"messages"`
	}

	if err := json.Unmarshal(payload.Data, &container); err == nil && len(container.Messages) > 0 {
		for _, entry := range container.Messages {
			if entry.Key.FromMe {
				continue
			}
			if err := processWebhookMessage(ctx, oa, evo, store, cfg, payload.Sender, entry.Message, entry.Key); err != nil {
				log.Printf("process messages.upsert entry error: %v", err)
			}
		}
		return nil
	}

	var single model.MessagesUpsertEntry
	if err := json.Unmarshal(payload.Data, &single); err != nil {
		return err
	}

	if single.Key.FromMe {
		return nil
	}

	return processWebhookMessage(ctx, oa, evo, store, cfg, payload.Sender, single.Message, single.Key)
}

func processWebhookMessage(ctx context.Context, oa *openai.Client, evo *EvolutionClient, store *ConversationStore, cfg *model.Config, sender string, msg model.WebhookMessage, key model.WebhookKey) error {
	text := extractMessageText(msg)
	if text == "" {
		return nil
	}

	if key.FromMe {
		return nil
	}

	recipient := chooseRecipient(key.RemoteJID, msg.From, sender)
	if recipient == "" {
		return nil
	}

	reply, err := generateAssistantReply(ctx, oa, store, cfg, recipient, text)
	if err != nil {
		return err
	}

	if reply == "" {
		return nil
	}

	if err := evo.SendTextMessage(ctx, recipient, reply); err != nil {
		return err
	}

	return nil
}

func generateAssistantReply(ctx context.Context, oa *openai.Client, store *ConversationStore, cfg *model.Config, recipient string, userInput string) (string, error) {
	normalizedID := normalizeWhatsAppID(recipient)
	if normalizedID == "" {
		return "", nil
	}

	var conversation []openai.ChatCompletionMessage
	if store != nil {
		stored, err := store.GetConversation(ctx, normalizedID)
		if err != nil {
			log.Printf("conversation load failed for %s: %v", normalizedID, err)
		} else {
			conversation = stored
		}
	}

	conversation = append(conversation, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userInput,
	})

	modelID := strings.TrimSpace(cfg.OpenAIModel)
	if modelID == "" {
		modelID = "gpt-4o-mini"
	}

	resp, err := oa.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    modelID,
		Messages: conversation,
	})
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		return "", nil
	}

	reply := strings.TrimSpace(resp.Choices[0].Message.Content)
	if reply == "" {
		return "", nil
	}

	conversation = append(conversation, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: reply,
	})

	if store != nil {
		if err := store.SaveConversation(ctx, normalizedID, conversation); err != nil {
			log.Printf("conversation save failed for %s: %v", normalizedID, err)
		}
	}

	return reply, nil
}

func extractMessageText(msg model.WebhookMessage) string {
	candidates := []string{
		msg.Body,
		msg.Text,
		msg.Conversation,
		msg.ExtendedText,
	}

	for _, candidate := range candidates {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			return trimmed
		}
	}

	if msg.ExtendedTextMessage != nil {
		if trimmed := strings.TrimSpace(msg.ExtendedTextMessage.Text); trimmed != "" {
			return trimmed
		}
	}

	if msg.ButtonsResponseMessage != nil {
		if trimmed := strings.TrimSpace(msg.ButtonsResponseMessage.SelectedDisplayText); trimmed != "" {
			return trimmed
		}
		if trimmed := strings.TrimSpace(msg.ButtonsResponseMessage.SelectedButtonID); trimmed != "" {
			return trimmed
		}
	}

	if msg.InteractiveResponseMessage != nil && msg.InteractiveResponseMessage.Body != nil {
		if trimmed := strings.TrimSpace(msg.InteractiveResponseMessage.Body.Text); trimmed != "" {
			return trimmed
		}
	}

	return ""
}

func chooseRecipient(values ...string) string {
	for _, value := range values {
		if normalized := normalizeWhatsAppID(value); normalized != "" {
			return normalized
		}
	}
	return ""
}

func normalizeWhatsAppID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}

	if idx := strings.IndexByte(id, '@'); idx >= 0 {
		id = id[:idx]
	}
	id = strings.TrimPrefix(id, "+")
	id = strings.TrimSpace(id)

	return id
}
