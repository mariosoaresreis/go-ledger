package config

import (
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

const (
	ServiceName         = "ledger-command-service"
	DefaultPort         = "8080"
	APIBasePath         = "/api"
	GracefulStopTimeout = 10 * time.Second
	DBMigrationsDir     = "internal/migrations"
)

type Config struct {
	ServiceName string
	Host        string
	Port        string
	Environment string
	LogLevel    string

	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string
	DBConns    int

	KafkaBootstrapServers string
	KafkaTopic            string

	RedisHost string
	RedisPort string
}

func Load() *Config {
	filename := "./files/.env"
	if _, err := os.Stat(filename); err == nil {
		_ = godotenv.Load(filename)
	} else {
		_ = godotenv.Load(".env")
	}

	cfg := &Config{
		ServiceName: ServiceName,
		Host:        getEnv("HOST", ""),
		Port:        getEnv("PORT", DefaultPort),
		Environment: getEnv("ENVIRONMENT", "local"),
		LogLevel:    getEnv("LOG_LEVEL", "debug"),

		DBHost:     getEnv("LEDGER_DB_HOST", "localhost"),
		DBPort:     getEnv("LEDGER_DB_PORT", "5432"),
		DBUser:     getEnv("LEDGER_DB_USERNAME", "ledger"),
		DBPassword: getEnv("LEDGER_DB_PASSWORD", "ledger"),
		DBName:     getEnv("LEDGER_DB_NAME", "ledger"),
		DBSSLMode:  getEnv("LEDGER_DB_SSL_MODE", "disable"),
		DBConns:    10,

		KafkaBootstrapServers: getEnv("LEDGER_KAFKA_BOOTSTRAP_SERVERS", "localhost:9092"),
		KafkaTopic:            getEnv("LEDGER_KAFKA_TOPIC", "ledger.events"),

		RedisHost: getEnv("LEDGER_REDIS_HOST", "localhost"),
		RedisPort: getEnv("LEDGER_REDIS_PORT", "6379"),
	}

	lvl, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		lvl = logrus.DebugLevel
	}
	logrus.SetLevel(lvl)
	logrus.SetFormatter(&logrus.JSONFormatter{})

	return cfg
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
