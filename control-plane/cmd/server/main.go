// Package main — Control Plane entry point.
// Loads config, connects to DB, runs migrations, starts HTTP server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/els3aty/goha-webpanel/control-plane/internal/agentclient"
	"github.com/els3aty/goha-webpanel/control-plane/internal/api"
	"github.com/els3aty/goha-webpanel/control-plane/internal/audit"
	"github.com/els3aty/goha-webpanel/control-plane/internal/auth"
	"github.com/els3aty/goha-webpanel/control-plane/internal/config"
	"github.com/els3aty/goha-webpanel/control-plane/internal/db"
)

func main() {
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println("GohaHost Control Plane v1.0.0")
		os.Exit(0)
	}

	// Structured JSON logger — machine parseable, no secrets logged
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	if err := run(logger); err != nil {
		logger.Error("server failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── Load and validate configuration ──────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		// Config errors may reveal structure but not secrets
		return fmt.Errorf("configuration error: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}
	logger.Info("configuration loaded",
		slog.String("env", cfg.App.Env),
		slog.Int("port", cfg.App.Port),
		// Deliberately NOT logging: DB DSN, secrets, keys
	)

	// ── Connect to database ───────────────────────────────────────────────────
	pool, err := db.Connect(ctx, &cfg.DB)
	if err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}
	defer pool.Close()
	logger.Info("database connected")

	// ── Run migrations ────────────────────────────────────────────────────────
	migrator := db.NewMigrator(pool)
	if err := migrator.Run(ctx, db.AllMigrations()); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}
	logger.Info("migrations complete")

	// ── Initialize services ───────────────────────────────────────────────────
	sessions := auth.NewSessionManager(pool, &cfg.Session)
	auditLog := audit.New(pool, logger)

	// TOTP requires an encryptor — Phase 2 uses a simple AES-GCM encryptor
	// Phase 16 will replace this with Vault/secret store integration
	encryptor := newAESEncryptor(cfg.App.SecretKey)
	totpMgr := auth.NewTOTPManager(pool, encryptor)

	// ── Initialize Agent Client ───────────────────────────────────────────────
	// Secure mTLS client configured via cfg.Agent.ClientCert/Key
	agentClient, err := agentclient.NewClient(
		cfg.Agent.ClientCert,
		cfg.Agent.ClientKey,
		cfg.Agent.CACert,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize agent client: %w", err)
	}

	// ── Build router ──────────────────────────────────────────────────────────
	router := api.NewRouter(&api.Dependencies{
		Pool:        pool,
		Sessions:    sessions,
		TOTP:        totpMgr,
		AuditLog:    auditLog,
		Config:      cfg,
		Logger:      logger,
		AgentClient: agentClient,
		Encryptor:   encryptor,
	})

	// ── Start HTTP server ─────────────────────────────────────────────────────
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.App.Port),
		Handler:           router,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
	}

	// Start server in background
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server starting",
			slog.Int("port", cfg.App.Port),
			slog.String("env", cfg.App.Env),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	logger.Info("server stopped gracefully")
	return nil
}

// ── AES-GCM encryptor (Phase 2 — replace with Vault in Phase 16) ─────────────

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"
)

type aesEncryptor struct {
	key []byte
}

func newAESEncryptor(key []byte) auth.Encryptor {
	// Ensure key is exactly 32 bytes for AES-256
	k := make([]byte, 32)
	copy(k, key)
	return &aesEncryptor{key: k}
}

func (e *aesEncryptor) Encrypt(plaintext []byte) (string, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (e *aesEncryptor) Decrypt(ciphertext string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, data := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	return gcm.Open(nil, nonce, data, nil)
}
