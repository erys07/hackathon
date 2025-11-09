package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	openai "github.com/sashabaranov/go-openai"

	"hackathon/model"
)

type ConversationStore struct {
	client      *redis.Client
	ttl         time.Duration
	maxMessages int
}

func NewConversationStore(cfg *model.Config) (*ConversationStore, error) {
	options := &redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	}

	client := redis.NewClient(options)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("connect redis: %w", err)
	}

	return &ConversationStore{
		client:      client,
		ttl:         24 * time.Hour,
		maxMessages: 20,
	}, nil
}

func (s *ConversationStore) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

func (s *ConversationStore) GetConversation(ctx context.Context, user string) ([]openai.ChatCompletionMessage, error) {
	if s == nil {
		return nil, nil
	}

	key := s.key(user)
	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var messages []openai.ChatCompletionMessage
	if len(data) == 0 {
		return messages, nil
	}

	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, fmt.Errorf("decode conversation: %w", err)
	}

	return messages, nil
}

func (s *ConversationStore) SaveConversation(ctx context.Context, user string, messages []openai.ChatCompletionMessage) error {
	if s == nil {
		return nil
	}

	if len(messages) > s.maxMessages {
		messages = messages[len(messages)-s.maxMessages:]
	}

	payload, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("encode conversation: %w", err)
	}

	return s.client.Set(ctx, s.key(user), payload, s.ttl).Err()
}

func (s *ConversationStore) key(user string) string {
	return fmt.Sprintf("conversation:%s", user)
}
