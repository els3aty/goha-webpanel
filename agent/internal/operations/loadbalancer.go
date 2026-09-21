package operations

import (
	"context"
	"fmt"

	"github.com/hosting-panel/agent/internal/executor"
	"github.com/hosting-panel/agent/internal/protocol"
	"github.com/hosting-panel/agent/internal/webserver"
)

func HandleConfigureLoadBalancer(ctx context.Context, payload []byte) (interface{}, error) {
	var params webserver.LBParams
	if err := protocol.ParsePayload[webserver.LBParams](&protocol.Task{Operation: "ConfigureLoadBalancer", Payload: payload}); err != nil {
		return nil, err
	}

	if err := executor.ValidateDomain(params.Domain); err != nil {
		return nil, fmt.Errorf("invalid domain: %w", err)
	}

	if len(params.ComputeIPs) == 0 {
		return nil, fmt.Errorf("at least one compute IP is required")
	}

	driver := webserver.GetDriver()
	if err := driver.ConfigureLoadBalancer(ctx, params); err != nil {
		return nil, fmt.Errorf("driver %s failed to configure loadbalancer: %w", driver.Name(), err)
	}

	return map[string]string{"status": "lb_configured", "domain": params.Domain, "backends": fmt.Sprintf("%d", len(params.ComputeIPs))}, nil
}

