package operations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/hosting-panel/agent/internal/executor"
	"github.com/hosting-panel/agent/internal/protocol"
)

type DNSRecord struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Priority *int   `json:"priority,omitempty"`
}

type CreateDNSZoneParams struct {
	Domain  string      `json:"domain"`
	Serial  int64       `json:"serial"`
	Records []DNSRecord `json:"records"`
}

type DeleteDNSZoneParams struct {
	Domain string `json:"domain"`
}

// Regex for DNS validation
var validDNSRecordType = regexp.MustCompile(`^(A|AAAA|CNAME|MX|TXT|SRV|NS)$`)
var validDNSName = regexp.MustCompile(`^([a-zA-Z0-9_\-\.\@]+)$`)

const pdnsAPIURL = "http://127.0.0.1:8081/api/v1/servers/localhost/zones"
const pdnsAPIKey = "changeme" // In production, this would be loaded from agent config

// Helper to make PowerDNS API requests
func pdnsRequest(method, url string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", pdnsAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	return client.Do(req)
}

// EnsureFQDN adds a trailing dot if missing
func ensureFQDN(domain string) string {
	if !strings.HasSuffix(domain, ".") {
		return domain + "."
	}
	return domain
}

// HandleCreateDNSZone creates a zone in PowerDNS via REST API
func HandleCreateDNSZone(ctx context.Context, payload []byte) (interface{}, error) {
	var params CreateDNSZoneParams
	if err := protocol.ParsePayload[CreateDNSZoneParams](&protocol.Task{Operation: string(OpCreateDNSZone), Payload: payload}); err != nil {
		return nil, err
	}

	if err := executor.ValidateDomain(params.Domain); err != nil {
		return nil, fmt.Errorf("invalid domain: %w", err)
	}

	fqdnDomain := ensureFQDN(params.Domain)

	// 1. Create Zone
	zonePayload := map[string]interface{}{
		"name":        fqdnDomain,
		"kind":        "Master",
		"nameservers": []string{"ns1." + fqdnDomain, "ns2." + fqdnDomain},
	}

	resp, err := pdnsRequest("POST", pdnsAPIURL, zonePayload)
	if err != nil {
		return nil, fmt.Errorf("failed to contact PowerDNS API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 && resp.StatusCode != 409 { // 409 Conflict = already exists
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("PowerDNS API error creating zone: %s (status %d)", string(b), resp.StatusCode)
	}

	// 2. Add Records (if any)
	if len(params.Records) > 0 {
		var rrsets []map[string]interface{}
		for _, rec := range params.Records {
			if !validDNSRecordType.MatchString(rec.Type) {
				return nil, fmt.Errorf("invalid record type: %s", rec.Type)
			}
			
			recName := rec.Name
			if recName == "@" {
				recName = fqdnDomain
			} else if !strings.HasSuffix(recName, ".") {
				recName = recName + "." + fqdnDomain
			}

			rrsets = append(rrsets, map[string]interface{}{
				"name":       recName,
				"type":       rec.Type,
				"ttl":        rec.TTL,
				"changetype": "REPLACE",
				"records": []map[string]interface{}{
					{"content": rec.Content, "disabled": false},
				},
			})
		}

		patchPayload := map[string]interface{}{"rrsets": rrsets}
		patchResp, err := pdnsRequest("PATCH", pdnsAPIURL+"/"+fqdnDomain, patchPayload)
		if err != nil {
			return nil, fmt.Errorf("failed to patch records: %w", err)
		}
		defer patchResp.Body.Close()

		if patchResp.StatusCode != 204 {
			b, _ := io.ReadAll(patchResp.Body)
			return nil, fmt.Errorf("PowerDNS API error patching records: %s (status %d)", string(b), patchResp.StatusCode)
		}
	}

	return map[string]string{"status": "zone_created", "domain": params.Domain}, nil
}

// HandleDeleteDNSZone deletes a zone in PowerDNS via REST API
func HandleDeleteDNSZone(ctx context.Context, payload []byte) (interface{}, error) {
	var params DeleteDNSZoneParams
	if err := protocol.ParsePayload[DeleteDNSZoneParams](&protocol.Task{Operation: string(OpDeleteDNSZone), Payload: payload}); err != nil {
		return nil, err
	}
	if err := executor.ValidateDomain(params.Domain); err != nil {
		return nil, fmt.Errorf("invalid domain: %w", err)
	}

	fqdnDomain := ensureFQDN(params.Domain)
	resp, err := pdnsRequest("DELETE", pdnsAPIURL+"/"+fqdnDomain, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to contact PowerDNS API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 && resp.StatusCode != 404 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("PowerDNS API error deleting zone: %s", string(b))
	}

	return map[string]string{"status": "zone_deleted", "domain": params.Domain}, nil
}
