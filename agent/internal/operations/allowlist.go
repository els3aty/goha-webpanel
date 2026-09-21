// Package operations defines the complete typed operation allowlist.
// The agent ONLY executes operations in this list.
// Any operation not in the allowlist is REJECTED with an error.
//
// CRITICAL SECURITY RULE:
//   - This file IS the security boundary for what the agent can do
//   - Adding an operation here means the Control Plane can trigger it
//   - Never add "ExecuteShell", "RunScript", or any generic command execution
//   - Every operation must have explicit, typed parameters — no raw strings passed to shell
package operations

import (
	"context"
	"errors"
	"fmt"
)

// OperationName is a typed string to prevent string comparison mistakes.
type OperationName string

// ─── Complete Operation Allowlist ────────────────────────────────────────────
// These are the ONLY operations the agent will execute.
// If an operation is not in this list, it is rejected.
const (
	// User Management
	OpCreateHostingUser OperationName = "CreateHostingUser"
	OpLockHostingUser   OperationName = "LockHostingUser"
	OpUnlockHostingUser OperationName = "UnlockHostingUser"
	OpDeleteHostingUser OperationName = "DeleteHostingUser"

	// Web Server
	OpCreateVirtualHost      OperationName = "CreateVirtualHost"
	OpUpdateVirtualHost      OperationName = "UpdateVirtualHost"
	OpDeleteVirtualHost      OperationName = "DeleteVirtualHost"
	OpValidateWebServerConfig OperationName = "ValidateWebServerConfig"
	OpEnableSSL              OperationName = "EnableSSL"
	OpDisableSSL             OperationName = "DisableSSL"
	OpConfigureReverseProxy  OperationName = "ConfigureReverseProxy"
	OpReloadWebServer        OperationName = "ReloadWebServer"
	OpRollbackWebServerConfig OperationName = "RollbackWebServerConfig"
	OpGetWebServerHealth     OperationName = "GetWebServerHealth"

	// PHP
	OpSetPHPVersion  OperationName = "SetPHPVersion"
	OpCreateFPMPool  OperationName = "CreateFPMPool"
	OpDeleteFPMPool  OperationName = "DeleteFPMPool"
	OpReloadPHPFPM   OperationName = "ReloadPHPFPM"

	// Node.js
	OpCreateNodeApp   OperationName = "CreateNodeApp"
	OpStartNodeApp    OperationName = "StartNodeApp"
	OpStopNodeApp     OperationName = "StopNodeApp"
	OpRestartNodeApp  OperationName = "RestartNodeApp"
	OpGetNodeAppStatus OperationName = "GetNodeAppStatus"
	OpDeleteNodeApp   OperationName = "DeleteNodeApp"

	// Database (MariaDB)
	OpCreateDatabase       OperationName = "CreateDatabase"
	OpDeleteDatabase       OperationName = "DeleteDatabase"
	OpCreateDatabaseUser   OperationName = "CreateDatabaseUser"
	OpRotateDatabasePassword OperationName = "RotateDatabasePassword"
	OpDeleteDatabaseUser   OperationName = "DeleteDatabaseUser"

	// DNS
	OpCreateDNSZone   OperationName = "CreateDNSZone"
	OpDeleteDNSZone   OperationName = "DeleteDNSZone"
	OpCreateDNSRecord OperationName = "CreateDNSRecord"
	OpUpdateDNSRecord OperationName = "UpdateDNSRecord"
	OpDeleteDNSRecord OperationName = "DeleteDNSRecord"

	// Mail
	OpCreateMailbox       OperationName = "CreateMailbox"
	OpDeleteMailbox       OperationName = "DeleteMailbox"
	OpSetMailboxQuota     OperationName = "SetMailboxQuota"
	OpResetMailboxPassword OperationName = "ResetMailboxPassword"
	OpCreateAlias         OperationName = "CreateAlias"
	OpDeleteAlias         OperationName = "DeleteAlias"

	// Backup
	OpRunBackup           OperationName = "RunBackup"
	OpRunRestore          OperationName = "RunRestore"
	OpVerifyBackupIntegrity OperationName = "VerifyBackupIntegrity"

	// Firewall
	OpApplyFirewallRuleSet OperationName = "ApplyFirewallRuleSet"

	// Monitoring
	OpGetSystemMetrics  OperationName = "GetSystemMetrics"
	OpGetServiceStatus  OperationName = "GetServiceStatus"

	// Certificate rotation (internal use)
	OpRotateAgentCertificate OperationName = "RotateAgentCertificate"
)

