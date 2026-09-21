package operations

import (
	"encoding/json"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/els3aty/goha-webpanel/agent/internal/executor"
)

type CreateMailboxParams struct {
	Address      string `json:"address"`
	PasswordHash string `json:"password_hash"`
	QuotaMB      int    `json:"quota_mb"`
}

type DeleteMailboxParams struct {
	Address string `json:"address"`
}

type CreateAliasParams struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

// Mail file paths
const (
	dovecotUsersFile = "/etc/dovecot/users"
	postfixVmailbox  = "/etc/postfix/vmailbox"
	postfixVirtual   = "/etc/postfix/virtual"
)

func HandleCreateMailbox(ctx context.Context, payload []byte) (interface{}, error) {
	var params CreateMailboxParams
	if err := json.Unmarshal(payload, &params); err != nil {
		return nil, err
	}

	// 1. Basic validation
	if !strings.Contains(params.Address, "@") {
		return nil, fmt.Errorf("invalid email address")
	}

	// 2. Add to dovecot users: format -> address:password_hash::::::
	// The hash is pre-computed by Control Plane, avoiding doveadm execution entirely.
	dovecotEntry := fmt.Sprintf("%s:%s::::::\n", params.Address, params.PasswordHash)
	if err := appendToFile(dovecotUsersFile, dovecotEntry); err != nil {
		return nil, fmt.Errorf("failed to write dovecot users: %w", err)
	}

	// 3. Add to postfix vmailbox: format -> address domain/address/
	domain := strings.Split(params.Address, "@")[1]
	mailboxPath := fmt.Sprintf("%s/%s/", domain, strings.Split(params.Address, "@")[0])
	postfixEntry := fmt.Sprintf("%s %s\n", params.Address, mailboxPath)
	if err := appendToFile(postfixVmailbox, postfixEntry); err != nil {
		return nil, fmt.Errorf("failed to write postfix vmailbox: %w", err)
	}

	// 4. Run postmap to compile the hash database securely (no shell)
	_, err := executor.Run(ctx, "/usr/sbin/postmap", postfixVmailbox)
	if err != nil {
		return nil, fmt.Errorf("failed to run postmap on vmailbox: %w", err)
	}

	return map[string]string{"status": "mailbox_created", "address": params.Address}, nil
}

func HandleDeleteMailbox(ctx context.Context, payload []byte) (interface{}, error) {
	var params DeleteMailboxParams
	if err := json.Unmarshal(payload, &params); err != nil {
		return nil, err
	}
	if !strings.Contains(params.Address, "@") {
		return nil, fmt.Errorf("invalid email address")
	}

	// Remove from dovecot
	if err := removeLinePrefix(dovecotUsersFile, params.Address+":"); err != nil {
		return nil, fmt.Errorf("failed to remove from dovecot: %w", err)
	}

	// Remove from postfix
	if err := removeLinePrefix(postfixVmailbox, params.Address+" "); err != nil {
		return nil, fmt.Errorf("failed to remove from postfix: %w", err)
	}

	// Re-run postmap securely
	_, err := executor.Run(ctx, "/usr/sbin/postmap", postfixVmailbox)
	if err != nil {
		return nil, fmt.Errorf("failed to run postmap on vmailbox: %w", err)
	}

	return map[string]string{"status": "mailbox_deleted"}, nil
}

func HandleCreateAlias(ctx context.Context, payload []byte) (interface{}, error) {
	var params CreateAliasParams
	if err := json.Unmarshal(payload, &params); err != nil {
		return nil, err
	}
	if !strings.Contains(params.Source, "@") || !strings.Contains(params.Destination, "@") {
		return nil, fmt.Errorf("invalid email address in alias")
	}

	// Add to postfix virtual: format -> source destination
	postfixEntry := fmt.Sprintf("%s %s\n", params.Source, params.Destination)
	if err := appendToFile(postfixVirtual, postfixEntry); err != nil {
		return nil, fmt.Errorf("failed to write postfix virtual: %w", err)
	}

	// Run postmap securely
	_, err := executor.Run(ctx, "/usr/sbin/postmap", postfixVirtual)
	if err != nil {
		return nil, fmt.Errorf("failed to run postmap on virtual: %w", err)
	}

	return map[string]string{"status": "alias_created"}, nil
}

// Helper functions for secure file manipulation without sed/awk shell scripts

func appendToFile(filepath, text string) error {
	f, err := os.OpenFile(filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(text)
	return err
}

func removeLinePrefix(filepath, prefix string) error {
	content, err := os.ReadFile(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Nothing to delete
		}
		return err
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	for _, line := range lines {
		if !strings.HasPrefix(line, prefix) && line != "" {
			newLines = append(newLines, line)
		}
	}

	// Ensure ends with newline if not empty
	newContent := strings.Join(newLines, "\n")
	if newContent != "" && !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}

	return os.WriteFile(filepath, []byte(newContent), 0644)
}
