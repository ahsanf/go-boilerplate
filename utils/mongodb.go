package utils

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var MongoClient *mongo.Client

func ConnectDB() {
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		log.Fatal("MONGO_URI is not set")
	}

	clientOpts := options.Client().ApplyURI(uri)

	if certPath := os.Getenv("MONGO_CREDENTIALS"); certPath != "" {
		tlsCfg, err := loadTLSConfig(certPath)
		if err != nil {
			log.Fatalf("failed to load TLS config: %v", err)
		}
		clientOpts.SetTLSConfig(tlsCfg)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(clientOpts)
	if err != nil {
		log.Fatalf("failed to connect to MongoDB: %v", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("failed to ping MongoDB: %v", err)
	}

	MongoClient = client
	Logger.Info("connected to MongoDB")
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
