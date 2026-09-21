package operations

import (
	"context"
	"fmt"
	"os"
	"regexp"

	"github.com/els3aty/goha-webpanel/agent/internal/executor"
	"github.com/els3aty/goha-webpanel/agent/internal/protocol"
)

type RunBackupParams struct {
	Username   string `json:"username"`
	BackupType string `json:"backup_type"` // files, database
	FilePath   string `json:"file_path"`
	TargetDB   string `json:"target_db,omitempty"`
}

type RunRestoreParams struct {
	Username   string `json:"username"`
	BackupType string `json:"backup_type"` // files, database
	FilePath   string `json:"file_path"`
	TargetDB   string `json:"target_db,omitempty"`
}

var validSQLNameBackup = regexp.MustCompile(`^[a-zA-Z0-9_]{1,64}$`)

// HandleRunBackup performs a backup safely.
func HandleRunBackup(ctx context.Context, payload []byte) (interface{}, error) {
	var params RunBackupParams
	if err := protocol.ParsePayload[RunBackupParams](&protocol.Task{Operation: string(OpRunBackup), Payload: payload}); err != nil {
		return nil, err
	}

	if err := executor.ValidateUsername(params.Username); err != nil {
		return nil, err
	}
	if err := executor.ValidateAbsolutePath(params.FilePath); err != nil {
		return nil, err
	}

	// 1. Ensure backup directory exists with restricted permissions
	backupDir := fmt.Sprintf("/home/%s/backups", params.Username)
	_, err := executor.Run(ctx, "/usr/bin/mkdir", "-p", backupDir)
	if err != nil {
		executor.Run(ctx, "/bin/mkdir", "-p", backupDir) //nolint:errcheck
	}
	// Restrict to owner
	executor.Run(ctx, "/usr/bin/chmod", "700", backupDir) //nolint:errcheck
	executor.Run(ctx, "/usr/bin/chown", fmt.Sprintf("%s:%s", params.Username, params.Username), backupDir) //nolint:errcheck

	if params.BackupType == "files" {
		// Run tar -czf /path/to/backup.tar.gz -C /home/user/public_html .
		targetDir := fmt.Sprintf("/home/%s/public_html", params.Username)
		_, err := executor.Run(ctx, "/usr/bin/tar", "-czf", params.FilePath, "-C", targetDir, ".")
		if err != nil {
			_, err = executor.Run(ctx, "/bin/tar", "-czf", params.FilePath, "-C", targetDir, ".")
			if err != nil {
				return nil, fmt.Errorf("tar failed: %w", err)
			}
		}
	} else if params.BackupType == "database" {
		if !validSQLNameBackup.MatchString(params.TargetDB) {
			return nil, fmt.Errorf("invalid database name")
		}
		// Run mysqldump and pipe output to file directly in go, zero shell.
		_, err := executor.RunWithOutputToFile(ctx, params.FilePath, "/usr/bin/mysqldump", params.TargetDB)
		if err != nil {
			_, err = executor.RunWithOutputToFile(ctx, params.FilePath, "/bin/mysqldump", params.TargetDB)
			if err != nil {
				return nil, fmt.Errorf("mysqldump failed: %w", err)
			}
		}
	} else {
		return nil, fmt.Errorf("unknown backup type: %s", params.BackupType)
	}

	// Secure the resulting file
	executor.Run(ctx, "/usr/bin/chmod", "600", params.FilePath) //nolint:errcheck
	executor.Run(ctx, "/usr/bin/chown", fmt.Sprintf("%s:%s", params.Username, params.Username), params.FilePath) //nolint:errcheck

	// Get file size
	info, err := os.Stat(params.FilePath)
	sizeBytes := int64(0)
	if err == nil {
		sizeBytes = info.Size()
	}

	return map[string]interface{}{"status": "completed", "size_bytes": float64(sizeBytes)}, nil
}

// HandleRunRestore performs a restoration safely.
func HandleRunRestore(ctx context.Context, payload []byte) (interface{}, error) {
	var params RunRestoreParams
	if err := protocol.ParsePayload[RunRestoreParams](&protocol.Task{Operation: string(OpRunRestore), Payload: payload}); err != nil {
		return nil, err
	}

	if err := executor.ValidateUsername(params.Username); err != nil {
		return nil, err
	}
	if err := executor.ValidateAbsolutePath(params.FilePath); err != nil {
		return nil, err
	}

	if _, err := os.Stat(params.FilePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("backup file does not exist")
	}

	if params.BackupType == "files" {
		// Run tar -xzf /path/to/backup.tar.gz -C /home/user/public_html
		targetDir := fmt.Sprintf("/home/%s/public_html", params.Username)
		_, err := executor.Run(ctx, "/usr/bin/tar", "-xzf", params.FilePath, "-C", targetDir)
		if err != nil {
			_, err = executor.Run(ctx, "/bin/tar", "-xzf", params.FilePath, "-C", targetDir)
			if err != nil {
				return nil, fmt.Errorf("tar restore failed: %w", err)
			}
		}
		
		// Ensure restored files are owned by the user
		executor.Run(ctx, "/usr/bin/chown", "-R", fmt.Sprintf("%s:%s", params.Username, params.Username), targetDir) //nolint:errcheck
		
	} else if params.BackupType == "database" {
		if !validSQLNameBackup.MatchString(params.TargetDB) {
			return nil, fmt.Errorf("invalid database name")
		}
		
		// Read sql file and pipe it to mysql
		sqlData, err := os.ReadFile(params.FilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read backup file: %w", err)
		}
		
		_, err = executor.RunWithInput(ctx, sqlData, "/usr/bin/mysql", params.TargetDB)
		if err != nil {
			_, err = executor.RunWithInput(ctx, sqlData, "/bin/mysql", params.TargetDB)
			if err != nil {
				return nil, fmt.Errorf("mysql restore failed: %w", err)
			}
		}
	} else {
		return nil, fmt.Errorf("unknown backup type: %s", params.BackupType)
	}

	return map[string]interface{}{"status": "restored"}, nil
}
