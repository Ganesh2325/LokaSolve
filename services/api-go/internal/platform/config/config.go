package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Env                  string
	HTTPAddr             string
	AutoMigrate          bool
	DatabaseURL          string
	MigrateDatabaseURL   string
	RedisURL             string
	AuthTokenSecret      string
	OTPPepper            string
	OTPDevExpose         bool
	AccessTokenTTL       time.Duration
	RefreshTokenTTL      time.Duration
	LockoutThreshold     int
	LockoutDuration      time.Duration
	CookieSecure         bool
	CORSOrigins          []string
	IdentityBaseURL      string
	IdentityServiceToken string
	IdentityWritesClosed bool
	IssueBaseURL         string
	IssueServiceToken    string
	IssueWritesClosed    bool
}

func Load() (Config, error) {
	loadEnvFiles()
	cfg := Config{
		Env:                  getenv("ENV", "development"),
		HTTPAddr:             getenv("HTTP_ADDR", ":8080"),
		AutoMigrate:          getenv("AUTO_MIGRATE", "true") == "true",
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		MigrateDatabaseURL:   os.Getenv("MIGRATE_DATABASE_URL"),
		RedisURL:             os.Getenv("REDIS_URL"),
		AuthTokenSecret:      os.Getenv("AUTH_TOKEN_SECRET"),
		OTPPepper:            os.Getenv("OTP_PEPPER"),
		OTPDevExpose:         os.Getenv("OTP_DEV_EXPOSE") == "true",
		LockoutThreshold:     5,
		LockoutDuration:      15 * time.Minute,
		CookieSecure:         os.Getenv("COOKIE_SECURE") == "true",
		CORSOrigins:          splitCSV(getenv("CORS_ORIGINS", "http://localhost:3000")),
		IdentityBaseURL:      strings.TrimRight(os.Getenv("IDENTITY_BASE_URL"), "/"),
		IdentityServiceToken: os.Getenv("IDENTITY_SERVICE_TOKEN"),
		IdentityWritesClosed: os.Getenv("IDENTITY_WRITES") == "closed" || os.Getenv("IDENTITY_BASE_URL") != "",
		IssueBaseURL:         strings.TrimRight(os.Getenv("ISSUE_BASE_URL"), "/"),
		IssueServiceToken:    os.Getenv("ISSUE_SERVICE_TOKEN"),
		IssueWritesClosed:    os.Getenv("ISSUE_WRITES") == "closed" || os.Getenv("ISSUE_BASE_URL") != "",
	}
	var err error
	if cfg.AccessTokenTTL, err = time.ParseDuration(getenv("ACCESS_TOKEN_TTL", "15m")); err != nil {
		return Config{}, fmt.Errorf("ACCESS_TOKEN_TTL: %w", err)
	}
	if cfg.RefreshTokenTTL, err = time.ParseDuration(getenv("REFRESH_TOKEN_TTL", "720h")); err != nil {
		return Config{}, fmt.Errorf("REFRESH_TOKEN_TTL: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if len(c.AuthTokenSecret) < 32 {
		return fmt.Errorf("AUTH_TOKEN_SECRET must be at least 32 characters")
	}
	if len(c.OTPPepper) < 32 {
		return fmt.Errorf("OTP_PEPPER must be at least 32 characters")
	}
	if c.Env == "production" {
		if c.OTPDevExpose {
			return fmt.Errorf("OTP_DEV_EXPOSE cannot be enabled in production")
		}
		if strings.Contains(c.AuthTokenSecret, "change-me") || strings.Contains(c.OTPPepper, "change-me") {
			return fmt.Errorf("production secrets still contain a placeholder")
		}
		if !c.CookieSecure {
			return fmt.Errorf("COOKIE_SECURE must be true in production")
		}
	}
	if c.IdentityBaseURL != "" && len(c.IdentityServiceToken) < 32 {
		return fmt.Errorf("IDENTITY_SERVICE_TOKEN must be at least 32 characters")
	}
	if c.IssueBaseURL != "" && len(c.IssueServiceToken) < 32 {
		return fmt.Errorf("ISSUE_SERVICE_TOKEN must be at least 32 characters")
	}
	return nil
}

func (c Config) Dev() bool {
	return c.Env != "production"
}

func loadEnvFiles() {
	for _, path := range []string{os.Getenv("ENV_FILE"), ".env", filepathDotEnv()} {
		if path == "" {
			continue
		}
		_ = loadEnvFile(path)
	}
}

func filepathDotEnv() string {
	if _, err := os.Stat(".env"); err == nil {
		return ""
	}
	if _, err := os.Stat("../../.env"); err == nil {
		return "../../.env"
	}
	return ""
}

func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
	return sc.Err()
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
