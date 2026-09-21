package webserver

import (
	"context"
)

type VHostParams struct {
	Domain       string
	Username     string
	DocumentRoot string
	PHPVersion   string
	RuntimeType  string
	RuntimePort  int
	SSLEnabled   bool
}

type LBParams struct {
	Domain     string
	ComputeIPs []string
}

// Driver defines the standard interface that any Web Server (Nginx, OpenLiteSpeed) must implement.
type Driver interface {
	Name() string
	CreateVirtualHost(ctx context.Context, params VHostParams) error
	ConfigureLoadBalancer(ctx context.Context, params LBParams) error
	Reload(ctx context.Context) error
}

var currentDriver Driver

// Init automatically detects the configured web server on startup
func Init() {
	// For now, default to Nginx. In a real environment, we'd read this from a config file or check installed binaries.
	currentDriver = &NginxDriver{}
}

// GetDriver returns the active Web Server driver
func GetDriver() Driver {
	if currentDriver == nil {
		Init()
	}
	return currentDriver
}

// SetDriver allows overriding the detected driver (useful for testing or explicit configuration)
func SetDriver(d Driver) {
	currentDriver = d
}
