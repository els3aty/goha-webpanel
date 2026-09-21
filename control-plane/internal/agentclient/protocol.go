// Package agentclient provides the Control Plane client for communicating with Agents.
// It handles HMAC-SHA256 signing, mTLS, and task serialization.
package agentclient

import (
	"encoding/json"
)

// Version must match the agent's expected version.
const Version = "1"

// Task is the complete signed task envelope sent to the Agent.
// Field names MUST match exactly what the Agent expects for signature validation.
type Task struct {
	Envelope  Envelope        `json:"envelope"`
	Operation string          `json:"operation"`
	Payload   json.RawMessage `json:"payload"`
	Signature string          `json:"signature"`
}

// Envelope contains task metadata used for authentication and replay protection.
type Envelope struct {
	Version        string `json:"version"`
	TaskID         string `json:"task_id"`
	RequestID      string `json:"request_id"`
	NodeID         string `json:"node_id"`
	IssuedAt       string `json:"issued_at"`
	ExpiresAt      string `json:"expires_at"`
	Nonce          string `json:"nonce"`
	IdempotencyKey string `json:"idempotency_key"`
}

// TaskResult is the response received from the Agent.
type TaskResult struct {
	TaskID     string      `json:"task_id"`
	Status     string      `json:"status"`
	Output     interface{} `json:"output,omitempty"`
	Error      *TaskError  `json:"error,omitempty"`
	ExecutedAt string      `json:"executed_at"`
	DurationMS int64       `json:"duration_ms"`
}

// TaskError is returned in TaskResult when execution failed.
type TaskError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}
