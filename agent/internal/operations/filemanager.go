package operations

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/els3aty/goha-webpanel/agent/internal/protocol"
)

// FileReq payload received from Control Plane
type FileReq struct {
	Username string `json:"username"`
	Path     string `json:"path"`
	Content  string `json:"content,omitempty"`
}

type FileInfo struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
	Mode  string `json:"mode"`
}

// SecureResolvePath is the core Path Sandbox.
// It resolves all symlinks and guarantees the requested path strictly resides within the user's home directory.
func SecureResolvePath(username, requestedPath string) (string, error) {
	// Base boundary
	baseDir := filepath.Clean("/var/www/" + username)

	// Combine base with requested path safely
	// Clean resolves any ../ or ./ lexically
	cleanReqPath := filepath.Clean(requestedPath)
	
	// Prevent root absolute paths bypass (e.g. if path is "/etc/passwd")
	// If it's absolute, we strip the leading slash to make it relative to baseDir.
	if filepath.IsAbs(cleanReqPath) {
		cleanReqPath = strings.TrimPrefix(cleanReqPath, "/")
	}

	targetPath := filepath.Join(baseDir, cleanReqPath)

	// Evaluate physical symlinks (if file exists)
	// If it doesn't exist (e.g., during Write), we evaluate the parent directory
	evalPath, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			// For non-existent files (like uploads), evaluate the parent dir
			parentDir := filepath.Dir(targetPath)
			evalParent, errParent := filepath.EvalSymlinks(parentDir)
			if errParent != nil {
				return "", fmt.Errorf("invalid path parent: %w", errParent)
			}
			evalPath = filepath.Join(evalParent, filepath.Base(targetPath))
		} else {
			return "", fmt.Errorf("invalid path: %w", err)
		}
	}

	// Strictly verify the final evaluated path is bounded by the baseDir
	if !strings.HasPrefix(evalPath, baseDir) {
		return "", fmt.Errorf("forbidden: path escapes user boundary")
	}

	return evalPath, nil
}

func HandleFileList(ctx context.Context, payload []byte) (interface{}, error) {
	var req FileReq
	if err := protocol.ParsePayload[FileReq](&protocol.Task{Operation: "FileList", Payload: payload}); err != nil {
		return nil, err
	}

	safePath, err := SecureResolvePath(req.Username, req.Path)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(safePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var list []FileInfo
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		list = append(list, FileInfo{
			Name:  e.Name(),
			IsDir: e.IsDir(),
			Size:  info.Size(),
			Mode:  info.Mode().String(),
		})
	}

	return map[string]interface{}{"files": list, "path": req.Path}, nil
}

func HandleFileRead(ctx context.Context, payload []byte) (interface{}, error) {
	var req FileReq
	if err := protocol.ParsePayload[FileReq](&protocol.Task{Operation: "FileRead", Payload: payload}); err != nil {
		return nil, err
	}

	safePath, err := SecureResolvePath(req.Username, req.Path)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(safePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return map[string]string{"content": string(content)}, nil
}

func HandleFileWrite(ctx context.Context, payload []byte) (interface{}, error) {
	var req FileReq
	if err := protocol.ParsePayload[FileReq](&protocol.Task{Operation: "FileWrite", Payload: payload}); err != nil {
		return nil, err
	}

	safePath, err := SecureResolvePath(req.Username, req.Path)
	if err != nil {
		return nil, err
	}

	// Fetch UID/GID for chown
	u, err := user.Lookup(req.Username)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup user %s: %w", req.Username, err)
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)

	// Write file natively
	if err := os.WriteFile(safePath, []byte(req.Content), 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	// Safely chown to the specific user (fixes permission issues so web apps can read it)
	if err := os.Chown(safePath, uid, gid); err != nil {
		return nil, fmt.Errorf("failed to chown file: %w", err)
	}

	return map[string]string{"status": "written", "path": req.Path}, nil
}

func HandleFileDelete(ctx context.Context, payload []byte) (interface{}, error) {
	var req FileReq
	if err := protocol.ParsePayload[FileReq](&protocol.Task{Operation: "FileDelete", Payload: payload}); err != nil {
		return nil, err
	}

	safePath, err := SecureResolvePath(req.Username, req.Path)
	if err != nil {
		return nil, err
	}

	// Delete native (Zero-Shell)
	if err := os.RemoveAll(safePath); err != nil {
		return nil, fmt.Errorf("failed to delete path: %w", err)
	}

	return map[string]string{"status": "deleted"}, nil
}
