package main

import (
	"log"
	"net/http"

	"github.com/joho/godotenv"
	openai "github.com/sashabaranov/go-openai"

	"hackathon/service"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("config: .env not found, relying on environment variables (%v)", err)
	}

	cfg, err := service.LoadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	openaiClient := openai.NewClient(cfg.OpenAIAPIKey)
	evoClient := service.NewEvolutionClient(cfg)
	conversationStore, err := service.NewConversationStore(cfg)
	if err != nil {
		log.Fatalf("redis error: %v", err)
	}
	defer conversationStore.Close()

	http.HandleFunc("/webhook", service.WebhookHandler(openaiClient, evoClient, conversationStore, cfg))

	addr := ":8080"
	log.Printf("server listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
