package configs

import (
	"context"

	"go-boilerplate/internal/utils"
	"cloud.google.com/go/pubsub/v2"
	"go.uber.org/zap"
)

var PubSubClient *pubsub.Client

func InitPubSub(ctx context.Context) {
	if Cfg.PubSubProjectID == "" {
		utils.Logger.Fatal("PUBSUB_PROJECT_ID is not set")
	}

	client, err := pubsub.NewClient(ctx, Cfg.PubSubProjectID)
	if err != nil {
		utils.Logger.Fatal("failed to create PubSub client", zap.Error(err))
	}

	PubSubClient = client
	utils.Logger.Info("connected to PubSub")
}
