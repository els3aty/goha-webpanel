package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/els3aty/goha-webpanel/control-plane/internal/agentclient"
	"github.com/els3aty/goha-webpanel/control-plane/internal/audit"
	"github.com/els3aty/goha-webpanel/control-plane/internal/auth"
	"github.com/els3aty/goha-webpanel/control-plane/internal/models"
	"github.com/els3aty/goha-webpanel/control-plane/internal/rbac"
)

type DNSHandler struct {
	dnsStore     *models.DNSStore
	hostingStore *models.HostingStore
	nodeStore    *models.NodeStore
	agentClient  *agentclient.Client
	auditLog     *audit.Logger
	encryptor    auth.Encryptor
}

func NewDNSHandler(
	dnsStore *models.DNSStore,
	hostingStore *models.HostingStore,
	nodeStore *models.NodeStore,
	agentClient *agentclient.Client,
	auditLog *audit.Logger,
	encryptor auth.Encryptor,
) *DNSHandler {
	return &DNSHandler{
		dnsStore:     dnsStore,
		hostingStore: hostingStore,
		nodeStore:    nodeStore,
		agentClient:  agentClient,
		auditLog:     auditLog,
		encryptor:    encryptor,
	}
}

// ── Requests ──────────────────────────────────────────────────────────────────

type CreateDNSZoneRequest struct {
	HostingUserID uuid.UUID `json:"hosting_user_id"`
	Domain        string    `json:"domain"`
}

type CreateDNSRecordRequest struct {
	ZoneID   uuid.UUID `json:"zone_id"`
	Name     string    `json:"name"`
	Type     string    `json:"type"`
	Content  string    `json:"content"`
	TTL      int       `json:"ttl"`
	Priority *int      `json:"priority,omitempty"`
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func (h *DNSHandler) CreateZone(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req CreateDNSZoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	user, err := h.hostingStore.GetHostingUser(r.Context(), req.HostingUserID)
	if err != nil || user == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "hosting user not found"})
		return
	}
	if p.Role != rbac.RoleAdmin && p.Role != rbac.RoleSuperAdmin && user.CustomerID != p.UserID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "access denied"})
		return
	}

	zone := &models.DNSZone{
		HostingUserID: user.ID,
		Domain:        req.Domain,
		Status:        models.DNSZoneStatusActive,
		Serial:        time.Now().Unix(), // Initial serial
	}
	if err := h.dnsStore.CreateZone(r.Context(), zone); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create zone record"})
		return
	}

	node, _ := h.nodeStore.GetByID(r.Context(), user.NodeID)
	keyBytes, _ := h.encryptor.Decrypt(node.SigningKeyEncrypted)
	builder := agentclient.NewTaskBuilder(node.ID.String(), keyBytes, 5*time.Minute)
	
	// Payload only sends domain and serial for an empty zone file creation
	payload := map[string]any{
		"domain": req.Domain,
		"serial": zone.Serial,
		"records": []any{},
	}
	task, _ := builder.Build(uuid.New().String(), "CreateDNSZone", payload, "idemp-zone-"+zone.ID.String())
	
	result, err := h.agentClient.SendTask(r.Context(), node.Address, task)
	if err != nil || result.Status != "succeeded" {
		h.auditLog.LogFromContext(r.Context(), "dns.zone.create", "dns_zone", &zone.ID, audit.ResultFailure, map[string]any{"error": err})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "agent failed to create dns zone"})
		return
	}

	h.auditLog.LogFromContext(r.Context(), "dns.zone.create", "dns_zone", &zone.ID, audit.ResultSuccess, nil)
	writeJSON(w, http.StatusOK, zone)
}

func (h *DNSHandler) CreateRecord(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req CreateDNSRecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	zone, err := h.dnsStore.GetZone(r.Context(), req.ZoneID)
	if err != nil || zone == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "zone not found"})
		return
	}

	user, _ := h.hostingStore.GetHostingUser(r.Context(), zone.HostingUserID)
	if p.Role != rbac.RoleAdmin && p.Role != rbac.RoleSuperAdmin && user.CustomerID != p.UserID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "access denied"})
		return
	}

	if req.TTL <= 0 {
		req.TTL = 3600
	}

	record := &models.DNSRecord{
		ZoneID:   zone.ID,
		Name:     req.Name,
		Type:     req.Type,
		Content:  req.Content,
		TTL:      req.TTL,
		Priority: req.Priority,
	}
	if err := h.dnsStore.CreateRecord(r.Context(), record); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create record"})
		return
	}

	// Update Serial
	newSerial, err := h.dnsStore.IncrementSerial(r.Context(), zone.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to increment serial"})
		return
	}

	// Fetch all records for the zone to recreate the file
	records, _ := h.dnsStore.ListRecordsByZone(r.Context(), zone.ID)

	node, _ := h.nodeStore.GetByID(r.Context(), user.NodeID)
	keyBytes, _ := h.encryptor.Decrypt(node.SigningKeyEncrypted)
	builder := agentclient.NewTaskBuilder(node.ID.String(), keyBytes, 5*time.Minute)
	
	payload := map[string]any{
		"domain":  zone.Domain,
		"serial":  newSerial,
		"records": records, // Will be marshaled into array of records
	}
	task, _ := builder.Build(uuid.New().String(), "CreateDNSZone", payload, "idemp-zone-update-"+record.ID.String())
	
	result, err := h.agentClient.SendTask(r.Context(), node.Address, task)
	if err != nil || result.Status != "succeeded" {
		h.auditLog.LogFromContext(r.Context(), "dns.record.create", "dns_record", &record.ID, audit.ResultFailure, map[string]any{"error": err})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "agent failed to update dns zone"})
		return
	}

	h.auditLog.LogFromContext(r.Context(), "dns.record.create", "dns_record", &record.ID, audit.ResultSuccess, nil)
	writeJSON(w, http.StatusOK, record)
}
