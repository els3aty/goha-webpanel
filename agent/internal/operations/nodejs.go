package operations

import (
	"context"
	"fmt"
	"os/user"

	"github.com/els3aty/goha-webpanel/agent/internal/executor"
	"github.com/els3aty/goha-webpanel/agent/internal/protocol"
)

type CreateNodeAppParams struct {
	Username    string `json:"username"`
	AppName     string `json:"app_name"`
	Domain      string `json:"domain"`
	AppPath     string `json:"app_path"`
	StartupFile string `json:"startup_file"`
	NodeVersion string `json:"node_version"`
	Port        int    `json:"port"`
}

// HandleCreateNodeApp creates and starts a PM2 process safely under the specific user.
func HandleCreateNodeApp(ctx context.Context, payload []byte) (interface{}, error) {
	var params CreateNodeAppParams
	if err := protocol.ParsePayload[CreateNodeAppParams](&protocol.Task{Operation: string(OpCreateNodeApp), Payload: payload}); err != nil {
		return nil, err
	}

	if err := executor.ValidateUsername(params.Username); err != nil {
		return nil, err
	}
	if err := executor.ValidateAbsolutePath(params.AppPath); err != nil {
		return nil, err
	}
	if err := executor.ValidateDomain(params.Domain); err != nil {
		return nil, err
	}

	// Validate user exists
	u, err := user.Lookup(params.Username)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// We use `sudo` with exact array arguments to drop privileges securely without invoking a shell.
	// `su -c` invokes a shell and is prone to injection. `sudo -u` executes the binary directly.
	
	portEnv := fmt.Sprintf("PORT=%d", params.Port)
	
	// Start the app using sudo
	_, err = executor.Run(ctx, "/usr/bin/sudo", "-u", params.Username, "/usr/bin/env", portEnv, "/usr/bin/pm2", "start", params.StartupFile, "--name", params.AppName)
	if err != nil {
		_, err = executor.Run(ctx, "/usr/bin/sudo", "-u", params.Username, "/usr/bin/env", portEnv, "/usr/local/bin/pm2", "start", params.StartupFile, "--name", params.AppName)
		if err != nil {
			return nil, fmt.Errorf("failed to start node app via pm2: %w", err)
		}
	}
	
	// Save pm2 list for the user so it survives reboots
	executor.Run(ctx, "/usr/bin/sudo", "-u", params.Username, "/usr/bin/pm2", "save") //nolint:errcheck

	return map[string]interface{}{"status": "started", "port": params.Port}, nil
}
