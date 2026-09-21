package api

import (
	"encoding/json"
	"net/http"
	"time"
	"fmt"
	"math/rand"

	"github.com/google/uuid"
	"github.com/els3aty/goha-webpanel/control-plane/internal/agentclient"
	"github.com/els3aty/goha-webpanel/control-plane/internal/audit"
	"github.com/els3aty/goha-webpanel/control-plane/internal/auth"
	"github.com/els3aty/goha-webpanel/control-plane/internal/models"
	"github.com/els3aty/goha-webpanel/control-plane/internal/rbac"
)

type NodeAppHandler struct {
	appStore     *models.NodeAppStore
	hostingStore *models.HostingStore
	nodeStore    *models.NodeStore // Assuming this is actually referring to Server Nodes
	agentClient  *agentclient.Client
	auditLog     *audit.Logger
	encryptor    auth.Encryptor
}

func NewNodeAppHandler(
	appStore *models.NodeAppStore,
	hostingStore *models.HostingStore,
	nodeStore *models.NodeStore, // Represents server nodes
	agentClient *agentclient.Client,
	auditLog *audit.Logger,
	encryptor auth.Encryptor,
) *NodeAppHandler {
	return &NodeAppHandler{
		appStore:     appStore,
		hostingStore: hostingStore,
		nodeStore:    nodeStore,
		agentClient:  agentClient,
		auditLog:     auditLog,
		encryptor:    encryptor,
	}
}

type CreateNodeAppRequest struct {
	HostingUserID uuid.UUID `json:"hosting_user_id"`
	AppName       string    `json:"app_name"`
	Domain        string    `json:"domain"`
	AppPath       string    `json:"app_path"` // relative to user home
	StartupFile   string    `json:"startup_file"` // relative to app_path
	NodeVersion   string    `json:"node_version"`
}

func (h *NodeAppHandler) CreateApp(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req CreateNodeAppRequest
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

	// Generate internal port between 3000-4000
	port := rand.Intn(1000) + 3000

	app := &models.NodeApp{
		HostingUserID: user.ID,
		AppName:       req.AppName,
		Domain:        req.Domain,
		AppPath:       fmt.Sprintf("/home/%s/%s", user.Username, req.AppPath),
		StartupFile:   req.StartupFile,
		NodeVersion:   req.NodeVersion,
		Port:          port,
		Status:        models.NodeAppStatusRunning,
	}

	if err := h.appStore.CreateApp(r.Context(), app); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create node app record"})
		return
	}

	node, _ := h.nodeStore.GetByID(r.Context(), user.NodeID)
	keyBytes, _ := h.encryptor.Decrypt(node.SigningKeyEncrypted)
	builder := agentclient.NewTaskBuilder(node.ID.String(), keyBytes, 5*time.Minute)
	
	payload := map[string]any{
		"username":     user.Username,
		"app_name":     app.AppName,
		"domain":       app.Domain,
		"app_path":     app.AppPath,
		"startup_file": app.StartupFile,
		"node_version": app.NodeVersion,
		"port":         app.Port,
	}
	task, _ := builder.Build(uuid.New().String(), "CreateNodeApp", payload, "idemp-node-"+app.ID.String())
	
	result, err := h.agentClient.SendTask(r.Context(), node.Address, task)
	if err != nil || result.Status != "succeeded" {
		h.appStore.UpdateAppStatus(r.Context(), app.ID, models.NodeAppStatusFailed)
		h.auditLog.LogFromContext(r.Context(), "nodeapp.create", "nodeapp", &app.ID, audit.ResultFailure, map[string]any{"error": err})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "agent failed to create node app"})
		return
	}

	h.auditLog.LogFromContext(r.Context(), "nodeapp.create", "nodeapp", &app.ID, audit.ResultSuccess, nil)
	writeJSON(w, http.StatusOK, app)
}
