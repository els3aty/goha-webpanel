// Package main — Agent entry point.
// Runs the mTLS server, verifies tasks, and executes operations.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/els3aty/goha-webpanel/agent/internal/config"
	"github.com/els3aty/goha-webpanel/agent/internal/mtls"
	"github.com/els3aty/goha-webpanel/agent/internal/nonce"
	"github.com/els3aty/goha-webpanel/agent/internal/operations"
	"github.com/els3aty/goha-webpanel/agent/internal/protocol"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	if err := run(logger); err != nil {
		logger.Error("agent failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 1. Load config
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger.Info("agent starting",
		slog.String("node_id", cfg.NodeID),
		slog.String("listen", cfg.ListenAddr))

	// 2. Setup mTLS configuration (requires mutual auth)
	tlsConfig, err := mtls.Config(cfg.TLSCertFile, cfg.TLSKeyFile, cfg.TLSCAFile)
	if err != nil {
		return fmt.Errorf("failed to configure mTLS: %w", err)
	}

	// 3. Setup protocol components
	// In production, the signing key is provisioned securely during enrollment.
	// For Phase 3, we read it from an env var.
	signingKey := []byte(os.Getenv("AGENT_SIGNING_KEY"))
	if len(signingKey) < 32 {
		return fmt.Errorf("AGENT_SIGNING_KEY must be at least 32 bytes")
	}

	nonceStore := nonce.New()
	verifier := protocol.NewVerifier(
		cfg.NodeID,
		signingKey,
		time.Duration(cfg.TaskMaxAgeSecs)*time.Second,
		nonceStore,
	)

	// 4. Register operations
	registry := operations.NewRegistry()
	registry.Register(operations.OpGetSystemMetrics, func(ctx context.Context, payload []byte) (interface{}, error) {
		return map[string]string{"status": "ok", "cpu": "10%"}, nil
	})
	registry.Register(operations.OpCreateHostingUser, operations.HandleCreateHostingUser)
	registry.Register(operations.OpCreateVirtualHost, operations.HandleCreateVirtualHost)
	registry.Register(operations.OpCreateDatabase, operations.HandleCreateDatabase)
	registry.Register(operations.OpCreateDatabaseUser, operations.HandleCreateDatabaseUser)
	registry.Register(operations.OpGrantDatabasePrivileges, operations.HandleGrantDatabasePrivileges)
	registry.Register(operations.OpEnableSSL, operations.HandleEnableSSL)
	registry.Register(operations.OpCreateDNSZone, operations.HandleCreateDNSZone)
	registry.Register(operations.OpDeleteDNSZone, operations.HandleDeleteDNSZone)
	registry.Register(operations.OpRunBackup, operations.HandleRunBackup)
	registry.Register(operations.OpRunRestore, operations.HandleRunRestore)
	registry.Register(operations.OpCreateNodeApp, operations.HandleCreateNodeApp)
	registry.Register(operations.OpCreateMailbox, operations.HandleCreateMailbox)
	registry.Register(operations.OpDeleteMailbox, operations.HandleDeleteMailbox)
	registry.Register(operations.OpCreateAlias, operations.HandleCreateAlias)
	registry.Register("FileList", operations.HandleFileList)
	registry.Register("FileRead", operations.HandleFileRead)
	registry.Register("FileWrite", operations.HandleFileWrite)
	registry.Register("FileDelete", operations.HandleFileDelete)
	registry.Register("GetMetrics", operations.HandleGetMetrics)
	registry.Register("InstallApp", operations.HandleInstallApp)
	registry.Register("ConfigureLoadBalancer", operations.HandleConfigureLoadBalancer)
	registry.Register("IssueSSL", operations.HandleIssueSSL)

	// 5. Setup HTTP handler
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/tasks", handleTask(logger, verifier, registry))

	// 6. Start server
	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		TLSConfig:         tlsConfig,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		// ListenAndServeTLS with empty strings because certs are already in tlsConfig
		if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	logger.Info("agent stopped gracefully")
	return nil
}

func handleTask(logger *slog.Logger, verifier *protocol.Verifier, registry *operations.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Read body (limit size to prevent DoS)
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB max
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		// 1. Parse JSON structurally
		task, err := protocol.ParseTask(body)
		if err != nil {
			logger.Error("failed to parse task", slog.String("error", err.Error()))
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		// 2. Perform all 9 security checks (Signature, Expiry, Replay, etc.)
		if err := verifier.Verify(task); err != nil {
			logger.Error("task verification failed",
				slog.String("task_id", task.Envelope.TaskID),
				slog.String("operation", task.Operation),
				slog.String("error", err.Error()))
			
			// Return generic 403 to avoid leaking reason to attacker
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		logger.Info("task verified",
			slog.String("task_id", task.Envelope.TaskID),
			slog.String("operation", task.Operation))

		// 3. Execute the operation (must be in allowlist)
		start := time.Now()
		op := operations.OperationName(task.Operation)
		result, err := registry.Execute(r.Context(), op, task.Payload)
		duration := time.Since(start)

		// 4. Build response
		resp := protocol.TaskResult{
			TaskID:     task.Envelope.TaskID,
			ExecutedAt: time.Now().UTC().Format(time.RFC3339),
			DurationMS: duration.Milliseconds(),
		}

		if err != nil {
			resp.Status = "failed"
			resp.Error = &protocol.TaskError{
				Code:    "EXECUTION_FAILED",
				Message: err.Error(),
			}
			logger.Error("task failed",
				slog.String("task_id", task.Envelope.TaskID),
				slog.String("error", err.Error()))
		} else {
			resp.Status = "succeeded"
			resp.Output = result
			logger.Info("task succeeded",
				slog.String("task_id", task.Envelope.TaskID),
				slog.Int64("duration_ms", duration.Milliseconds()))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}
}
