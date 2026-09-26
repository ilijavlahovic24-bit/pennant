package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	Env      string
	HTTPAddr string

	DatabaseURL string
	RedisURL    string

	JWTSecret  string
	AccessTTL  time.Duration
	RefreshTTL time.Duration

	CookieSecure   bool
	CookieDomain   string // empty for localhost
	ExpiryInterval time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{
		Env:         getEnv("APP_ENV", "development"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL: getEnv("DATABASE_URL", ""),
		RedisURL:    getEnv("REDIS_URL", ""),
		JWTSecret:   getEnv("JWT_SECRET", ""),
		AccessTTL:   getDuration("ACCESS_TTL", 15*time.Minute),
		RefreshTTL:  getDuration("REFRESH_TTL", 7*24*time.Hour),
	}
	cfg.CookieSecure = cfg.Env == "production"
	cfg.CookieDomain = getEnv("COOKIE_DOMAIN", "")
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.RedisURL == "" {
		return nil, fmt.Errorf("REDIS_URL is required")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}
	cfg.ExpiryInterval = getDuration("EXPIRY_INTERVAL", time.Minute)
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
