// Package config loads agent configuration from environment variables.
// The agent is intentionally minimal — few config knobs, secure defaults.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// AgentConfig holds all agent configuration.
type AgentConfig struct {
	// Node identity
	NodeID   string
	NodeName string

	// mTLS — all fields required
	TLSCertFile string // agent's certificate (signed by internal CA)
	TLSKeyFile  string // agent's private key (mode 0600, owned by agent user)
	TLSCAFile   string // internal CA certificate (for verifying Control Plane cert)

	// Listen address — MUST be private network IP, never 0.0.0.0 in production
	ListenAddr string // e.g. "10.0.0.5:9090"

	// Control Plane
	ControlPlaneAddr string // e.g. "10.0.0.1:8443" — used for callbacks

	// Task security
	TaskMaxAgeSecs    int // reject tasks older than this many seconds
	NonceStoreTTLSecs int // how long to remember seen nonces (replay protection)

	// Operations root
	// All file operations are bounded to this path — never allow escaping
	AccountsRoot string // e.g. "/home" — hosting accounts live here

	// Logging
	LogLevel string // "debug" | "info" | "warn" | "error"
}

// Load reads agent config from environment variables.
// Returns an error if any required field is missing.
// The agent MUST NOT start if Load returns an error.
func Load() (*AgentConfig, error) {
	c := &AgentConfig{}
	var errs []string

	c.NodeID   = requireEnv("AGENT_NODE_ID", &errs)
	c.NodeName  = requireEnv("AGENT_NODE_NAME", &errs)

	c.TLSCertFile = requireEnv("AGENT_TLS_CERT_FILE", &errs)
	c.TLSKeyFile  = requireEnv("AGENT_TLS_KEY_FILE", &errs)
	c.TLSCAFile   = requireEnv("AGENT_TLS_CA_FILE", &errs)

	c.ListenAddr = getEnv("AGENT_LISTEN_ADDR", "")
	if c.ListenAddr == "" {
		errs = append(errs, "AGENT_LISTEN_ADDR is required (e.g. 10.0.0.5:9090)")
	}
	// Security check: reject 0.0.0.0 binding
	if strings.HasPrefix(c.ListenAddr, "0.0.0.0") {
		errs = append(errs, "AGENT_LISTEN_ADDR must not bind to 0.0.0.0 — use a specific private IP")
	}

	c.ControlPlaneAddr = requireEnv("AGENT_CONTROL_PLANE_ADDR", &errs)
	c.AccountsRoot     = getEnv("AGENT_ACCOUNTS_ROOT", "/home")
	c.LogLevel         = getEnv("AGENT_LOG_LEVEL", "info")

	c.TaskMaxAgeSecs    = getEnvInt("AGENT_TASK_MAX_AGE_SECS", 300)    // 5 minutes
	c.NonceStoreTTLSecs = getEnvInt("AGENT_NONCE_TTL_SECS", 600)       // 2× task lifetime

	// Validate cert files exist
	for _, f := range []struct{ name, path string }{
		{"AGENT_TLS_CERT_FILE", c.TLSCertFile},
		{"AGENT_TLS_KEY_FILE", c.TLSKeyFile},
		{"AGENT_TLS_CA_FILE", c.TLSCAFile},
	} {
		if f.path != "" {
			if _, err := os.Stat(f.path); err != nil {
				errs = append(errs, fmt.Sprintf("%s: file not found: %s", f.name, f.path))
			}
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("agent configuration errors:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return c, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func requireEnv(key string, errs *[]string) string {
	v := os.Getenv(key)
	if v == "" {
		*errs = append(*errs, fmt.Sprintf("%s is required", key))
	}
	return v
}

func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
