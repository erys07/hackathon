package service

import (
	"errors"
	"os"
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

	return cfg, nil
}
