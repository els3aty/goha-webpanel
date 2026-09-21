package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hosting-panel/control-plane/internal/agentclient"
	"github.com/hosting-panel/control-plane/internal/auth"
	"github.com/hosting-panel/control-plane/internal/models"
	"github.com/hosting-panel/control-plane/internal/rbac"
)

type MetricsHandler struct {
	nodeStore   *models.NodeStore
	agentClient *agentclient.Client
	encryptor   auth.Encryptor
}

func NewMetricsHandler(
	nodeStore *models.NodeStore,
	agentClient *agentclient.Client,
	encryptor auth.Encryptor,
) *MetricsHandler {
	return &MetricsHandler{
		nodeStore:   nodeStore,
		agentClient: agentClient,
		encryptor:   encryptor,
	}
}

func (h *MetricsHandler) GetNodeMetrics(w http.ResponseWriter, r *http.Request) {
	// Only SuperAdmin can view node metrics
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil || p.Role != rbac.RoleSuperAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "access denied"})
		return
	}

	nodeIDStr := chi.URLParam(r, "id")
	nodeID, err := uuid.Parse(nodeIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid node id"})
		return
	}

	node, err := h.nodeStore.GetByID(r.Context(), nodeID)
	if err != nil || node == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "node not found"})
		return
	}

	keyBytes, err := h.encryptor.Decrypt(node.SigningKeyEncrypted)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error decrypting key"})
		return
	}

	// Dispatch task to agent
	builder := agentclient.NewTaskBuilder(node.ID.String(), keyBytes, 30*time.Second) // metrics should be fast
	task, err := builder.Build(uuid.New().String(), "GetMetrics", nil, "")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to build task"})
		return
	}

	result, err := h.agentClient.SendTask(r.Context(), node.Address, task)
	if err != nil || result.Status != "succeeded" {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "agent failed", "details": result.Output})
		return
	}

	// Stream agent response directly to user
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if result.Output != nil {
		json.NewEncoder(w).Encode(result.Output)
	} else {
		w.Write([]byte(`{"status":"success"}`))
	}
}
