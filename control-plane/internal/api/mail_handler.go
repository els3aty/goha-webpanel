package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/els3aty/goha-webpanel/control-plane/internal/agentclient"
	"github.com/els3aty/goha-webpanel/control-plane/internal/audit"
	"github.com/els3aty/goha-webpanel/control-plane/internal/auth"
	"github.com/els3aty/goha-webpanel/control-plane/internal/models"
	"github.com/els3aty/goha-webpanel/control-plane/internal/rbac"
)

type MailHandler struct {
	mailStore    *models.MailStore
	hostingStore *models.HostingStore
	nodeStore    *models.NodeStore
	agentClient  *agentclient.Client
	auditLog     *audit.Logger
	encryptor    auth.Encryptor
}

func NewMailHandler(
	mailStore *models.MailStore,
	hostingStore *models.HostingStore,
	nodeStore *models.NodeStore,
	agentClient *agentclient.Client,
	auditLog *audit.Logger,
	encryptor auth.Encryptor,
) *MailHandler {
	return &MailHandler{
		mailStore:    mailStore,
		hostingStore: hostingStore,
		nodeStore:    nodeStore,
		agentClient:  agentClient,
		auditLog:     auditLog,
		encryptor:    encryptor,
	}
}

type CreateMailboxReq struct {
	HostingUserID uuid.UUID `json:"hosting_user_id"`
	Domain        string    `json:"domain"`
	Address       string    `json:"address"`
	Password      string    `json:"password"`
	QuotaMB       int       `json:"quota_mb"`
}

func (h *MailHandler) CreateMailbox(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req CreateMailboxReq
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

	// Generate Dovecot-compatible Bcrypt hash: {BLF-CRYPT}$2a$...
	// This prevents the Agent from needing to run `doveadm pw` or any shell commands.
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
		return
	}
	dovecotHash := fmt.Sprintf("{BLF-CRYPT}%s", string(hashBytes))

	mb := &models.Mailbox{
		HostingUserID: user.ID,
		Domain:        req.Domain,
		Address:       req.Address,
		PasswordHash:  dovecotHash,
		QuotaMB:       req.QuotaMB,
		Status:        models.MailboxStatusActive,
	}

	if err := h.mailStore.CreateMailbox(r.Context(), mb); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create mailbox record"})
		return
	}

	node, _ := h.nodeStore.GetByID(r.Context(), user.NodeID)
	keyBytes, _ := h.encryptor.Decrypt(node.SigningKeyEncrypted)
	builder := agentclient.NewTaskBuilder(node.ID.String(), keyBytes, 5*time.Minute)

	payload := map[string]any{
		"address":       mb.Address,
		"password_hash": mb.PasswordHash,
		"quota_mb":      mb.QuotaMB,
	}
	task, _ := builder.Build(uuid.New().String(), "CreateMailbox", payload, "idemp-mail-"+mb.ID.String())

	result, err := h.agentClient.SendTask(r.Context(), node.Address, task)
	if err != nil || result.Status != "succeeded" {
		h.auditLog.LogFromContext(r.Context(), "mailbox.create", "mailbox", &mb.ID, audit.ResultFailure, map[string]any{"error": err})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "agent failed to provision mailbox"})
		return
	}

	h.auditLog.LogFromContext(r.Context(), "mailbox.create", "mailbox", &mb.ID, audit.ResultSuccess, nil)
	// Do not return password hash in response
	mb.PasswordHash = ""
	writeJSON(w, http.StatusOK, mb)
}

func (h *MailHandler) DeleteMailbox(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid mailbox id"})
		return
	}

	mb, err := h.mailStore.GetMailbox(r.Context(), id)
	if err != nil || mb == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "mailbox not found"})
		return
	}

	user, _ := h.hostingStore.GetHostingUser(r.Context(), mb.HostingUserID)
	if p.Role != rbac.RoleAdmin && p.Role != rbac.RoleSuperAdmin && user.CustomerID != p.UserID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "access denied"})
		return
	}

	node, _ := h.nodeStore.GetByID(r.Context(), user.NodeID)
	keyBytes, _ := h.encryptor.Decrypt(node.SigningKeyEncrypted)
	builder := agentclient.NewTaskBuilder(node.ID.String(), keyBytes, 5*time.Minute)

	payload := map[string]any{"address": mb.Address}
	task, _ := builder.Build(uuid.New().String(), "DeleteMailbox", payload, "idemp-mail-del-"+mb.ID.String())

	result, err := h.agentClient.SendTask(r.Context(), node.Address, task)
	if err != nil || result.Status != "succeeded" {
		h.auditLog.LogFromContext(r.Context(), "mailbox.delete", "mailbox", &mb.ID, audit.ResultFailure, map[string]any{"error": err})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "agent failed to delete mailbox"})
		return
	}

	h.mailStore.DeleteMailbox(r.Context(), id)
	h.auditLog.LogFromContext(r.Context(), "mailbox.delete", "mailbox", &mb.ID, audit.ResultSuccess, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type CreateAliasReq struct {
	HostingUserID uuid.UUID `json:"hosting_user_id"`
	Domain        string    `json:"domain"`
	Source        string    `json:"source"`
	Destination   string    `json:"destination"`
}

func (h *MailHandler) CreateAlias(w http.ResponseWriter, r *http.Request) {
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req CreateAliasReq
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

	alias := &models.MailAlias{
		HostingUserID: user.ID,
		Domain:        req.Domain,
		Source:        req.Source,
		Destination:   req.Destination,
	}

	if err := h.mailStore.CreateAlias(r.Context(), alias); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create alias record"})
		return
	}

	node, _ := h.nodeStore.GetByID(r.Context(), user.NodeID)
	keyBytes, _ := h.encryptor.Decrypt(node.SigningKeyEncrypted)
	builder := agentclient.NewTaskBuilder(node.ID.String(), keyBytes, 5*time.Minute)

	payload := map[string]any{
		"source":      alias.Source,
		"destination": alias.Destination,
	}
	task, _ := builder.Build(uuid.New().String(), "CreateAlias", payload, "idemp-alias-"+alias.ID.String())

	result, err := h.agentClient.SendTask(r.Context(), node.Address, task)
	if err != nil || result.Status != "succeeded" {
		h.auditLog.LogFromContext(r.Context(), "alias.create", "mailalias", &alias.ID, audit.ResultFailure, map[string]any{"error": err})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "agent failed to create alias"})
		return
	}

	h.auditLog.LogFromContext(r.Context(), "alias.create", "mailalias", &alias.ID, audit.ResultSuccess, nil)
	writeJSON(w, http.StatusOK, alias)
}
