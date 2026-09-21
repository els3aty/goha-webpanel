module github.com/els3aty/goha-webpanel/agent

go 1.22

require (
	golang.org/x/crypto v0.27.0
	github.com/els3aty/goha-webpanel/control-plane v0.0.0
)

replace github.com/els3aty/goha-webpanel/control-plane => ../control-plane