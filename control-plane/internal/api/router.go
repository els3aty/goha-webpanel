// Package api — HTTP router.
// All routes and middleware are registered here.
package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/els3aty/goha-webpanel/control-plane/internal/audit"
	"github.com/els3aty/goha-webpanel/control-plane/internal/auth"
	"github.com/els3aty/goha-webpanel/control-plane/internal/config"
	"github.com/els3aty/goha-webpanel/control-plane/internal/middleware"
	"github.com/els3aty/goha-webpanel/control-plane/internal/models"
	"github.com/els3aty/goha-webpanel/control-plane/internal/rbac"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/els3aty/goha-webpanel/control-plane/internal/agentclient"
)

// Dependencies groups all router dependencies.
type Dependencies struct {
	Pool      *pgxpool.Pool
	Sessions  *auth.SessionManager
	TOTP      *auth.TOTPManager
	AuditLog    *audit.Logger
	Config      *config.Config
	Logger      *slog.Logger
	AgentClient *agentclient.Client
	Encryptor   auth.Encryptor
}

// NewRouter builds and returns the application HTTP router.
func NewRouter(deps *Dependencies) http.Handler {
	r := chi.NewRouter()

	// ── Global middleware (applied to all routes) ─────────────────────────────
	r.Use(chiMiddleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger(deps.Logger))
	r.Use(chiMiddleware.Recoverer)

	// Security headers on all responses
	r.Use(securityHeaders)

	// Authentication middleware — sets principal in context if valid session found
	authn := middleware.NewAuthenticator(deps.Sessions, deps.Pool, &deps.Config.Session)
	r.Use(authn.Authenticate)

	// ── Public routes (no auth required) ─────────────────────────────────────
	r.Group(func(r chi.Router) {
		// Rate limit login endpoint strictly
		loginLimiter := middleware.NewRateLimiter(1.0/60, 10, 15*60*1e9) // 10/10min
		r.Use(middleware.LimitByIP(loginLimiter))

		authHandler := NewAuthHandler(
			deps.Pool, deps.Sessions, deps.TOTP, deps.AuditLog, deps.Config,
		)

		r.Post("/api/auth/login", authHandler.Login)
	})

	// ── Authenticated routes ──────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth)

		authHandler := NewAuthHandler(
			deps.Pool, deps.Sessions, deps.TOTP, deps.AuditLog, deps.Config,
		)

		// Auth management
		r.Post("/api/auth/logout", authHandler.Logout)
		r.Post("/api/auth/logout-all", authHandler.LogoutAll)
		r.Get("/api/auth/me", authHandler.Me)

		// TOTP management (any authenticated user)
		totpHandler := NewTOTPHandler(deps.Pool, deps.TOTP, deps.AuditLog, deps.Config)
		r.Post("/api/auth/totp/enroll", totpHandler.BeginEnrollment)
		r.Post("/api/auth/totp/confirm", totpHandler.ConfirmEnrollment)
		r.Post("/api/auth/totp/verify", totpHandler.VerifyMFA)
		r.Delete("/api/auth/totp", totpHandler.DisableMFA)

		dbStore := models.NewDatabaseStore(deps.Pool)
		hostingStore := models.NewHostingStore(deps.Pool)
		nodeStore := models.NewNodeStore(deps.Pool)
		// Packages
		pkgStore := models.NewPackageStore(deps.Pool)
		
		// Hosting management
		hostingHandler := NewHostingHandler(hostingStore, nodeStore, pkgStore, deps.AgentClient, deps.AuditLog, deps.Encryptor)
		r.Post("/api/hosting/users", hostingHandler.CreateHostingUser)
		r.Post("/api/hosting/vhosts", hostingHandler.CreateVirtualHost)
		r.Post("/api/hosting/ssl", hostingHandler.IssueSSL)

		// Database management
		dbHandler := NewDatabaseHandler(dbStore, hostingStore, nodeStore, deps.AgentClient, deps.AuditLog, deps.Encryptor)
		r.Post("/api/hosting/databases", dbHandler.CreateDatabase)
		r.Post("/api/hosting/database-users", dbHandler.CreateDatabaseUser)
		r.Post("/api/hosting/database-grants", dbHandler.GrantPrivileges)

		// DNS management
		dnsStore := models.NewDNSStore(deps.Pool)
		dnsHandler := NewDNSHandler(dnsStore, hostingStore, nodeStore, deps.AgentClient, deps.AuditLog, deps.Encryptor)
		r.Post("/api/hosting/dns/zones", dnsHandler.CreateZone)
		r.Post("/api/hosting/dns/records", dnsHandler.CreateRecord)

		// Backup management
		backupStore := models.NewBackupStore(deps.Pool)
		backupHandler := NewBackupHandler(backupStore, hostingStore, nodeStore, deps.AgentClient, deps.AuditLog, deps.Encryptor)
		r.Post("/api/hosting/backups", backupHandler.RunBackup)
		r.Post("/api/hosting/backups/restore", backupHandler.RunRestore)

		// Node.js Apps management
		nodeAppStore := models.NewNodeAppStore(deps.Pool)
		nodeAppHandler := NewNodeAppHandler(nodeAppStore, hostingStore, nodeStore, deps.AgentClient, deps.AuditLog, deps.Encryptor)
		r.Post("/api/hosting/nodejs", nodeAppHandler.CreateApp)

		// Mail Stack management
		mailStore := models.NewMailStore(deps.Pool)
		mailHandler := NewMailHandler(mailStore, hostingStore, nodeStore, deps.AgentClient, deps.AuditLog, deps.Encryptor)
		r.Post("/api/hosting/mail/mailboxes", mailHandler.CreateMailbox)
		r.Delete("/api/hosting/mail/mailboxes/{id}", mailHandler.DeleteMailbox)
		r.Post("/api/hosting/mail/aliases", mailHandler.CreateAlias)

		// File Manager
		fmHandler := NewFileManagerHandler(hostingStore, nodeStore, deps.AgentClient, deps.AuditLog, deps.Encryptor)
		r.Post("/api/hosting/files/list", fmHandler.List)
		r.Post("/api/hosting/files/read", fmHandler.Read)
		r.Post("/api/hosting/files/write", fmHandler.Write)
		r.Post("/api/hosting/files/delete", fmHandler.Delete)

		// Packages & Resellers
		pkgHandler := NewPackageHandler(pkgStore, deps.AuditLog)
		r.Get("/api/hosting/packages", pkgHandler.ListPackages)
		r.Post("/api/hosting/packages", pkgHandler.CreatePackage)

		// One-Click Installer
		installerHandler := NewInstallerHandler(hostingStore, dbStore, nodeStore, deps.AgentClient, deps.AuditLog, deps.Encryptor)
		r.Post("/api/hosting/apps/install", installerHandler.InstallApp)
	})

	// ── WHMCS Integration Routes ────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		whmcsStore := models.NewWHMCSStore(deps.Pool)
		hostingStore := models.NewHostingStore(deps.Pool)
		nodeStore := models.NewNodeStore(deps.Pool)
		
		r.Use(middleware.RequireWHMCSAuth(whmcsStore))
		r.Use(middleware.RequireWHMCSContext)

		whmcsHandler := NewWHMCSHandler(hostingStore, nodeStore, deps.AgentClient, deps.AuditLog, deps.Encryptor)
		r.Post("/api/whmcs/accounts/create", whmcsHandler.CreateAccount)
		r.Post("/api/whmcs/accounts/suspend", whmcsHandler.SuspendAccount)
		r.Post("/api/whmcs/accounts/unsuspend", whmcsHandler.UnsuspendAccount)
		r.Post("/api/whmcs/accounts/terminate", whmcsHandler.TerminateAccount)
	})

	// ── Admin-only routes ─────────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth)
		r.Use(rbac.Require(rbac.RoleAdmin))

		// Admin user management (Phase 2 stubs — full implementation in Phase 15)
	})

	// ── SuperAdmin-only routes ────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth)
		r.Use(rbac.Require(rbac.RoleSuperAdmin))

		nodeStore := models.NewNodeStore(deps.Pool)
		clusterStore := models.NewClusterStore(deps.Pool)

		r.Get("/api/admin/users", listUsersStub)
		r.Get("/api/admin/nodes", listNodesStub)

		metricsHandler := NewMetricsHandler(nodeStore, deps.AgentClient, deps.Encryptor)
		r.Get("/api/admin/nodes/{id}/metrics", metricsHandler.GetNodeMetrics)

		clusterHandler := NewClusterHandler(clusterStore, nodeStore, deps.AgentClient, deps.Encryptor)
		r.Post("/api/admin/clusters", clusterHandler.CreateCluster)
		r.Post("/api/admin/clusters/provision-lb", clusterHandler.ProvisionLoadBalancer)
	})

	// Health check — public, no auth
	r.Get("/health", healthCheck)
	r.Get("/readyz", readinessCheck(deps.Pool))

	return r
}

// ── Security headers ──────────────────────────────────────────────────────────

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// Prevent clickjacking
		h.Set("X-Frame-Options", "DENY")
		// Prevent MIME sniffing
		h.Set("X-Content-Type-Options", "nosniff")
		// XSS protection (legacy browsers)
		h.Set("X-XSS-Protection", "1; mode=block")
		// Strict Transport Security (HTTPS only)
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		// Referrer policy
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		// CSP — strict; frontend adjusts as needed
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// ── Health checks ─────────────────────────────────────────────────────────────

func healthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func readinessCheck(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "not ready",
				"reason": "database unavailable",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

// ── Stubs for future phases ───────────────────────────────────────────────────

func listUsersStub(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"message": "not implemented — Phase 15"})
}

func listNodesStub(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"message": "not implemented — Phase 3"})
}
