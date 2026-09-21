# Web Server Abstraction & OpenLiteSpeed (Phase 17)

This document describes how GohaHost supports multiple Web Servers interchangeably.

## 1. Goal
To prevent hardcoding Nginx dependencies throughout the codebase, allowing hosting providers to choose the web server that best fits their load (e.g. Nginx for raw proxying, OpenLiteSpeed for WordPress caching).

## 2. WebServerDriver Interface
The Control Plane is completely unaware of the underlying Web Server. It simply says "Create VirtualHost".
The Agent receives this command and delegates it to the `webserver.Driver` interface:
```go
type Driver interface {
	Name() string
	CreateVirtualHost(ctx context.Context, params VHostParams) error
	ConfigureLoadBalancer(ctx context.Context, params LBParams) error
	Reload(ctx context.Context) error
}
```

## 3. Supported Drivers
1. **NginxDriver**:
   - Renders `.conf` files to `/etc/nginx/sites-available`.
   - Renders `php-fpm` pool files to `/etc/php/8.x/fpm/pool.d`.
   - Symlinks and reloads `systemctl reload nginx`.
2. **OpenLiteSpeedDriver**:
   - Renders `vhconf.conf` XML/text format to `/usr/local/lsws/conf/vhosts/`.
   - Defines LiteSpeed-native `lsapi` context blocks for PHP integration.
   - Restarts via `lswsctrl restart`.

## 4. The Future
This abstraction makes it trivial to add an Apache (`httpd`) driver or a Caddy driver if needed, without changing any Control Plane or Operations logic.
