package service

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"hackathon/model"
)

func LoadConfig() (*model.Config, error) {
	cfg := &model.Config{
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

	cfg.RedisAddr = os.Getenv("REDIS_ADDR")
	cfg.RedisPassword = os.Getenv("REDIS_PASSWORD")

	if redisDB := os.Getenv("REDIS_DB"); redisDB != "" {
		parsedDB, err := strconv.Atoi(redisDB)
		if err != nil {
			return nil, fmt.Errorf("invalid REDIS_DB: %w", err)
		}
		cfg.RedisDB = parsedDB
	}

	if cfg.RedisAddr == "" {
		return nil, errors.New("missing redis configuration: REDIS_ADDR")
	}

	return cfg, nil
}
