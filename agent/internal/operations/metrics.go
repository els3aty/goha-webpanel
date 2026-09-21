package operations

import (
	"context"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/els3aty/goha-webpanel/agent/internal/executor"
)

type NodeMetrics struct {
	CPU      CPUStats               `json:"cpu"`
	Memory   MemStats               `json:"memory"`
	Disk     DiskStats              `json:"disk"`
	Services map[string]ServiceStat `json:"services"`
}

type CPUStats struct {
	Load1  float64 `json:"load_1"`
	Load5  float64 `json:"load_5"`
	Load15 float64 `json:"load_15"`
}

type MemStats struct {
	TotalMB int64 `json:"total_mb"`
	FreeMB  int64 `json:"free_mb"`
	UsedMB  int64 `json:"used_mb"`
}

type DiskStats struct {
	TotalGB float64 `json:"total_gb"`
	FreeGB  float64 `json:"free_gb"`
	UsedGB  float64 `json:"used_gb"`
	UsagePct float64 `json:"usage_pct"`
}

type ServiceStat struct {
	Status string `json:"status"` // "active", "inactive", "failed"
}

// HandleGetMetrics gathers node metrics natively (Zero-Shell)
func HandleGetMetrics(ctx context.Context, payload []byte) (interface{}, error) {
	metrics := NodeMetrics{
		Services: make(map[string]ServiceStat),
	}

	// 1. CPU Load
	if loadBytes, err := os.ReadFile("/proc/loadavg"); err == nil {
		parts := strings.Fields(string(loadBytes))
		if len(parts) >= 3 {
			metrics.CPU.Load1, _ = strconv.ParseFloat(parts[0], 64)
			metrics.CPU.Load5, _ = strconv.ParseFloat(parts[1], 64)
			metrics.CPU.Load15, _ = strconv.ParseFloat(parts[2], 64)
		}
	}

	// 2. Memory
	if memBytes, err := os.ReadFile("/proc/meminfo"); err == nil {
		lines := strings.Split(string(memBytes), "\n")
		var memTotal, memFree, memAvailable, buffers, cached int64
		for _, line := range lines {
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}
			val, _ := strconv.ParseInt(parts[1], 10, 64) // in KB
			switch parts[0] {
			case "MemTotal:":
				memTotal = val
			case "MemFree:":
				memFree = val
			case "MemAvailable:":
				memAvailable = val
			case "Buffers:":
				buffers = val
			case "Cached:":
				cached = val
			}
		}

		metrics.Memory.TotalMB = memTotal / 1024
		
		// If MemAvailable is present (modern Linux), use it for Free. Otherwise calculate.
		if memAvailable > 0 {
			metrics.Memory.FreeMB = memAvailable / 1024
		} else {
			metrics.Memory.FreeMB = (memFree + buffers + cached) / 1024
		}
		metrics.Memory.UsedMB = metrics.Memory.TotalMB - metrics.Memory.FreeMB
	}

	// 3. Disk (Root partition)
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err == nil {
		total := float64(stat.Blocks*uint64(stat.Bsize)) / (1024 * 1024 * 1024)
		free := float64(stat.Bavail*uint64(stat.Bsize)) / (1024 * 1024 * 1024)
		used := total - free
		metrics.Disk.TotalGB = total
		metrics.Disk.FreeGB = free
		metrics.Disk.UsedGB = used
		if total > 0 {
			metrics.Disk.UsagePct = (used / total) * 100
		}
	}

	// 4. Services
	servicesToMonitor := []string{"nginx", "php8.3-fpm", "mariadb", "postfix", "dovecot"}
	for _, svc := range servicesToMonitor {
		// systemctl is-active returns 0 if active, >0 if inactive/failed
		// Using strict exec.Command via executor.Run prevents shell injection
		out, err := executor.Run(ctx, "/usr/bin/systemctl", "is-active", svc)
		statusStr := strings.TrimSpace(string(out))
		if err != nil && statusStr == "" {
			statusStr = "failed/unknown"
		}
		metrics.Services[svc] = ServiceStat{Status: statusStr}
	}

	return metrics, nil
}