// allowlist is the definitive set of permitted operations.
// This is a closed set — operations not listed here are REJECTED.
var allowlist = map[OperationName]bool{
	OpCreateHostingUser:       true,
	OpLockHostingUser:         true,
	OpUnlockHostingUser:       true,
	OpDeleteHostingUser:       true,
	OpCreateVirtualHost:       true,
	OpUpdateVirtualHost:       true,
	OpDeleteVirtualHost:       true,
	OpValidateWebServerConfig:  true,
	OpEnableSSL:               true,
	OpDisableSSL:              true,
	OpConfigureReverseProxy:   true,
	OpReloadWebServer:         true,
	OpRollbackWebServerConfig:  true,
	OpGetWebServerHealth:      true,
	OpSetPHPVersion:           true,
	OpCreateFPMPool:           true,
	OpDeleteFPMPool:           true,
	OpReloadPHPFPM:            true,
	OpCreateNodeApp:           true,
	OpStartNodeApp:            true,
	OpStopNodeApp:             true,
	OpRestartNodeApp:          true,
	OpGetNodeAppStatus:        true,
	OpDeleteNodeApp:           true,
	OpCreateDatabase:          true,
	OpDeleteDatabase:          true,
	OpCreateDatabaseUser:      true,
	OpRotateDatabasePassword:  true,
	OpDeleteDatabaseUser:      true,
	OpCreateDNSZone:           true,
	OpDeleteDNSZone:           true,
	OpCreateDNSRecord:         true,
	OpUpdateDNSRecord:         true,
	OpDeleteDNSRecord:         true,
	OpCreateMailbox:           true,
	OpDeleteMailbox:           true,
	OpSetMailboxQuota:         true,
	OpResetMailboxPassword:    true,
	OpCreateAlias:             true,
	OpDeleteAlias:             true,
	OpRunBackup:               true,
	OpRunRestore:              true,
	OpVerifyBackupIntegrity:   true,
	OpApplyFirewallRuleSet:    true,
	OpGetSystemMetrics:        true,
	OpGetServiceStatus:        true,
	OpRotateAgentCertificate:  true,
}

// ErrForbiddenOperation is returned when a task requests an operation not in the allowlist.
var ErrForbiddenOperation = errors.New("operation not in allowlist")

// IsAllowed returns true if the operation is in the allowlist.
func IsAllowed(op OperationName) bool {
	return allowlist[op]
}

// CheckAllowed returns ErrForbiddenOperation if the operation is not allowed.
// Logs the attempt — forbidden operation requests are security events.
func CheckAllowed(op OperationName) error {
	if !allowlist[op] {
		// This is a security event — caller should log it
		return fmt.Errorf("%w: %q", ErrForbiddenOperation, op)
	}
	return nil
}

// Handler is a function that executes a typed operation.
// It receives the task context and raw payload bytes.
// It returns a result (any serializable type) or an error.
type Handler func(ctx context.Context, payload []byte) (result interface{}, err error)

// Registry maps operation names to their handler functions.
type Registry struct {
	handlers map[OperationName]Handler
}

// NewRegistry creates an empty operation registry.
func NewRegistry() *Registry {
	return &Registry{handlers: make(map[OperationName]Handler)}
}

// Register adds a handler for an operation.
// Panics if the operation is not in the allowlist — programming error.
func (r *Registry) Register(op OperationName, h Handler) {
	if !IsAllowed(op) {
		panic(fmt.Sprintf("BUG: attempt to register handler for non-allowlisted operation: %q", op))
	}
	r.handlers[op] = h
}

// Execute finds and runs the handler for an operation.
// Returns ErrForbiddenOperation if not in allowlist.
// Returns an error if no handler is registered (unimplemented).
func (r *Registry) Execute(ctx context.Context, op OperationName, payload []byte) (interface{}, error) {
	// Always check allowlist first — defense in depth
	if err := CheckAllowed(op); err != nil {
		return nil, err
	}

	handler, ok := r.handlers[op]
	if !ok {
		// Operation is in allowlist but not yet implemented
		return nil, fmt.Errorf("operation %q is not yet implemented on this node", op)
	}

	return handler(ctx, payload)
}
