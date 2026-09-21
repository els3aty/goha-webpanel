package operations

import (
	"context"
	"fmt"
	"os"

	"github.com/els3aty/goha-webpanel/agent/internal/executor"
	"github.com/els3aty/goha-webpanel/agent/internal/protocol"
)

type CreateHostingUserParams struct {
	Username string `json:"username"`
}

func HandleCreateHostingUser(ctx context.Context, payload []byte) (interface{}, error) {
	var params CreateHostingUserParams
	if err := protocol.ParsePayload[CreateHostingUserParams](&protocol.Task{Operation: string(OpCreateHostingUser), Payload: payload}); err != nil {
		return nil, err
	}

	// 1. Strict Validation
	if err := executor.ValidateUsername(params.Username); err != nil {
		return nil, fmt.Errorf("invalid username: %w", err)
	}

	homeDir := fmt.Sprintf("/home/%s", params.Username)

	// 2. Check if user already exists
	_, err := os.Stat(homeDir)
	if err == nil {
		// Idempotency: User already exists.
		return map[string]string{"status": "already_exists", "home": homeDir}, nil
	}

	// 3. Create user securely using absolute path and explicit args
	// useradd -m -d /home/username -s /bin/false username
	_, err = executor.Run(ctx, "/usr/sbin/useradd", "-m", "-d", homeDir, "-s", "/bin/false", params.Username)
	if err != nil {
		// Fallback to /usr/bin/useradd just in case
		_, err = executor.Run(ctx, "/usr/bin/useradd", "-m", "-d", homeDir, "-s", "/bin/false", params.Username)
		if err != nil {
			return nil, fmt.Errorf("failed to create linux user: %w", err)
		}
	}

	// 4. Set correct permissions for home directory (750 so www-data can't read other users' homes)
	_, err = executor.Run(ctx, "/usr/bin/chmod", "750", homeDir)
	if err != nil {
		_, err = executor.Run(ctx, "/bin/chmod", "750", homeDir)
		if err != nil {
			return nil, fmt.Errorf("failed to chmod home dir: %w", err)
		}
	}

	// 5. Create public_html
	publicHTML := homeDir + "/public_html"
	_, err = executor.Run(ctx, "/usr/bin/mkdir", "-p", publicHTML)
	if err != nil {
		_, err = executor.Run(ctx, "/bin/mkdir", "-p", publicHTML)
	}
	
	// chown username:username public_html
	_, err = executor.Run(ctx, "/usr/bin/chown", params.Username+":"+params.Username, publicHTML)
	if err != nil {
		_, err = executor.Run(ctx, "/bin/chown", params.Username+":"+params.Username, publicHTML)
	}

	return map[string]string{"status": "created", "home": homeDir}, nil
}

func init() {
	// Notice: We don't register it in init() since Registry is explicitly created in main.
	// But this file will be referenced in agent's main.go.
}
