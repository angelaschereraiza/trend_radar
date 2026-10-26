package config

import (
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/angelaschereraiza/trend_radar/backend/internal/domain"
)

type Config struct {
	Environment         string
	APIHost             string
	APIPort             int
	DBHost              string
	DBPort              int
	DBName              string
	DBUser              string
	DBPassword          string
	DBSSLMode           string
	BaseCurrency        string
	MaxCapitalCHF       int
	AlertScoreThreshold float64
	LogLevel            string
}

func Load() (Config, error) {
	cfg := Config{
		Environment:         envOr("APP_ENV", "development"),
		APIHost:             envOr("API_HOST", "127.0.0.1"),
		DBHost:              envOr("DB_HOST", "localhost"),
		DBName:              envOr("DB_NAME", "trend_radar"),
		DBUser:              envOr("DB_USER", "trend_radar"),
		DBPassword:          os.Getenv("DB_PASSWORD"),
		DBSSLMode:           envOr("DB_SSLMODE", "disable"),
		BaseCurrency:        envOr("BASE_CURRENCY", "CHF"),
		AlertScoreThreshold: 80,
		LogLevel:            strings.ToLower(envOr("LOG_LEVEL", "info")),
	}

	var err error
	if cfg.APIPort, err = intEnv("API_PORT", 8080); err != nil {
		return Config{}, err
	}
	if cfg.DBPort, err = intEnv("DB_PORT", 5432); err != nil {
		return Config{}, err
	}
	if cfg.MaxCapitalCHF, err = intEnv("MAX_CAPITAL_CHF", domain.MaxCapitalCHF); err != nil {
		return Config{}, err
	}
	if threshold, ok := os.LookupEnv("ALERT_SCORE_THRESHOLD"); ok {
		cfg.AlertScoreThreshold, err = strconv.ParseFloat(threshold, 64)
		if err != nil {
			return Config{}, fmt.Errorf("ALERT_SCORE_THRESHOLD must be a number: %w", err)
		}
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	apiIP := net.ParseIP(c.APIHost)
	if c.APIHost != "localhost" && (apiIP == nil || !apiIP.IsLoopback()) {
		return fmt.Errorf("API_HOST must be localhost or a loopback IP address (got %q)", c.APIHost)
	}
	if c.APIPort < 1 || c.APIPort > 65535 {
		return fmt.Errorf("API_PORT must be between 1 and 65535 (got %d)", c.APIPort)
	}
	if c.DBHost == "" || c.DBName == "" || c.DBUser == "" {
		return fmt.Errorf("DB_HOST, DB_NAME, and DB_USER must not be empty")
	}
	switch c.DBSSLMode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
	default:
		return fmt.Errorf("DB_SSLMODE is invalid (got %q)", c.DBSSLMode)
	}
	if c.DBPort < 1 || c.DBPort > 65535 {
		return fmt.Errorf("DB_PORT must be between 1 and 65535 (got %d)", c.DBPort)
	}
	if c.BaseCurrency != "CHF" {
		return fmt.Errorf("BASE_CURRENCY must be CHF (got %q)", c.BaseCurrency)
	}
	if c.MaxCapitalCHF < 1 || c.MaxCapitalCHF > domain.MaxCapitalCHF {
		return fmt.Errorf("MAX_CAPITAL_CHF must be between 1 and %d (got %d)", domain.MaxCapitalCHF, c.MaxCapitalCHF)
	}
	if math.IsNaN(c.AlertScoreThreshold) || math.IsInf(c.AlertScoreThreshold, 0) ||
		c.AlertScoreThreshold < 0 || c.AlertScoreThreshold > 100 {
		return fmt.Errorf("ALERT_SCORE_THRESHOLD must be between 0 and 100 (got %v)", c.AlertScoreThreshold)
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error (got %q)", c.LogLevel)
	}
	return nil
}

func (c Config) DatabaseURL() string {
	query := url.Values{}
	query.Set("sslmode", c.DBSSLMode)
	query.Set("timezone", "UTC")
	return (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.DBUser, c.DBPassword),
		Host:     net.JoinHostPort(c.DBHost, strconv.Itoa(c.DBPort)),
		Path:     "/" + c.DBName,
		RawQuery: query.Encode(),
	}).String()
}

func envOr(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func intEnv(key string, fallback int) (int, error) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return parsed, nil
}
