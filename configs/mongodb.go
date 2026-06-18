package configs

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"
	"time"

	"go-boilerplate/internal/utils"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

var MongoClient *mongo.Client

func ConnectDB() {
	if Cfg.MongoURI == "" {
		utils.Logger.Fatal("MONGO_URI is not set")
	}

	clientOpts := options.Client().ApplyURI(Cfg.MongoURI)

	if Cfg.MongoCredentials != "" {
		tlsCfg, err := loadTLSConfig(Cfg.MongoCredentials)
		if err != nil {
			utils.Logger.Fatal("failed to load TLS config", zap.Error(err))
		}
		clientOpts.SetTLSConfig(tlsCfg)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(clientOpts)
	if err != nil {
		utils.Logger.Fatal("failed to connect to MongoDB", zap.Error(err))
	}

	if err := client.Ping(ctx, nil); err != nil {
		utils.Logger.Fatal("failed to ping MongoDB", zap.Error(err))
	}

	MongoClient = client
	utils.Logger.Info("connected to MongoDB")
}

func loadTLSConfig(certPath string) (*tls.Config, error) {
	certs, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(certs)
	return &tls.Config{RootCAs: pool}, nil
}
