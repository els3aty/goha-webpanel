// Package config loads and validates application configuration from environment variables.
// It never reads from files directly to avoid accidental secret file commits.
// All sensitive values are loaded from environment variables injected by the process supervisor.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all application configuration.
// Fields are validated on load — the app refuses to start with invalid config.
type Config struct {
	App      AppConfig
	DB       DBConfig
	Redis    RedisConfig
	Session  SessionConfig
	Security SecurityConfig
	SMTP     SMTPConfig
}

// AppConfig holds general application settings.
type AppConfig struct {
	Env        string // "development" | "production"
	Port       int
	URL        string
	SecretKey  []byte // minimum 32 bytes
}

// DBConfig holds PostgreSQL connection settings.
type DBConfig struct {
	Host     string
	Port     int
	Name     string
	User     string
	Password string // loaded from env — never logged
	SSLMode  string
	MaxConns int
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	Host     string
	Port     int
	Password string // loaded from env — never logged
	DB       int
}

// SessionConfig holds session management settings.
type SessionConfig struct {
	MaxAge        time.Duration
	SecureCookie  bool
	CookieName    string
	CookieDomain  string
}

// SecurityConfig holds security-related settings.
type SecurityConfig struct {
	// Argon2id parameters — do not lower these in production
	Argon2Memory      uint32
	Argon2Iterations  uint32
	Argon2Parallelism uint8
	// Rate limiting
	LoginMaxAttempts    int
	LoginWindowSeconds  int
	// CSRF
	CSRFSecret []byte
}

// SMTPConfig holds mail settings for panel notifications only.
type SMTPConfig struct {
	Host     string
	Port     int
	User     string
	Password string // loaded from env — never logged
	From     string
	TLS      bool
}

// Load reads configuration from environment variables and validates it.
// Returns an error if any required field is missing or invalid.
// The application MUST NOT start if Load returns an error.
func Load() (*Config, error) {
	c := &Config{}
	var errs []string

	// ── App ──────────────────────────────────────────────────
	c.App.Env = getEnv("APP_ENV", "development")
	if c.App.Env != "development" && c.App.Env != "production" {
		errs = append(errs, "APP_ENV must be 'development' or 'production'")
	}

	c.App.Port = getEnvInt("APP_PORT", 8080)
	c.App.URL = requireEnv("APP_URL", &errs)

	secretKeyHex := requireEnv("APP_SECRET_KEY", &errs)
	if len(secretKeyHex) < 32 {
		errs = append(errs, "APP_SECRET_KEY must be at least 32 characters")
	}
	c.App.SecretKey = []byte(secretKeyHex)

	// ── Database ──────────────────────────────────────────────
	c.DB.Host = getEnv("DB_HOST", "127.0.0.1")
	c.DB.Port = getEnvInt("DB_PORT", 5432)
	c.DB.Name = requireEnv("DB_NAME", &errs)
	c.DB.User = requireEnv("DB_USER", &errs)
	c.DB.Password = requireEnv("DB_PASSWORD", &errs)
	c.DB.SSLMode = getEnv("DB_SSLMODE", "require")
	c.DB.MaxConns = getEnvInt("DB_MAX_CONNS", 20)

	// Production: require SSL unless connecting to localhost
	isLocalDB := c.DB.Host == "localhost" || c.DB.Host == "127.0.0.1"
	if c.App.Env == "production" && c.DB.SSLMode == "disable" && !isLocalDB {
		errs = append(errs, "DB_SSLMODE cannot be 'disable' in production for remote databases")
	}

	// ── Redis ─────────────────────────────────────────────────
	c.Redis.Host = getEnv("REDIS_HOST", "127.0.0.1")
	c.Redis.Port = getEnvInt("REDIS_PORT", 6379)
	c.Redis.Password = os.Getenv("REDIS_PASSWORD") // optional
	c.Redis.DB = getEnvInt("REDIS_DB", 0)

	// ── Session ───────────────────────────────────────────────
	maxAgeHours := getEnvInt("SESSION_MAX_AGE_HOURS", 24)
	c.Session.MaxAge = time.Duration(maxAgeHours) * time.Hour
	c.Session.SecureCookie = getEnvBool("SESSION_SECURE_COOKIE", c.App.Env == "production")
	c.Session.CookieName = getEnv("SESSION_COOKIE_NAME", "__Host-panel-session")
	c.Session.CookieDomain = os.Getenv("SESSION_COOKIE_DOMAIN")

	// ── Security ──────────────────────────────────────────────
	// Argon2id parameters — OWASP recommended minimums
	c.Security.Argon2Memory = uint32(getEnvInt("ARGON2_MEMORY_KB", 65536)) // 64 MB
	c.Security.Argon2Iterations = uint32(getEnvInt("ARGON2_ITERATIONS", 3))
	c.Security.Argon2Parallelism = uint8(getEnvInt("ARGON2_PARALLELISM", 4))

	if c.Security.Argon2Memory < 16384 {
		errs = append(errs, "ARGON2_MEMORY_KB must be at least 16384 (16 MB)")
	}

	c.Security.LoginMaxAttempts = getEnvInt("LOGIN_MAX_ATTEMPTS", 10)
	c.Security.LoginWindowSeconds = getEnvInt("LOGIN_WINDOW_SECONDS", 600)

	csrfSecret := requireEnv("SESSION_SECRET", &errs)
	if len(csrfSecret) < 32 {
		errs = append(errs, "SESSION_SECRET must be at least 32 characters")
	}
	c.Security.CSRFSecret = []byte(csrfSecret)

	// ── SMTP ──────────────────────────────────────────────────
	c.SMTP.Host = getEnv("SMTP_HOST", "localhost")
	c.SMTP.Port = getEnvInt("SMTP_PORT", 587)
	c.SMTP.User = os.Getenv("SMTP_USER")
	c.SMTP.Password = os.Getenv("SMTP_PASSWORD") // not required
	c.SMTP.From = getEnv("SMTP_FROM", "noreply@example.test")
	c.SMTP.TLS = getEnvBool("SMTP_TLS", true)

	if len(errs) > 0 {
		return nil, fmt.Errorf("configuration errors:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return c, nil
}

// DSN returns a PostgreSQL connection string.
// IMPORTANT: Never log the returned string — it contains the password.
func (c *DBConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s pool_max_conns=%d",
		c.Host, c.Port, c.Name, c.User, c.Password, c.SSLMode, c.MaxConns,
	)
}

// IsProduction returns true when running in production mode.
func (c *Config) IsProduction() bool {
	return c.App.Env == "production"
}

// ── helpers ──────────────────────────────────────────────────────────────────

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func requireEnv(key string, errs *[]string) string {
	v := os.Getenv(key)
	if v == "" {
		*errs = append(*errs, fmt.Sprintf("%s is required but not set", key))
	}
	return v
}

func getEnvInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

func getEnvBool(key string, defaultVal bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return defaultVal
	}
	return b
}

// Validate checks that config fields meet minimum security requirements.
func (c *Config) Validate() error {
	if c.IsProduction() {
		var errs []string
		if !c.Session.SecureCookie {
			errs = append(errs, "SESSION_SECURE_COOKIE must be true in production")
		}
		if c.DB.SSLMode == "disable" {
			errs = append(errs, "DB_SSLMODE cannot be disable in production")
		}
		if len(errs) > 0 {
			return errors.New(strings.Join(errs, "; "))
		}
	}
	return nil
}
