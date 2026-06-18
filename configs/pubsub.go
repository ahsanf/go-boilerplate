package configs

import (
	"go-boilerplate/internal/utils"
	"context"
	"log"
	"os"

	"cloud.google.com/go/pubsub"
)

var PubSubClient *pubsub.Client

func InitPubSub(ctx context.Context) {
	projectID := os.Getenv("PUBSUB_PROJECT_ID")
	if projectID == "" {
		log.Fatal("PUBSUB_PROJECT_ID is not set")
	}

	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("failed to create PubSub client: %v", err)
	}

	PubSubClient = client
	utils.Logger.Info("connected to PubSub")
}
