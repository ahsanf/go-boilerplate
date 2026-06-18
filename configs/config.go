package configs

import (
	"os"
	"strconv"
)

type AppConfig struct {
	Port             string
	MongoURI         string
	MongoCredentials string
	DBName           string
	SecretKey        string
	PubSubProjectID  string
	StaticToken      string
	SkipPermission   bool
	AuthAPIURL       string
	RedisURL         string
	RedisEnabled     bool
	ServiceAccount   string // path to Google service account JSON (GOOGLE_SERVICE_ACCOUNT)
	GoogleCredPath   string // GOOGLE_APPLICATION_CREDENTIALS
}

var Cfg AppConfig

func LoadConfig() {
	Cfg = AppConfig{
		Port:             envOrDefault("PORT", "3000"),
		MongoURI:         os.Getenv("MONGO_URI"),
		MongoCredentials: os.Getenv("MONGO_CREDENTIALS"),
		DBName:           os.Getenv("DB_NAME"),
		SecretKey:        os.Getenv("SECRET_KEY"),
		PubSubProjectID:  os.Getenv("PUBSUB_PROJECT_ID"),
		StaticToken:      os.Getenv("STATIC_TOKEN"),
		SkipPermission:   envBool("SKIP_PERMISSION", false),
		AuthAPIURL:       os.Getenv("AUTH_API_URL"),
		RedisURL:         os.Getenv("REDIS_URL"),
		RedisEnabled:     envBool("REDIS_ENABLE", false),
		ServiceAccount:   os.Getenv("GOOGLE_SERVICE_ACCOUNT"),
		GoogleCredPath:   os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
