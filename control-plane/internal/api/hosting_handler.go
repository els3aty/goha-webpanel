package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/hosting-panel/control-plane/internal/agentclient"
	"github.com/hosting-panel/control-plane/internal/audit"
	"github.com/hosting-panel/control-plane/internal/auth"
	"github.com/hosting-panel/control-plane/internal/models"
	"github.com/hosting-panel/control-plane/internal/rbac"
)

type HostingHandler struct {
	hostingStore *models.HostingStore
	nodeStore    *models.NodeStore
	pkgStore     *models.PackageStore
	agentClient  *agentclient.Client
	auditLog     *audit.Logger
	encryptor    auth.Encryptor // used to decrypt node signing keys
}

func NewHostingHandler(
	hostingStore *models.HostingStore,
	nodeStore *models.NodeStore,
	pkgStore *models.PackageStore,
	agentClient *agentclient.Client,
	auditLog *audit.Logger,
	encryptor auth.Encryptor,
) *HostingHandler {
	return &HostingHandler{
		hostingStore: hostingStore,
		nodeStore:    nodeStore,
		pkgStore:     pkgStore,
		agentClient:  agentClient,
		auditLog:     auditLog,
		encryptor:    encryptor,
	}
}

// ── Requests ──────────────────────────────────────────────────────────────────

type CreateHostingUserRequest struct {
	NodeID    uuid.UUID `json:"node_id"`
	PackageID uuid.UUID `json:"package_id"`
	Username  string    `json:"username"`
}

type CreateVirtualHostRequest struct {
	HostingUserID uuid.UUID `json:"hosting_user_id"`
	Domain        string    `json:"domain"`
	RuntimeType   string    `json:"runtime_type"` // e.g. "php", "nodejs", "python"
	RuntimePort   *int      `json:"runtime_port,omitempty"` // used for nodejs/python
	PHPVersion    string    `json:"php_version"`  // used for php
}

type IssueSSLRequest struct {
	VirtualHostID uuid.UUID `json:"virtual_host_id"`
	AdminEmail    string    `json:"admin_email"` // Email for Let's Encrypt registration
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// CreateHostingUser handles POST /api/hosting/users
func (h *HostingHandler) CreateHostingUser(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req CreateHostingUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	// Verify Node exists
	node, err := h.nodeStore.GetByID(r.Context(), req.NodeID)
	if err != nil || node == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node not found"})
		return
	}

	// Verify Package exists
	pkg, err := h.pkgStore.GetPackage(r.Context(), req.PackageID)
	if err != nil || pkg == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "package not found"})
		return
	}

	// Enforce Reseller role restrictions if the principal is a reseller
	if p.Role == rbac.RoleReseller {
		// In a full implementation, check if the package belongs to the reseller or is globally available.
		// For now, any active package is valid.
	}

	// Create record in DB
	hostingUser := &models.HostingUser{
		CustomerID: p.UserID,
		NodeID:     node.ID,
		PackageID:  pkg.ID,
		Username:   req.Username,
		Status:     models.HostingUserStatusActive,
	}
	if err := h.hostingStore.CreateHostingUser(r.Context(), hostingUser); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create user in db"})
		return
	}

	// Get Node signing key
	keyBytes, err := h.encryptor.Decrypt(node.SigningKeyEncrypted)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to decrypt node key"})
		return
	}

	// Build Task
	builder := agentclient.NewTaskBuilder(node.ID.String(), keyBytes, 5*time.Minute)
	payload := map[string]any{
		"username":      req.Username,
		"disk_quota_mb": pkg.DiskQuotaMB,
	}
	task, err := builder.Build(uuid.New().String(), "CreateHostingUser", payload, "idemp-"+hostingUser.ID.String())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to build task"})
		return
	}

	// Send Task to Agent
	result, err := h.agentClient.SendTask(r.Context(), node.Address, task)
	if err != nil || result.Status != "succeeded" {
		// Log error, potentially update DB status to failed
		h.auditLog.LogFromContext(r.Context(), "hosting.user.create", "hosting_user", &hostingUser.ID, audit.ResultFailure, map[string]any{"error": err.Error()})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "agent failed to create user"})
		return
	}

	h.auditLog.LogFromContext(r.Context(), "hosting.user.create", "hosting_user", &hostingUser.ID, audit.ResultSuccess, nil)
	writeJSON(w, http.StatusOK, hostingUser)
}

