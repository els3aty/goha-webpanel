package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/els3aty/goha-webpanel/control-plane/internal/agentclient"
	"github.com/els3aty/goha-webpanel/control-plane/internal/auth"
	"github.com/els3aty/goha-webpanel/control-plane/internal/models"
	"github.com/els3aty/goha-webpanel/control-plane/internal/rbac"
)

type ClusterHandler struct {
	clusterStore *models.ClusterStore
	nodeStore    *models.NodeStore
	agentClient  *agentclient.Client
	encryptor    auth.Encryptor
}

func NewClusterHandler(
	clusterStore *models.ClusterStore,
	nodeStore *models.NodeStore,
	agentClient *agentclient.Client,
	encryptor auth.Encryptor,
) *ClusterHandler {
	return &ClusterHandler{
		clusterStore: clusterStore,
		nodeStore:    nodeStore,
		agentClient:  agentClient,
		encryptor:    encryptor,
	}
}

type CreateClusterReq struct {
	Name string `json:"name"`
}

func (h *ClusterHandler) CreateCluster(w http.ResponseWriter, r *http.Request) {
	var req CreateClusterReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	cluster := &models.Cluster{Name: req.Name}
	if err := h.clusterStore.CreateCluster(r.Context(), cluster); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create cluster"})
		return
	}
	writeJSON(w, http.StatusCreated, cluster)
}

type ProvisionLBReq struct {
	ClusterID uuid.UUID `json:"cluster_id"`
	Domain    string    `json:"domain"`
}

// ProvisionLoadBalancer triggers the loadbalancer node in a cluster to update its Nginx upstreams
func (h *ClusterHandler) ProvisionLoadBalancer(w http.ResponseWriter, r *http.Request) {
	// SuperAdmin only for infrastructural changes
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil || p.Role != rbac.RoleSuperAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "access denied"})
		return
	}

	var req ProvisionLBReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	nodes, err := h.nodeStore.GetNodesByCluster(r.Context(), req.ClusterID)
	if err != nil || len(nodes) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no nodes in cluster"})
		return
	}

	var lbNode *models.Node
	var computeIPs []string

	for _, node := range nodes {
		if node.Role == "loadbalancer" {
			lbNode = node
		} else if node.Role == "compute" {
			computeIPs = append(computeIPs, node.Address)
		}
	}

	if lbNode == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cluster has no loadbalancer node"})
		return
	}
	if len(computeIPs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cluster has no compute nodes to balance"})
		return
	}

	keyBytes, err := h.encryptor.Decrypt(lbNode.SigningKeyEncrypted)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to decrypt lb key"})
		return
	}

	builder := agentclient.NewTaskBuilder(lbNode.ID.String(), keyBytes, 5*time.Minute)
	task, _ := builder.Build(uuid.New().String(), "ConfigureLoadBalancer", map[string]any{
		"domain":      req.Domain,
		"compute_ips": computeIPs,
	}, "lb-"+req.Domain)

	result, err := h.agentClient.SendTask(r.Context(), lbNode.Address, task)
	if err != nil || result.Status != "succeeded" {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "lb provisioning failed", "details": result.Output})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "provisioned", "lb_node": lbNode.Name, "backends": "configured"})
}
