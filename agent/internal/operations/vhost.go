package operations

import (
	"context"
	"fmt"
	"os"

	"github.com/hosting-panel/agent/internal/executor"
	"github.com/hosting-panel/agent/internal/protocol"
	"github.com/hosting-panel/agent/internal/webserver"
)

func HandleCreateVirtualHost(ctx context.Context, payload []byte) (interface{}, error) {
	var params webserver.VHostParams
	if err := protocol.ParsePayload[webserver.VHostParams](&protocol.Task{Operation: string(OpCreateVirtualHost), Payload: payload}); err != nil {
		return nil, err
	}

	// 1. Validate inputs
	if err := executor.ValidateDomain(params.Domain); err != nil {
		return nil, fmt.Errorf("invalid domain: %w", err)
	}
	if err := executor.ValidateUsername(params.Username); err != nil {
		return nil, fmt.Errorf("invalid username: %w", err)
	}
	if err := executor.ValidateAbsolutePath(params.DocumentRoot); err != nil {
		return nil, fmt.Errorf("invalid document root: %w", err)
	}
	if params.PHPVersion != "8.1" && params.PHPVersion != "8.2" && params.PHPVersion != "8.3" {
		params.PHPVersion = "8.2"
	}

	// 2. Create Document Root
	_, err := executor.Run(ctx, "/usr/bin/mkdir", "-p", params.DocumentRoot)
	if err != nil {
		_, _ = executor.Run(ctx, "/bin/mkdir", "-p", params.DocumentRoot)
	}
	
	// chown user:user document_root
	chownArg := fmt.Sprintf("%s:%s", params.Username, params.Username)
	_, err = executor.Run(ctx, "/usr/bin/chown", chownArg, params.DocumentRoot)
	if err != nil {
		_, _ = executor.Run(ctx, "/bin/chown", chownArg, params.DocumentRoot)
	}

	// 3. Delegate configuration to the active WebServer Driver
	driver := webserver.GetDriver()
	if err := driver.CreateVirtualHost(ctx, params); err != nil {
		return nil, fmt.Errorf("driver %s failed to create vhost: %w", driver.Name(), err)
	}

	return map[string]string{"status": "created", "domain": params.Domain, "server": driver.Name()}, nil
}