// CreateVirtualHost handles POST /api/hosting/vhosts
func (h *HostingHandler) CreateVirtualHost(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req CreateVirtualHostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	// Get HostingUser to find target Node
	user, err := h.hostingStore.GetHostingUser(r.Context(), req.HostingUserID)
	if err != nil || user == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "hosting user not found"})
		return
	}
	
	// Enforce ownership
	if p.Role != rbac.RoleAdmin && p.Role != rbac.RoleSuperAdmin {
		if user.CustomerID != p.UserID {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "access denied"})
			return
		}
	}

	node, err := h.nodeStore.GetByID(r.Context(), user.NodeID)
	if err != nil || node == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "node not found"})
		return
	}

	// Validate runtime defaults
	if req.RuntimeType == "" {
		req.RuntimeType = "php"
	}
	if req.RuntimeType != "php" && req.RuntimePort == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "runtime_port is required for non-PHP runtimes"})
		return
	}

	vhost := &models.VirtualHost{
		HostingUserID: user.ID,
		Domain:        req.Domain,
		DocumentRoot:  "/home/" + user.Username + "/public_html/" + req.Domain,
		RuntimeType:   req.RuntimeType,
		RuntimePort:   req.RuntimePort,
		PHPVersion:    &req.PHPVersion,
		SSLEnabled:    false,
		Status:        models.VHostStatusActive,
	}

	if err := h.hostingStore.CreateVirtualHost(r.Context(), vhost); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create vhost in db"})
		return
	}

	keyBytes, err := h.encryptor.Decrypt(node.SigningKeyEncrypted)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	builder := agentclient.NewTaskBuilder(node.ID.String(), keyBytes, 5*time.Minute)
	payload := map[string]any{
		"domain":        req.Domain,
		"username":      user.Username,
		"document_root": vhost.DocumentRoot,
		"runtime_type":  vhost.RuntimeType,
		"php_version":   req.PHPVersion,
	}
	if vhost.RuntimePort != nil {
		payload["runtime_port"] = *vhost.RuntimePort
	}
	
	task, err := builder.Build(uuid.New().String(), "CreateVirtualHost", payload, "idemp-"+vhost.ID.String())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to build task"})
		return
	}

	result, err := h.agentClient.SendTask(r.Context(), node.Address, task)
	if err != nil || result.Status != "succeeded" {
		h.auditLog.LogFromContext(r.Context(), "hosting.vhost.create", "virtual_host", &vhost.ID, audit.ResultFailure, map[string]any{"error": err.Error()})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "agent failed to create vhost"})
		return
	}

	h.auditLog.LogFromContext(r.Context(), "hosting.vhost.create", "virtual_host", &vhost.ID, audit.ResultSuccess, nil)
	writeJSON(w, http.StatusOK, vhost)
}

// IssueSSL handles POST /api/hosting/ssl
func (h *HostingHandler) IssueSSL(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req IssueSSLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	vhost, err := h.hostingStore.GetVirtualHostByID(r.Context(), req.VirtualHostID)
	if err != nil || vhost == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "virtual host not found"})
		return
	}

	user, err := h.hostingStore.GetHostingUser(r.Context(), vhost.HostingUserID)
	if err != nil || user == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Enforce ownership
	if p.Role != rbac.RoleAdmin && p.Role != rbac.RoleSuperAdmin {
		if user.CustomerID != p.UserID {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "access denied"})
			return
		}
	}

	node, err := h.nodeStore.GetByID(r.Context(), user.NodeID)
	if err != nil || node == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "node not found"})
		return
	}

	keyBytes, err := h.encryptor.Decrypt(node.SigningKeyEncrypted)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	builder := agentclient.NewTaskBuilder(node.ID.String(), keyBytes, 15*time.Minute) // 15 mins for certbot
	payload := map[string]any{
		"domain":        vhost.Domain,
		"username":      user.Username,
		"document_root": vhost.DocumentRoot,
		"php_version":   *vhost.PHPVersion,
		"runtime_type":  vhost.RuntimeType,
	}
	if vhost.RuntimePort != nil {
		payload["runtime_port"] = *vhost.RuntimePort
	}

	task, err := builder.Build(uuid.New().String(), "IssueSSL", payload, "idemp-ssl-"+vhost.ID.String())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to build task"})
		return
	}

	result, err := h.agentClient.SendTask(r.Context(), node.Address, task)
	if err != nil || result.Status != "succeeded" {
		h.auditLog.LogFromContext(r.Context(), "hosting.ssl.issue", "virtual_host", &vhost.ID, audit.ResultFailure, map[string]any{"error": err})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "agent failed to issue ssl"})
		return
	}

	if err := h.hostingStore.EnableSSL(r.Context(), vhost.ID); err != nil {
		h.auditLog.LogFromContext(r.Context(), "hosting.ssl.issue", "virtual_host", &vhost.ID, audit.ResultFailure, map[string]any{"error": "failed to update db"})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ssl issued but db update failed"})
		return
	}

	h.auditLog.LogFromContext(r.Context(), "hosting.ssl.issue", "virtual_host", &vhost.ID, audit.ResultSuccess, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ssl_enabled"})
}
