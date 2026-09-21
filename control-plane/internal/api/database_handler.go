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

type DatabaseHandler struct {
	dbStore      *models.DatabaseStore
	hostingStore *models.HostingStore
	nodeStore    *models.NodeStore
	agentClient  *agentclient.Client
	auditLog     *audit.Logger
	encryptor    auth.Encryptor
}

func NewDatabaseHandler(
	dbStore *models.DatabaseStore,
	hostingStore *models.HostingStore,
	nodeStore *models.NodeStore,
	agentClient *agentclient.Client,
	auditLog *audit.Logger,
	encryptor auth.Encryptor,
) *DatabaseHandler {
	return &DatabaseHandler{
		dbStore:      dbStore,
		hostingStore: hostingStore,
		nodeStore:    nodeStore,
		agentClient:  agentClient,
		auditLog:     auditLog,
		encryptor:    encryptor,
	}
}

// ── Requests ──────────────────────────────────────────────────────────────────

type CreateDatabaseRequest struct {
	HostingUserID uuid.UUID `json:"hosting_user_id"`
	DBName        string    `json:"db_name"`
}

type CreateDatabaseUserRequest struct {
	HostingUserID uuid.UUID `json:"hosting_user_id"`
	DBUsername    string    `json:"db_username"`
	Password      string    `json:"password"`
}

type GrantPrivilegesRequest struct {
	DatabaseID     uuid.UUID `json:"database_id"`
	DatabaseUserID uuid.UUID `json:"database_user_id"`
	Privileges     string    `json:"privileges"`
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func (h *DatabaseHandler) CreateDatabase(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req CreateDatabaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	// Find the hosting user and verify ownership
	user, err := h.hostingStore.GetHostingUser(r.Context(), req.HostingUserID)
	if err != nil || user == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "hosting user not found"})
		return
	}
	if p.Role != rbac.RoleAdmin && p.Role != rbac.RoleSuperAdmin && user.CustomerID != p.UserID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "access denied"})
		return
	}

	// Create record
	db := &models.Database{
		HostingUserID: user.ID,
		DBName:        req.DBName,
		Status:        models.DatabaseStatusActive,
	}
	if err := h.dbStore.CreateDatabase(r.Context(), db); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create db record"})
		return
	}

	// Send to Agent
	node, _ := h.nodeStore.GetByID(r.Context(), user.NodeID)
	keyBytes, _ := h.encryptor.Decrypt(node.SigningKeyEncrypted)
	builder := agentclient.NewTaskBuilder(node.ID.String(), keyBytes, 5*time.Minute)
	
	payload := map[string]string{
		"db_name": req.DBName,
	}
	task, _ := builder.Build(uuid.New().String(), "CreateDatabase", payload, "idemp-db-"+db.ID.String())
	
	result, err := h.agentClient.SendTask(r.Context(), node.Address, task)
	if err != nil || result.Status != "succeeded" {
		h.auditLog.LogFromContext(r.Context(), "database.create", "database", &db.ID, audit.ResultFailure, map[string]any{"error": err})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "agent failed to create database"})
		return
	}

	h.auditLog.LogFromContext(r.Context(), "database.create", "database", &db.ID, audit.ResultSuccess, nil)
	writeJSON(w, http.StatusOK, db)
}

func (h *DatabaseHandler) CreateDatabaseUser(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req CreateDatabaseUserRequest
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

	dbUser := &models.DatabaseUser{
		HostingUserID: user.ID,
		DBUsername:    req.DBUsername,
		Status:        models.DatabaseStatusActive,
	}
	if err := h.dbStore.CreateDatabaseUser(r.Context(), dbUser); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create user record"})
		return
	}

	node, _ := h.nodeStore.GetByID(r.Context(), user.NodeID)
	keyBytes, _ := h.encryptor.Decrypt(node.SigningKeyEncrypted)
	builder := agentclient.NewTaskBuilder(node.ID.String(), keyBytes, 5*time.Minute)
	
	payload := map[string]string{
		"db_username": req.DBUsername,
		"password":    req.Password,
	}
	task, _ := builder.Build(uuid.New().String(), "CreateDatabaseUser", payload, "idemp-dbuser-"+dbUser.ID.String())
	
	result, err := h.agentClient.SendTask(r.Context(), node.Address, task)
	if err != nil || result.Status != "succeeded" {
		h.auditLog.LogFromContext(r.Context(), "database.user.create", "database_user", &dbUser.ID, audit.ResultFailure, map[string]any{"error": err})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "agent failed to create database user"})
		return
	}

	h.auditLog.LogFromContext(r.Context(), "database.user.create", "database_user", &dbUser.ID, audit.ResultSuccess, nil)
	
	// Blank out password before returning
	req.Password = ""
	writeJSON(w, http.StatusOK, dbUser)
}

func (h *DatabaseHandler) GrantPrivileges(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req GrantPrivilegesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	db, err := h.dbStore.GetDatabase(r.Context(), req.DatabaseID)
	if err != nil || db == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "database not found"})
		return
	}
	
	dbUser, err := h.dbStore.GetDatabaseUser(r.Context(), req.DatabaseUserID)
	if err != nil || dbUser == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "database user not found"})
		return
	}

	// Must belong to same hosting account
	if db.HostingUserID != dbUser.HostingUserID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "mismatched hosting accounts"})
		return
	}

	hostUser, _ := h.hostingStore.GetHostingUser(r.Context(), db.HostingUserID)
	if p.Role != rbac.RoleAdmin && p.Role != rbac.RoleSuperAdmin && hostUser.CustomerID != p.UserID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "access denied"})
		return
	}

	if req.Privileges == "" {
		req.Privileges = "ALL PRIVILEGES"
	}

	if err := h.dbStore.GrantPrivileges(r.Context(), db.ID, dbUser.ID, req.Privileges); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record grant"})
		return
	}

	node, _ := h.nodeStore.GetByID(r.Context(), hostUser.NodeID)
	keyBytes, _ := h.encryptor.Decrypt(node.SigningKeyEncrypted)
	builder := agentclient.NewTaskBuilder(node.ID.String(), keyBytes, 5*time.Minute)
	
	payload := map[string]string{
		"db_name":     db.DBName,
		"db_username": dbUser.DBUsername,
		"privileges":  req.Privileges,
	}
	task, _ := builder.Build(uuid.New().String(), "GrantDatabasePrivileges", payload, "idemp-grant-"+db.ID.String()+"-"+dbUser.ID.String())
	
	result, err := h.agentClient.SendTask(r.Context(), node.Address, task)
	if err != nil || result.Status != "succeeded" {
		h.auditLog.LogFromContext(r.Context(), "database.grant", "database", &db.ID, audit.ResultFailure, map[string]any{"error": err})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "agent failed to grant privileges"})
		return
	}

	h.auditLog.LogFromContext(r.Context(), "database.grant", "database", &db.ID, audit.ResultSuccess, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "granted"})
}
