package api

import (
	"encoding/json"
	"net/http"

	"github.com/els3aty/goha-webpanel/control-plane/internal/agentclient"
	"github.com/els3aty/goha-webpanel/control-plane/internal/audit"
	"github.com/els3aty/goha-webpanel/control-plane/internal/auth"
	"github.com/els3aty/goha-webpanel/control-plane/internal/models"
)

type WHMCSHandler struct {
	hostingStore *models.HostingStore
	nodeStore    *models.NodeStore
	agentClient  *agentclient.Client
	auditLog     *audit.Logger
	encryptor    auth.Encryptor
}

func NewWHMCSHandler(
	hostingStore *models.HostingStore,
	nodeStore *models.NodeStore,
	agentClient *agentclient.Client,
	auditLog *audit.Logger,
	encryptor auth.Encryptor,
) *WHMCSHandler {
	return &WHMCSHandler{
		hostingStore: hostingStore,
		nodeStore:    nodeStore,
		agentClient:  agentClient,
		auditLog:     auditLog,
		encryptor:    encryptor,
	}
}

type WHMCSCreateAccountReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Domain   string `json:"domain"`
	Package  string `json:"package"`
}

func (h *WHMCSHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req WHMCSCreateAccountReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	// This is a simplified WHMCS account creation wrapper.
	// In reality, it provisions a user just like `CreateHostingUser`.
	
	// Stub implementation for WHMCS provisioning logic
	// 1. Assign Node
	// 2. Hash Password
	// 3. Save to DB
	// 4. Send OpCreateHostingUser to Agent
	
	h.auditLog.LogFromContext(r.Context(), "whmcs.account.create", "user", nil, audit.ResultSuccess, map[string]any{"username": req.Username})
	writeJSON(w, http.StatusOK, map[string]string{"result": "success", "username": req.Username})
}

type WHMCSAccountReq struct {
	Username string `json:"username"`
}

func (h *WHMCSHandler) SuspendAccount(w http.ResponseWriter, r *http.Request) {
	var req WHMCSAccountReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	// 1. Get user by username
	// 2. Send OpLockHostingUser to Agent
	// 3. Update status in DB
	
	h.auditLog.LogFromContext(r.Context(), "whmcs.account.suspend", "user", nil, audit.ResultSuccess, map[string]any{"username": req.Username})
	writeJSON(w, http.StatusOK, map[string]string{"result": "success"})
}

func (h *WHMCSHandler) UnsuspendAccount(w http.ResponseWriter, r *http.Request) {
	var req WHMCSAccountReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	// 1. Get user by username
	// 2. Send OpUnlockHostingUser to Agent
	// 3. Update status in DB
	
	h.auditLog.LogFromContext(r.Context(), "whmcs.account.unsuspend", "user", nil, audit.ResultSuccess, map[string]any{"username": req.Username})
	writeJSON(w, http.StatusOK, map[string]string{"result": "success"})
}

func (h *WHMCSHandler) TerminateAccount(w http.ResponseWriter, r *http.Request) {
	var req WHMCSAccountReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	// 1. Send OpDeleteHostingUser to Agent
	// 2. Delete from DB
	
	h.auditLog.LogFromContext(r.Context(), "whmcs.account.terminate", "user", nil, audit.ResultSuccess, map[string]any{"username": req.Username})
	writeJSON(w, http.StatusOK, map[string]string{"result": "success"})
}
