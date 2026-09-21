package executor_test

import (
	"context"
	"strings"
	"testing"

	"github.com/els3aty/goha-webpanel/agent/internal/executor"
)

func TestValidateAbsolutePath(t *testing.T) {
	valid := []string{"/usr/bin/echo", "/bin/ls", "/usr/sbin/nginx"}
	for _, p := range valid {
		if err := executor.ValidateAbsolutePath(p); err != nil {
			t.Errorf("expected valid path %q, got err: %v", p, err)
		}
	}

	invalid := []string{
		"echo",                  // not absolute
		"bin/ls",                // not absolute
		"/usr/bin/../bin/echo",  // traversal
		"/usr/bin/echo\x00test", // null byte
	}
	for _, p := range invalid {
		if err := executor.ValidateAbsolutePath(p); err == nil {
			t.Errorf("SECURITY: expected invalid path %q to fail", p)
		}
	}
}

func TestValidateUsername(t *testing.T) {
	valid := []string{"user1", "my_user", "test-user", "a"}
	for _, u := range valid {
		if err := executor.ValidateUsername(u); err != nil {
			t.Errorf("expected valid username %q, got err: %v", u, err)
		}
	}

	invalid := []string{
		"user;rm -rf /",  // injection
		"user space",     // space
		"-user",          // starts with hyphen
		"1user",          // starts with digit
		strings.Repeat("a", 33), // too long
		"user$name",      // invalid char
	}
	for _, u := range invalid {
		if err := executor.ValidateUsername(u); err == nil {
			t.Errorf("SECURITY: expected invalid username %q to fail", u)
		}
	}
}

func TestValidateDomain(t *testing.T) {
	valid := []string{"example.com", "sub.example.com", "my-domain.net"}
	for _, d := range valid {
		if err := executor.ValidateDomain(d); err != nil {
			t.Errorf("expected valid domain %q, got err: %v", d, err)
		}
	}

	invalid := []string{
		"example.com;ls", // injection
		"example com",    // space
		".example.com",   // starts with dot
		"example.com.",   // ends with dot
		"-example.com",   // starts with hyphen
		strings.Repeat("a", 254), // too long
	}
	for _, d := range invalid {
		if err := executor.ValidateDomain(d); err == nil {
			t.Errorf("SECURITY: expected invalid domain %q to fail", d)
		}
	}
}

func TestRun_RequiresAbsolutePath(t *testing.T) {
	_, err := executor.Run(context.Background(), "echo", "hello")
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Errorf("SECURITY: expected Run to reject non-absolute path, got: %v", err)
	}
}
