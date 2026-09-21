package webserver

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/els3aty/goha-webpanel/agent/internal/executor"
	"github.com/els3aty/goha-webpanel/agent/internal/osenv"
)

type NginxDriver struct{}

func (n *NginxDriver) Name() string {
	return "nginx"
}

// Nginx Template
const nginxTemplate = `server {
    listen 80;
{{if .SSLEnabled}}
    listen 443 ssl;
    ssl_certificate /etc/letsencrypt/live/{{.Domain}}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/{{.Domain}}/privkey.pem;
{{end}}
    server_name {{.Domain}} www.{{.Domain}};
    root {{.DocumentRoot}};
    index index.php index.html index.htm;

    access_log /var/log/nginx/{{.Domain}}.access.log;
    error_log /var/log/nginx/{{.Domain}}.error.log;

{{if or (eq .RuntimeType "nodejs") (eq .RuntimeType "python")}}
    location / {
        proxy_pass http://127.0.0.1:{{.RuntimePort}};
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
    }
{{else}}
    location / {
        try_files $uri $uri/ /index.php?$args;
    }

    location ~ \.php$ {
        include snippets/fastcgi-php.conf;
        fastcgi_pass unix:/run/php/php{{.PHPVersion}}-fpm-{{.Username}}.sock;
    }
{{end}}

    location ~ /\.ht {
        deny all;
    }
}
`

// PHP-FPM Pool Template
const fpmTemplate = `[{{.Domain}}]
user = {{.Username}}
group = {{.Username}}
listen = /run/php/php{{.PHPVersion}}-fpm-{{.Username}}.sock
listen.owner = www-data
listen.group = www-data
listen.mode = 0660
pm = dynamic
pm.max_children = 10
pm.start_servers = 2
pm.min_spare_servers = 1
pm.max_spare_servers = 3
php_admin_value[open_basedir] = {{.DocumentRoot}}:/tmp
`

func (n *NginxDriver) CreateVirtualHost(ctx context.Context, params VHostParams) error {
	nginxTmpl, err := template.New("nginx").Parse(nginxTemplate)
	if err != nil {
		return err
	}
	var nginxConfig bytes.Buffer
	if err := nginxTmpl.Execute(&nginxConfig, params); err != nil {
		return err
	}

	fpmTmpl, err := template.New("fpm").Parse(fpmTemplate)
	if err != nil {
		return err
	}
	var fpmConfig bytes.Buffer
	if err := fpmTmpl.Execute(&fpmConfig, params); err != nil {
		return err
	}

	env := osenv.Get()
	nginxVhostDir := env.NginxVhostDir()
	nginxSymlinkDir := env.NginxSymlinkDir()
	fpmDir := env.PHPFPMPoolDir(params.PHPVersion)

	nginxPath := filepath.Join(nginxVhostDir, params.Domain+".conf")
	fpmPath := filepath.Join(fpmDir, params.Domain+".conf")

	if err := os.WriteFile(nginxPath, nginxConfig.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write nginx config: %w", err)
	}

	if params.RuntimeType == "php" || params.RuntimeType == "" {
		// Ensure the FPM directory exists (especially important for custom Remi paths)
		os.MkdirAll(fpmDir, 0755)
		if err := os.WriteFile(fpmPath, fpmConfig.Bytes(), 0644); err != nil {
			return fmt.Errorf("failed to write fpm pool: %w", err)
		}
	}

	if nginxSymlinkDir != "" {
		symlinkPath := filepath.Join(nginxSymlinkDir, params.Domain+".conf")
		if _, err := os.Stat(symlinkPath); os.IsNotExist(err) {
			_, err = executor.Run(ctx, "/usr/bin/ln", "-s", nginxPath, symlinkPath)
			if err != nil {
				_, _ = executor.Run(ctx, "/bin/ln", "-s", nginxPath, symlinkPath)
			}
		}
	}

	if params.RuntimeType == "php" || params.RuntimeType == "" {
		executor.Run(ctx, "/usr/bin/systemctl", "reload", "php"+params.PHPVersion+"-fpm") //nolint:errcheck
		executor.Run(ctx, "/bin/systemctl", "reload", "php"+params.PHPVersion+"-fpm") //nolint:errcheck
	}

	return n.Reload(ctx)
}

const lbNginxTemplate = `upstream cluster_{{.Domain}} {
{{range .ComputeIPs}}
    server {{.}}:80 max_fails=3 fail_timeout=30s;
{{end}}
}

server {
    listen 80;
    server_name {{.Domain}} www.{{.Domain}};

    access_log /var/log/nginx/{{.Domain}}.lb.access.log;
    error_log /var/log/nginx/{{.Domain}}.lb.error.log;

    location / {
        proxy_pass http://cluster_{{.Domain}};
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
`

func (n *NginxDriver) ConfigureLoadBalancer(ctx context.Context, params LBParams) error {
	tmpl, err := template.New("lb-nginx").Parse(lbNginxTemplate)
	if err != nil {
		return err
	}
	var configBuf bytes.Buffer
	if err := tmpl.Execute(&configBuf, params); err != nil {
		return err
	}

	env := osenv.Get()
	nginxVhostDir := env.NginxVhostDir()
	nginxSymlinkDir := env.NginxSymlinkDir()

	nginxPath := filepath.Join(nginxVhostDir, params.Domain+".conf")
	if err := os.WriteFile(nginxPath, configBuf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write nginx config: %w", err)
	}

	if nginxSymlinkDir != "" {
		symlinkPath := filepath.Join(nginxSymlinkDir, params.Domain+".conf")
		if _, err := os.Stat(symlinkPath); os.IsNotExist(err) {
			_, err = executor.Run(ctx, "/usr/bin/ln", "-s", nginxPath, symlinkPath)
			if err != nil {
				_, _ = executor.Run(ctx, "/bin/ln", "-s", nginxPath, symlinkPath)
			}
		}
	}

	return n.Reload(ctx)
}

func (n *NginxDriver) Reload(ctx context.Context) error {
	env := osenv.Get()
	_, err := executor.Run(ctx, "/usr/bin/systemctl", "reload", env.NginxService())
	if err != nil {
		_, err = executor.Run(ctx, "/bin/systemctl", "reload", env.NginxService())
	}
	return err
}
