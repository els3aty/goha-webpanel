package operations

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hosting-panel/agent/internal/executor"
	"github.com/hosting-panel/agent/internal/protocol"
	"github.com/hosting-panel/agent/internal/webserver"
)

func HandleIssueSSL(ctx context.Context, payload []byte) (interface{}, error) {
	var params webserver.VHostParams // Reuse the same params as CreateVirtualHost since we need them for templating
	if err := protocol.ParsePayload[webserver.VHostParams](&protocol.Task{Operation: "IssueSSL", Payload: payload}); err != nil {
		return nil, err
	}

	if err := executor.ValidateDomain(params.Domain); err != nil {
		return nil, fmt.Errorf("invalid domain: %w", err)
	}

	// 1. Run Certbot in Webroot Mode (Zero-Shell, safe execution)
	certbotArgs := []string{
		"certonly",
		"--webroot",
		"-w", params.DocumentRoot,
		"-d", params.Domain,
		"-d", "www." + params.Domain,
		"--non-interactive",
		"--agree-tos",
		"-m", "admin@" + params.Domain,
	}
	
	// We run certbot. If certbot is not installed on dev machine, it will fail, which is expected.
	// But in a real environment it successfully fetches the certs to /etc/letsencrypt/live/...
	if _, err := executor.Run(ctx, "/usr/bin/certbot", certbotArgs...); err != nil {
		return nil, fmt.Errorf("certbot failed: %w", err)
	}

	// 2. Enable SSL in our parameters
	params.SSLEnabled = true

	// 3. Re-generate the Nginx config with the new SSL blocks by calling HandleCreateVirtualHost internally
	// Wait, calling HandleCreateVirtualHost directly is perfectly safe, but it requires a JSON payload
	newPayload, _ := json.Marshal(params)
	
	_, err := HandleCreateVirtualHost(ctx, newPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to rewrite nginx config for SSL: %w", err)
	}

	return map[string]string{"status": "ssl_issued", "domain": params.Domain}, nil
}
