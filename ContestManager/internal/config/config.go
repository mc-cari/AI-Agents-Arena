package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Database DatabaseConfig
	Redis    RedisConfig
	Server   ServerConfig
	Contest  ContestConfig
	Logging  LoggingConfig
	Worker   WorkerConfig
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type ServerConfig struct {
	GRPCPort string
	HTTPPort string
}

type ContestConfig struct {
	MaxConcurrentContests int
	DurationSeconds       int
	MaxTokensPerContest   int
}

type LoggingConfig struct {
	Level string
}

type WorkerConfig struct {
	MaxWorkers       int
	HeartbeatIntervalSeconds int
	JobTimeoutSeconds       int
}

func LoadConfig() *Config {
	config, _ := Load()
	return config
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	config := &Config{
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "contestmanager"),
			Password: getEnv("DB_PASSWORD", ""),
			DBName:   getEnv("DB_NAME", "contestmanager"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvAsInt("REDIS_DB", 0),
		},
		Server: ServerConfig{
			GRPCPort: getEnv("GRPC_PORT", "50051"),
			HTTPPort: getEnv("HTTP_PORT", "8080"),
		},
		Contest: ContestConfig{
			MaxConcurrentContests: getEnvAsInt("MAX_CONCURRENT_CONTESTS", 3),
			DurationSeconds:       getEnvAsInt("CONTEST_DURATION_SECONDS", 300),
			MaxTokensPerContest:   getEnvAsInt("MAX_TOKENS_PER_CONTEST", 200000),
		},
		Logging: LoggingConfig{
			Level: getEnv("LOG_LEVEL", "info"),
		},
		Worker: WorkerConfig{
			MaxWorkers:               getEnvAsInt("MAX_WORKERS", 3),
			HeartbeatIntervalSeconds: getEnvAsInt("WORKER_HEARTBEAT_INTERVAL", 15),
			JobTimeoutSeconds:        getEnvAsInt("WORKER_JOB_TIMEOUT", 300),
		},
	}

	return config, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
