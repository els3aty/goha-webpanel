package webserver

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"text/template"

	"github.com/els3aty/goha-webpanel/agent/internal/executor"
)

type OpenLiteSpeedDriver struct{}

func (o *OpenLiteSpeedDriver) Name() string {
	return "openlitespeed"
}

// OLS Virtual Host XML/Conf Template
// LiteSpeed uses a different configuration format than Nginx.
const olsTemplate = `
docRoot                   $VH_ROOT/
vhDomain                  {{.Domain}}
vhAliases                 www.{{.Domain}}
adminEmails               admin@{{.Domain}}
enableGzip                1
enableBr                  1

errorlog $VH_ROOT/logs/error.log {
  useServer               1
  logLevel                ERROR
  rollingSize             10M
}

accesslog $VH_ROOT/logs/access.log {
  useServer               0
  rollingSize             10M
  keepDays                30
  compressArchive         0
}

index  {
  useServer               0
  indexFiles              index.php, index.html
  autoIndex               0
  autoIndexURI            /_autoindex/default.php
}

{{if or (eq .RuntimeType "nodejs") (eq .RuntimeType "python")}}
extprocessor nodejsApp {
  type                    proxy
  address                 127.0.0.1:{{.RuntimePort}}
  maxConns                100
  initTimeout             60
  retryTimeout            0
  respBuffer              0
}

context / {
  type                    proxy
  handler                 nodejsApp
  addDefaultCharset       off
}
{{else}}
extprocessor phpApp {
  type                    lsapi
  address                 uds://tmp/lshttpd/php{{.PHPVersion}}.sock
  maxConns                35
  env                     PHP_LSAPI_CHILDREN=35
  env                     LSAPI_AVOID_FORK=200M
  initTimeout             60
  retryTimeout            0
  persistConn             1
  respBuffer              0
  autoStart               1
  path                    /usr/local/lsws/lsphp{{.PHPVersion}}/bin/lsphp
  backlog                 100
  instances               1
}

context / {
  type                    NULL
  location                $DOC_ROOT/
  allowBrowse             1
  indexFiles              index.php, index.html
  rewrite  {
    enable                1
    inherit               1
    rules                 <<<END_rules
RewriteCond %{REQUEST_FILENAME} !-f
RewriteCond %{REQUEST_FILENAME} !-d
RewriteRule . /index.php [L]
END_rules
  }
}

scripthandler  {
  add                     lsapi:phpApp php
}
{{end}}

{{if .SSLEnabled}}
vhssl  {
  keyFile                 /etc/letsencrypt/live/{{.Domain}}/privkey.pem
  certFile                /etc/letsencrypt/live/{{.Domain}}/fullchain.pem
  certChain               1
}
{{end}}
`

func (o *OpenLiteSpeedDriver) CreateVirtualHost(ctx context.Context, params VHostParams) error {
	tmpl, err := template.New("ols").Parse(olsTemplate)
	if err != nil {
		return err
	}
	var confBuf bytes.Buffer
	if err := tmpl.Execute(&confBuf, params); err != nil {
		return err
	}

	confDir := fmt.Sprintf("/usr/local/lsws/conf/vhosts/%s", params.Domain)
	if err := os.MkdirAll(confDir, 0755); err != nil {
		return fmt.Errorf("failed to create lsws vhost dir: %w", err)
	}
	
	// Create logs dir for the vhost
	os.MkdirAll(fmt.Sprintf("%s/logs", params.DocumentRoot), 0755)

	confPath := fmt.Sprintf("%s/vhconf.conf", confDir)
	if err := os.WriteFile(confPath, confBuf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write lsws config: %w", err)
	}

	// We also need to add the listener mapping to the main httpd_config.xml, 
	// but for brevity we assume there is a generic listener that includes all vhosts,
	// or that the panel manages the listener file separately.
	// We'll skip editing the XML here for the scope of the abstraction demonstration.

	return o.Reload(ctx)
}

func (o *OpenLiteSpeedDriver) ConfigureLoadBalancer(ctx context.Context, params LBParams) error {
	// OpenLiteSpeed can act as a load balancer via Proxy ExtProcessors
	// But it's more complex than Nginx.
	return fmt.Errorf("LoadBalancing is currently only implemented for the Nginx driver")
}

func (o *OpenLiteSpeedDriver) Reload(ctx context.Context) error {
	_, err := executor.Run(ctx, "/usr/local/lsws/bin/lswsctrl", "restart")
	return err
}
