// Package executor provides safe OS-level command execution.
//
// CRITICAL SECURITY RULES — NEVER VIOLATE:
//
//  1. NEVER use exec.Command("sh", "-c", userInput)
//     This allows arbitrary command injection if any input is unsanitized.
//
//  2. ALWAYS pass arguments as separate strings to exec.Command:
//     exec.Command("/usr/bin/nginx", "-t")   ← CORRECT
//     exec.Command("sh", "-c", "nginx -t")   ← FORBIDDEN
//
//  3. ALWAYS use absolute paths for executables:
//     exec.Command("/usr/sbin/nginx", ...)   ← CORRECT
//     exec.Command("nginx", ...)             ← WRONG (PATH lookup)
//
//  4. NEVER construct command strings from user input:
//     cmd := "nginx -t --domain=" + domain  ← FORBIDDEN
//
//  5. ALWAYS validate all input parameters BEFORE passing to exec:
//     Validate domain format, username, path, etc.
//
// These rules are enforced by code review and by the fact that this package
// is the ONLY place in the agent that calls exec.Command.
// Operations call this package — they do NOT call exec.Command directly.
package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode"
)

// Result holds the output of a command execution.
type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

// RunResult wraps Result with an error for chaining.
type RunResult struct {
	Result
	Err error
}

// DefaultTimeout is the maximum time a single command may run.
const DefaultTimeout = 60 * time.Second

// Run executes a command safely.
//
// IMPORTANT: executable must be an absolute path.
// args must be separate strings — never include shell metacharacters.
// Never construct executable or args from untrusted user input without validation.
//
// Example:
//
//	result, err := executor.Run(ctx, "/usr/sbin/nginx", "-t")
//	result, err := executor.Run(ctx, "/usr/bin/systemctl", "reload", "nginx")
func Run(ctx context.Context, executable string, args ...string) (*Result, error) {
	if err := validateExecutable(executable); err != nil {
		return nil, err
	}

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	// #nosec G204 -- executor is the only place that calls exec.Command.
	// All callers must pass validated, absolute-path executables and
	// pre-validated argument slices. Never pass user input directly.
	cmd := exec.CommandContext(ctx, executable, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	result := &Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, fmt.Errorf("command %s exited with code %d: %s",
				executable, result.ExitCode, sanitizeOutput(result.Stderr))
		}
		return result, fmt.Errorf("command %s failed: %w", executable, err)
	}

	return result, nil
}

// RunWithInput runs a command with the given stdin input.
// Used for operations like mysql import where data is piped in.
func RunWithInput(ctx context.Context, input []byte, executable string, args ...string) (*Result, error) {
	if err := validateExecutable(executable); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	// #nosec G204
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Stdin = bytes.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	result := &Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, fmt.Errorf("command failed with code %d", result.ExitCode)
		}
		return result, fmt.Errorf("command failed: %w", err)
	}

	return result, nil
}

// RunWithOutputToFile runs a command and redirects its stdout directly to the specified file path.
// This is used for streaming large outputs (e.g., mysqldump) safely without shell redirection.
func RunWithOutputToFile(ctx context.Context, outputFile string, executable string, args ...string) (*Result, error) {
	if err := validateExecutable(executable); err != nil {
		return nil, err
	}
	if err := ValidateAbsolutePath(outputFile); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	// #nosec G204
	cmd := exec.CommandContext(ctx, executable, args...)

	out, err := os.Create(outputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}
	defer out.Close()

	cmd.Stdout = out

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	start := time.Now()
	err = cmd.Run()
	duration := time.Since(start)

	result := &Result{
		Stderr:   stderr.String(),
		Duration: duration,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, fmt.Errorf("command failed with code %d", result.ExitCode)
		}
		return result, fmt.Errorf("command failed: %w", err)
	}

	return result, nil
}

// ── Input Validation ──────────────────────────────────────────────────────────

// ValidateUsername checks that a Linux username is safe.
// Prevents injection via username parameter in hosting operations.
func ValidateUsername(username string) error {
	if len(username) == 0 || len(username) > 32 {
		return fmt.Errorf("username must be 1-32 characters")
	}
	for _, c := range username {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' && c != '-' {
			return fmt.Errorf("username contains invalid character: %q", c)
		}
	}
	// Must not start with a digit or hyphen
	if unicode.IsDigit(rune(username[0])) || username[0] == '-' {
		return fmt.Errorf("username must start with a letter or underscore")
	}
	return nil
}

// ValidateDomain checks that a domain name is safe for use in config files.
// Prevents nginx config injection via domain name.
func ValidateDomain(domain string) error {
	if len(domain) == 0 || len(domain) > 253 {
		return fmt.Errorf("domain must be 1-253 characters")
	}
	// Only allow: letters, digits, dots, hyphens
	for _, c := range domain {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '.' && c != '-' {
			return fmt.Errorf("domain contains invalid character: %q", c)
		}
	}
	// No leading/trailing dots or hyphens
	if domain[0] == '.' || domain[0] == '-' || domain[len(domain)-1] == '.' {
		return fmt.Errorf("domain has invalid start or end character")
	}
	return nil
}

// ValidateAbsolutePath checks that a path is absolute and does not contain
// traversal sequences. Used before passing paths to operations.
func ValidateAbsolutePath(path string) error {
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must be absolute (start with /)")
	}
	if strings.Contains(path, "..") {
		return fmt.Errorf("path must not contain traversal sequences (..)")
	}
	if strings.Contains(path, "\x00") {
		return fmt.Errorf("path must not contain null bytes")
	}
	return nil
}

// validateExecutable ensures the executable path is absolute.
// Prevents PATH lookup attacks.
func validateExecutable(executable string) error {
	if !strings.HasPrefix(executable, "/") {
		return fmt.Errorf("BUG: executor.Run called with non-absolute executable path: %q — "+
			"always use absolute paths to prevent PATH lookup attacks", executable)
	}
	return nil
}

// sanitizeOutput truncates and cleans command output for safe logging.
// Prevents log injection from malicious command output.
func sanitizeOutput(output string) string {
	// Truncate long output
	if len(output) > 500 {
		output = output[:500] + "...[truncated]"
	}
	// Remove control characters except newline and tab
	result := make([]rune, 0, len(output))
	for _, r := range output {
		if r == '\n' || r == '\t' || (r >= 32 && r < 127) {
			result = append(result, r)
		}
	}
	return string(result)
}
