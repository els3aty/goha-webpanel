package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/els3aty/goha-webpanel/control-plane/internal/agentclient"
	"github.com/els3aty/goha-webpanel/control-plane/internal/audit"
	"github.com/els3aty/goha-webpanel/control-plane/internal/auth"
	"github.com/els3aty/goha-webpanel/control-plane/internal/models"
	"github.com/els3aty/goha-webpanel/control-plane/internal/rbac"
)

type BackupHandler struct {
	backupStore  *models.BackupStore
	hostingStore *models.HostingStore
	nodeStore    *models.NodeStore
	agentClient  *agentclient.Client
	auditLog     *audit.Logger
	encryptor    auth.Encryptor
}

func NewBackupHandler(
	backupStore *models.BackupStore,
	hostingStore *models.HostingStore,
	nodeStore *models.NodeStore,
	agentClient *agentclient.Client,
	auditLog *audit.Logger,
	encryptor auth.Encryptor,
) *BackupHandler {
	return &BackupHandler{
		backupStore:  backupStore,
		hostingStore: hostingStore,
		nodeStore:    nodeStore,
		agentClient:  agentClient,
		auditLog:     auditLog,
		encryptor:    encryptor,
	}
}

type RunBackupRequest struct {
	HostingUserID uuid.UUID `json:"hosting_user_id"`
	Type          string    `json:"type"` // "files" or "database"
	TargetDB      string    `json:"target_db,omitempty"`
}

type RunRestoreRequest struct {
	BackupID uuid.UUID `json:"backup_id"`
	TargetDB string    `json:"target_db,omitempty"` // Required if type is database
}

func (h *BackupHandler) RunBackup(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req RunBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if req.Type != string(models.BackupTypeFiles) && req.Type != string(models.BackupTypeDatabase) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid backup type"})
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

	// Backup file path formulation
	timestamp := time.Now().Format("20060102_150405")
	ext := ".tar.gz"
	if req.Type == string(models.BackupTypeDatabase) {
		ext = ".sql.gz"
	}
	fileName := fmt.Sprintf("backup_%s_%s%s", req.Type, timestamp, ext)
	filePath := fmt.Sprintf("/home/%s/backups/%s", user.Username, fileName)

	backup := &models.Backup{
		HostingUserID: user.ID,
		BackupType:    models.BackupType(req.Type),
		FilePath:      filePath,
		Status:        models.BackupStatusRunning,
	}

	if err := h.backupStore.CreateBackup(r.Context(), backup); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create backup record"})
		return
	}

	node, _ := h.nodeStore.GetByID(r.Context(), user.NodeID)
	keyBytes, _ := h.encryptor.Decrypt(node.SigningKeyEncrypted)
	builder := agentclient.NewTaskBuilder(node.ID.String(), keyBytes, 30*time.Minute)
	
	payload := map[string]string{
		"username":    user.Username,
		"backup_type": req.Type,
		"file_path":   filePath,
		"target_db":   req.TargetDB,
	}
	task, _ := builder.Build(uuid.New().String(), "RunBackup", payload, "idemp-backup-"+backup.ID.String())
	
	result, err := h.agentClient.SendTask(r.Context(), node.Address, task)
	if err != nil || result.Status != "succeeded" {
		h.backupStore.UpdateBackupStatus(r.Context(), backup.ID, models.BackupStatusFailed, 0)
		h.auditLog.LogFromContext(r.Context(), "backup.create", "backup", &backup.ID, audit.ResultFailure, map[string]any{"error": err})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "agent failed to run backup"})
		return
	}

	// Assume agent returns size_bytes in Data
	var sizeBytes float64
	if dataMap, ok := result.Data.(map[string]any); ok {
		if s, ok := dataMap["size_bytes"].(float64); ok {
			sizeBytes = s
		}
	}

	h.backupStore.UpdateBackupStatus(r.Context(), backup.ID, models.BackupStatusCompleted, int64(sizeBytes))
	h.auditLog.LogFromContext(r.Context(), "backup.create", "backup", &backup.ID, audit.ResultSuccess, nil)
	writeJSON(w, http.StatusOK, backup)
}

func (h *BackupHandler) RunRestore(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req RunRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	backup, err := h.backupStore.GetBackup(r.Context(), req.BackupID)
	if err != nil || backup == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "backup not found"})
		return
	}

	if backup.Status != models.BackupStatusCompleted {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "backup is not completed"})
		return
	}

	user, _ := h.hostingStore.GetHostingUser(r.Context(), backup.HostingUserID)
	if p.Role != rbac.RoleAdmin && p.Role != rbac.RoleSuperAdmin && user.CustomerID != p.UserID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "access denied"})
		return
	}

	node, _ := h.nodeStore.GetByID(r.Context(), user.NodeID)
	keyBytes, _ := h.encryptor.Decrypt(node.SigningKeyEncrypted)
	builder := agentclient.NewTaskBuilder(node.ID.String(), keyBytes, 30*time.Minute)
	
	payload := map[string]string{
		"username":    user.Username,
		"backup_type": string(backup.BackupType),
		"file_path":   backup.FilePath,
		"target_db":   req.TargetDB,
	}
	task, _ := builder.Build(uuid.New().String(), "RunRestore", payload, "")
	
	result, err := h.agentClient.SendTask(r.Context(), node.Address, task)
	if err != nil || result.Status != "succeeded" {
		h.auditLog.LogFromContext(r.Context(), "backup.restore", "backup", &backup.ID, audit.ResultFailure, map[string]any{"error": err})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "agent failed to run restore"})
		return
	}

	h.auditLog.LogFromContext(r.Context(), "backup.restore", "backup", &backup.ID, audit.ResultSuccess, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "restored"})
}
