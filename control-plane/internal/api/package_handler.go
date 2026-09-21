package api

import (
	"encoding/json"
	"net/http"

	"github.com/els3aty/goha-webpanel/control-plane/internal/audit"
	"github.com/els3aty/goha-webpanel/control-plane/internal/models"
	"github.com/els3aty/goha-webpanel/control-plane/internal/rbac"
)

type PackageHandler struct {
	pkgStore *models.PackageStore
	auditLog *audit.Logger
}

func NewPackageHandler(pkgStore *models.PackageStore, auditLog *audit.Logger) *PackageHandler {
	return &PackageHandler{
		pkgStore: pkgStore,
		auditLog: auditLog,
	}
}

func (h *PackageHandler) ListPackages(w http.ResponseWriter, r *http.Request) {
	// Resellers and Admins can list packages to assign them to users
	pkgs, err := h.pkgStore.ListPackages(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list packages"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"packages": pkgs})
}

type CreatePackageReq struct {
	Name         string `json:"name"`
	DiskQuotaMB  int    `json:"disk_quota_mb"`
	MaxDomains   int    `json:"max_domains"`
	MaxDatabases int    `json:"max_databases"`
	MaxMailboxes int    `json:"max_mailboxes"`
}

func (h *PackageHandler) CreatePackage(w http.ResponseWriter, r *http.Request) {
	// Only SuperAdmins and Admins can create global packages
	p := rbac.PrincipalFromContext(r.Context())
	if p == nil || (p.Role != rbac.RoleAdmin && p.Role != rbac.RoleSuperAdmin) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "access denied"})
		return
	}

	var req CreatePackageReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	pkg := &models.HostingPackage{
		Name:         req.Name,
		DiskQuotaMB:  req.DiskQuotaMB,
		MaxDomains:   req.MaxDomains,
		MaxDatabases: req.MaxDatabases,
		MaxMailboxes: req.MaxMailboxes,
	}

	if err := h.pkgStore.CreatePackage(r.Context(), pkg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create package"})
		return
	}

	h.auditLog.LogFromContext(r.Context(), "package.create", "package", &pkg.ID, audit.ResultSuccess, map[string]any{"name": pkg.Name})
	writeJSON(w, http.StatusOK, pkg)
}
