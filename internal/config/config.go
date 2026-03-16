package config

import (
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv                     string
	Port                       string
	DatabaseURL                string
	JWTSecret                  string
	JWTIssuer                  string
	StorageRoot                string
	UploadMaxBytes             int64
	AutoCheckoutEnabled        bool
	AutoCheckoutGraceMinutes   int
	AutoCheckoutPollSeconds    int
	AutoCheckoutSystemPhotoURL string
	AutoCheckoutSystemNote     string
	ReadTimeout                time.Duration
	WriteTimeout               time.Duration
	ShutdownTimeout            time.Duration
}

func Load() (Config, error) {
	_ = godotenv.Load(".env.local")
	_ = godotenv.Load(".env")

	cfg := Config{
		AppEnv:                     getEnv("APP_ENV", "development"),
		Port:                       getEnv("PORT", "8080"),
		DatabaseURL:                os.Getenv("DATABASE_URL"),
		JWTSecret:                  os.Getenv("JWT_SECRET"),
		JWTIssuer:                  getEnv("JWT_ISSUER", "satpam-go"),
		StorageRoot:                getEnv("STORAGE_ROOT", "storage"),
		UploadMaxBytes:             getInt64Env("UPLOAD_MAX_BYTES", 3*1024*1024),
		AutoCheckoutEnabled:        getBoolEnv("AUTO_CHECKOUT_ENABLED", false),
		AutoCheckoutGraceMinutes:   getIntEnv("AUTO_CHECKOUT_GRACE_MINUTES", 5),
		AutoCheckoutPollSeconds:    getIntEnv("AUTO_CHECKOUT_POLL_SECONDS", 60),
		AutoCheckoutSystemPhotoURL: getEnv("AUTO_CHECKOUT_SYSTEM_PHOTO_URL", "/uploads/system/attendance/check-out-by-system.svg"),
		AutoCheckoutSystemNote:     getEnv("AUTO_CHECKOUT_SYSTEM_NOTE", "Check out by system"),
		ReadTimeout:                getDurationFromSeconds("READ_TIMEOUT_SECONDS", 15),
		WriteTimeout:               getDurationFromSeconds("WRITE_TIMEOUT_SECONDS", 15),
		ShutdownTimeout:            getDurationFromSeconds("SHUTDOWN_TIMEOUT_SECONDS", 10),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return Config{}, errors.New("JWT_SECRET is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getDurationFromSeconds(key string, fallback int) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return time.Duration(fallback) * time.Second
	}

	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 1 {
		return time.Duration(fallback) * time.Second
	}

	return time.Duration(seconds) * time.Second
}

func getInt64Env(key string, fallback int64) int64 {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 {
		return fallback
	}

	return value
}

func getIntEnv(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return fallback
	}

	return value
}

func getBoolEnv(key string, fallback bool) bool {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}

	return value
}
