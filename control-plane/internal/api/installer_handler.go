package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/els3aty/goha-webpanel/control-plane/internal/agentclient"
	"github.com/els3aty/goha-webpanel/control-plane/internal/audit"
	"github.com/els3aty/goha-webpanel/control-plane/internal/auth"
	"github.com/els3aty/goha-webpanel/control-plane/internal/models"
	"github.com/els3aty/goha-webpanel/control-plane/internal/rbac"
)

type InstallerHandler struct {
	hostingStore *models.HostingStore
	dbStore      *models.DatabaseStore
	nodeStore    *models.NodeStore
	agentClient  *agentclient.Client
	auditLog     *audit.Logger
	encryptor    auth.Encryptor
}

func NewInstallerHandler(
	hostingStore *models.HostingStore,
	dbStore *models.DatabaseStore,
	nodeStore *models.NodeStore,
	agentClient *agentclient.Client,
	auditLog *audit.Logger,
	encryptor auth.Encryptor,
) *InstallerHandler {
	return &InstallerHandler{
		hostingStore: hostingStore,
		dbStore:      dbStore,
		nodeStore:    nodeStore,
		agentClient:  agentClient,
		auditLog:     auditLog,
		encryptor:    encryptor,
	}
}

type InstallAppReq struct {
	VirtualHostID uuid.UUID `json:"virtual_host_id"`
	AppName       string    `json:"app_name"` // e.g. "wordpress"
}

func (h *InstallerHandler) InstallApp(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req InstallAppReq
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
	if p.Role != rbac.RoleAdmin && p.Role != rbac.RoleSuperAdmin && user.CustomerID != p.UserID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "access denied"})
		return
	}

	node, err := h.nodeStore.GetByID(r.Context(), user.NodeID)
	if err != nil || node == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "node not found"})
		return
	}

	keyBytes, err := h.encryptor.Decrypt(node.SigningKeyEncrypted)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error decrypting key"})
		return
	}

	// Step 1: Create Database for the app
	dbName := user.Username + "_wp" + strings.ReplaceAll(uuid.New().String()[:6], "-", "")
	dbUser := user.Username + "_u" + strings.ReplaceAll(uuid.New().String()[:4], "-", "")
	dbPass := uuid.New().String() + uuid.New().String() // Random secure password

	// 1a. DB Creation Task
	builder := agentclient.NewTaskBuilder(node.ID.String(), keyBytes, 5*time.Minute)
	task1, _ := builder.Build(uuid.New().String(), "CreateDatabase", map[string]string{
		"db_name": dbName,
	}, "")
	if res, err := h.agentClient.SendTask(r.Context(), node.Address, task1); err != nil || res.Status != "succeeded" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to provision database"})
		return
	}

	// 1b. DB User Creation Task
	task2, _ := builder.Build(uuid.New().String(), "CreateDatabaseUser", map[string]string{
		"db_user": dbUser,
		"db_pass": dbPass, // Plaintext sent to agent, agent hashes it for mysql natively
	}, "")
	if res, err := h.agentClient.SendTask(r.Context(), node.Address, task2); err != nil || res.Status != "succeeded" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to provision db user"})
		return
	}

	// 1c. DB Grant Task
	task3, _ := builder.Build(uuid.New().String(), "GrantPrivileges", map[string]string{
		"db_name": dbName,
		"db_user": dbUser,
	}, "")
	if res, err := h.agentClient.SendTask(r.Context(), node.Address, task3); err != nil || res.Status != "succeeded" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to grant db privileges"})
		return
	}

	// 1d. Save DB info to Control Plane
	// Assuming you want to track it
	_ = h.dbStore.CreateDatabase(r.Context(), &models.Database{
		HostingUserID: user.ID,
		DBName:        dbName,
	})
	_ = h.dbStore.CreateDatabaseUser(r.Context(), &models.DatabaseUser{
		HostingUserID: user.ID,
		DBUser:        dbUser,
	})

	// Step 2: Install App Files via Agent
	task4, _ := builder.Build(uuid.New().String(), "InstallApp", map[string]string{
		"app_name":      req.AppName,
		"username":      user.Username,
		"document_root": vhost.DocumentRoot,
		"db_name":       dbName,
		"db_user":       dbUser,
		"db_pass":       dbPass,
	}, "")
	
	result, err := h.agentClient.SendTask(r.Context(), node.Address, task4)
	if err != nil || result.Status != "succeeded" {
		h.auditLog.LogFromContext(r.Context(), "app.install", "virtual_host", &vhost.ID, audit.ResultFailure, map[string]any{"error": err})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to install app files"})
		return
	}

	h.auditLog.LogFromContext(r.Context(), "app.install", "virtual_host", &vhost.ID, audit.ResultSuccess, map[string]any{"app": req.AppName})
	writeJSON(w, http.StatusOK, map[string]string{"status": "installed", "db_name": dbName, "db_user": dbUser})
}
