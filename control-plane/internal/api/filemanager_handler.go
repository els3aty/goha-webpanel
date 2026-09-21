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

type FileManagerHandler struct {
	hostingStore *models.HostingStore
	nodeStore    *models.NodeStore
	agentClient  *agentclient.Client
	auditLog     *audit.Logger
	encryptor    auth.Encryptor
}

func NewFileManagerHandler(
	hostingStore *models.HostingStore,
	nodeStore *models.NodeStore,
	agentClient *agentclient.Client,
	auditLog *audit.Logger,
	encryptor auth.Encryptor,
) *FileManagerHandler {
	return &FileManagerHandler{
		hostingStore: hostingStore,
		nodeStore:    nodeStore,
		agentClient:  agentClient,
		auditLog:     auditLog,
		encryptor:    encryptor,
	}
}

type FileReq struct {
	HostingUserID uuid.UUID `json:"hosting_user_id"`
	Path          string    `json:"path"`
	Content       string    `json:"content,omitempty"` // For Write
}

func (h *FileManagerHandler) dispatchToAgent(w http.ResponseWriter, r *http.Request, op string, req FileReq) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
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

	node, _ := h.nodeStore.GetByID(r.Context(), user.NodeID)
	keyBytes, _ := h.encryptor.Decrypt(node.SigningKeyEncrypted)
	builder := agentclient.NewTaskBuilder(node.ID.String(), keyBytes, 1*time.Minute)

	payload := map[string]any{
		"username": user.Username,
		"path":     req.Path,
	}
	if req.Content != "" {
		payload["content"] = req.Content
	}

	task, _ := builder.Build(uuid.New().String(), op, payload, "") // No idempotency key needed for FM ops usually

	result, err := h.agentClient.SendTask(r.Context(), node.Address, task)
	if err != nil || result.Status != "succeeded" {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "agent failed", "details": result.Output})
		return
	}

	// Assuming result.Output contains the JSON response from agent (e.g. file content or list)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if result.Output != nil {
		json.NewEncoder(w).Encode(result.Output)
	} else {
		w.Write([]byte(`{"status":"success"}`))
	}
}

func (h *FileManagerHandler) List(w http.ResponseWriter, r *http.Request) {
	var req FileReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	h.dispatchToAgent(w, r, "FileList", req)
}

func (h *FileManagerHandler) Read(w http.ResponseWriter, r *http.Request) {
	var req FileReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	h.dispatchToAgent(w, r, "FileRead", req)
}

func (h *FileManagerHandler) Write(w http.ResponseWriter, r *http.Request) {
	var req FileReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	h.auditLog.LogFromContext(r.Context(), "filemanager.write", "file", nil, audit.ResultSuccess, map[string]any{"path": req.Path})
	h.dispatchToAgent(w, r, "FileWrite", req)
}

func (h *FileManagerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	var req FileReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	h.auditLog.LogFromContext(r.Context(), "filemanager.delete", "file", nil, audit.ResultSuccess, map[string]any{"path": req.Path})
	h.dispatchToAgent(w, r, "FileDelete", req)
}
