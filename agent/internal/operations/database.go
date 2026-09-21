package operations

import (
	"encoding/json"
	"context"
	"fmt"
	"regexp"

	"github.com/els3aty/goha-webpanel/agent/internal/executor"
)

// SQL injection prevention: strictly limit DB names and usernames to alphanumeric + underscores
var validSQLName = regexp.MustCompile(`^[a-zA-Z0-9_]{1,64}$`)

type CreateDatabaseParams struct {
	DBName string `json:"db_name"`
}

type CreateDatabaseUserParams struct {
	DBUsername string `json:"db_username"`
	Password   string `json:"password"`
}

type GrantDatabasePrivilegesParams struct {
	DBName     string `json:"db_name"`
	DBUsername string `json:"db_username"`
	Privileges string `json:"privileges"` // Defaults to "ALL PRIVILEGES"
}

// HandleCreateDatabase executes a CREATE DATABASE statement securely.
func HandleCreateDatabase(ctx context.Context, payload []byte) (interface{}, error) {
	var params CreateDatabaseParams
	if err := json.Unmarshal(payload, &params); err != nil {
		return nil, err
	}

	if !validSQLName.MatchString(params.DBName) {
		return nil, fmt.Errorf("invalid database name")
	}

	// We pipe the SQL directly into mysql via stdin.
	// This avoids passing any parameters on the command line.
	sqlQuery := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`;", params.DBName)
	
	_, err := executor.RunWithInput(ctx, []byte(sqlQuery), "/usr/bin/mysql")
	if err != nil {
		_, err = executor.RunWithInput(ctx, []byte(sqlQuery), "/bin/mysql")
		if err != nil {
			return nil, fmt.Errorf("failed to create database: %w", err)
		}
	}

	return map[string]string{"status": "created", "db_name": params.DBName}, nil
}

// HandleCreateDatabaseUser executes a CREATE USER statement securely.
func HandleCreateDatabaseUser(ctx context.Context, payload []byte) (interface{}, error) {
	var params CreateDatabaseUserParams
	if err := json.Unmarshal(payload, &params); err != nil {
		return nil, err
	}

	if !validSQLName.MatchString(params.DBUsername) {
		return nil, fmt.Errorf("invalid database username")
	}

	// Minimal escaping for password just to be safe, although RunWithInput prevents shell injection.
	// MariaDB allows passwords up to 41 chars (hashed) or plaintext. We should just pass it in string literals.
	// We will avoid backticks or single quotes in password by returning error if present, 
	// because we are injecting into single quotes.
	for _, c := range params.Password {
		if c == '\'' || c == '\\' {
			return nil, fmt.Errorf("password contains illegal characters (' or \\)")
		}
	}

	// Create user if not exists
	sqlQuery := fmt.Sprintf("CREATE USER IF NOT EXISTS `%s`@`localhost` IDENTIFIED BY '%s';", params.DBUsername, params.Password)
	
	// Execute via stdin
	_, err := executor.RunWithInput(ctx, []byte(sqlQuery), "/usr/bin/mysql")
	if err != nil {
		_, err = executor.RunWithInput(ctx, []byte(sqlQuery), "/bin/mysql")
		if err != nil {
			return nil, fmt.Errorf("failed to create database user: %w", err)
		}
	}

	return map[string]string{"status": "created", "db_username": params.DBUsername}, nil
}

// HandleGrantDatabasePrivileges grants access for a user to a DB.
func HandleGrantDatabasePrivileges(ctx context.Context, payload []byte) (interface{}, error) {
	var params GrantDatabasePrivilegesParams
	if err := json.Unmarshal(payload, &params); err != nil {
		return nil, err
	}

	if !validSQLName.MatchString(params.DBName) || !validSQLName.MatchString(params.DBUsername) {
		return nil, fmt.Errorf("invalid db name or username")
	}

	// Hardcode privileges since we only support ALL PRIVILEGES for now.
	// If dynamic privileges are needed later, they MUST be strictly allowlisted.
	privileges := "ALL PRIVILEGES"

	sqlQuery := fmt.Sprintf("GRANT %s ON `%s`.* TO `%s`@`localhost`; FLUSH PRIVILEGES;", privileges, params.DBName, params.DBUsername)
	
	_, err := executor.RunWithInput(ctx, []byte(sqlQuery), "/usr/bin/mysql")
	if err != nil {
		_, err = executor.RunWithInput(ctx, []byte(sqlQuery), "/bin/mysql")
		if err != nil {
			return nil, fmt.Errorf("failed to grant privileges: %w", err)
		}
	}

	return map[string]string{"status": "granted"}, nil
}
