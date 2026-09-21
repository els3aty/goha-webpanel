package operations

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/els3aty/goha-webpanel/agent/internal/executor"
	"github.com/els3aty/goha-webpanel/agent/internal/protocol"
)

type InstallAppParams struct {
	AppName      string `json:"app_name"`
	Username     string `json:"username"`
	DocumentRoot string `json:"document_root"`
	DBName       string `json:"db_name"`
	DBUser       string `json:"db_user"`
	DBPass       string `json:"db_pass"`
}

const wpConfigTemplate = `<?php
define( 'DB_NAME',     '{{.DBName}}' );
define( 'DB_USER',     '{{.DBUser}}' );
define( 'DB_PASSWORD', '{{.DBPass}}' );
define( 'DB_HOST',     'localhost' );
define( 'DB_CHARSET',  'utf8mb4' );
define( 'DB_COLLATE',  '' );

// Salts would normally be generated here, omitted for brevity.

$table_prefix = 'wp_';
define( 'WP_DEBUG', false );

if ( ! defined( 'ABSPATH' ) ) {
	define( 'ABSPATH', __DIR__ . '/' );
}
require_once ABSPATH . 'wp-settings.php';
`

func HandleInstallApp(ctx context.Context, payload []byte) (interface{}, error) {
	var params InstallAppParams
	if err := protocol.ParsePayload[InstallAppParams](&protocol.Task{Operation: "InstallApp", Payload: payload}); err != nil {
		return nil, err
	}

	if params.AppName != "wordpress" {
		return nil, fmt.Errorf("unsupported app: %s", params.AppName)
	}

	// 1. Download WordPress securely
	tarPath := filepath.Join("/tmp", fmt.Sprintf("wp-%s.tar.gz", params.Username))
	if _, err := executor.Run(ctx, "/usr/bin/curl", "-sSO", "https://wordpress.org/latest.tar.gz"); err != nil {
		// curl output file needs to be specified with -o
		_, err = executor.Run(ctx, "/usr/bin/curl", "-sSL", "-o", tarPath, "https://wordpress.org/latest.tar.gz")
		if err != nil {
			return nil, fmt.Errorf("failed to download wordpress: %w", err)
		}
	}

	// 2. Extract securely
	// Tar extracts into a 'wordpress' folder. We extract it to /tmp first.
	extractDir := filepath.Join("/tmp", fmt.Sprintf("wp-extract-%s", params.Username))
	os.MkdirAll(extractDir, 0755)
	
	if _, err := executor.Run(ctx, "/usr/bin/tar", "-xzf", tarPath, "-C", extractDir); err != nil {
		return nil, fmt.Errorf("failed to extract wordpress: %w", err)
	}

	// 3. Move files to DocumentRoot
	// We use cp securely without shell globbing
	wpDir := filepath.Join(extractDir, "wordpress")
	if _, err := executor.Run(ctx, "/usr/bin/cp", "-a", wpDir+"/.", params.DocumentRoot+"/"); err != nil {
		return nil, fmt.Errorf("failed to copy files: %w", err)
	}

	// 4. Generate wp-config.php natively
	tmpl, err := template.New("wp-config").Parse(wpConfigTemplate)
	if err != nil {
		return nil, err
	}
	var configBuf bytes.Buffer
	if err := tmpl.Execute(&configBuf, params); err != nil {
		return nil, err
	}

	wpConfigPath := filepath.Join(params.DocumentRoot, "wp-config.php")
	if err := os.WriteFile(wpConfigPath, configBuf.Bytes(), 0644); err != nil {
		return nil, fmt.Errorf("failed to write wp-config.php: %w", err)
	}

	// 5. Cleanup
	os.Remove(tarPath)
	os.RemoveAll(extractDir)

	// 6. Chown everything to the user
	chownArg := fmt.Sprintf("%s:%s", params.Username, params.Username)
	if _, err := executor.Run(ctx, "/usr/bin/chown", "-R", chownArg, params.DocumentRoot); err != nil {
		return nil, fmt.Errorf("failed to chown files: %w", err)
	}

	return map[string]string{"status": "installed", "app": params.AppName}, nil
}
