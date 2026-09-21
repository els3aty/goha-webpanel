// Package rbac implements Role-Based Access Control.
// IMPORTANT: Authorization is enforced on the BACKEND only.
// Frontend checks are UI hints — never security controls.
//
// Every protected endpoint must call rbac.Require() or rbac.RequireOwnership().
package rbac

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

// Role represents a user role in the system.
type Role string

const (
	RoleSuperAdmin     Role = "super_admin"
	RoleAdmin          Role = "admin"
	RoleReseller       Role = "reseller"
	RoleCustomer       Role = "customer"
	RoleAPIIntegration Role = "api_integration"
)

// roleHierarchy defines the numeric level of each role.
// Higher level = more privilege. Used for "at least" comparisons.
var roleHierarchy = map[Role]int{
	RoleCustomer:       1,
	RoleAPIIntegration: 1, // same level as customer but different permissions
	RoleReseller:       2,
	RoleAdmin:          3,
	RoleSuperAdmin:     4,
}

// Principal carries the authenticated identity for a request.
// It is attached to the request context by the auth middleware.
type Principal struct {
	UserID      uuid.UUID
	Email       string
	Role        Role
	SessionID   uuid.UUID
	MFAVerified bool
	// ResellerID is set for Reseller principals — their own reseller ID
	ResellerID *uuid.UUID
	// TokenScopes is set for API token principals — allowed operation scopes
	TokenScopes []string
	// IsAPIToken indicates this principal authenticated via API token (not session)
	IsAPIToken bool
}

// contextKey is the type for context keys to avoid collisions.
type contextKey int

const principalKey contextKey = iota

// WithPrincipal stores the principal in the request context.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// PrincipalFromContext retrieves the principal from the request context.
// Returns nil if no principal is set (unauthenticated request).
func PrincipalFromContext(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey).(*Principal)
	return p
}

// HasRole returns true if the principal has at least the specified role level.
// Note: APIIntegration has level 1 but different permissions than Customer.
func (p *Principal) HasRole(minimum Role) bool {
	myLevel := roleHierarchy[p.Role]
	minLevel := roleHierarchy[minimum]
	return myLevel >= minLevel
}

// IsAtLeast returns true if the principal's role level is >= the given role.
func (p *Principal) IsAtLeast(role Role) bool {
	return roleHierarchy[p.Role] >= roleHierarchy[role]
}

// HasScope returns true if an API token principal has the required scope.
// For session-based principals, scopes are not used.
func (p *Principal) HasScope(scope string) bool {
	if !p.IsAPIToken {
		return true // session-based principals use role checks, not scopes
	}
	for _, s := range p.TokenScopes {
		if s == scope {
			return true
		}
	}
	return false
}

// ── Middleware ────────────────────────────────────────────────────────────────

// Require returns an HTTP middleware that enforces the minimum role requirement.
// Requests without a valid principal or insufficient role are rejected with 403.
func Require(minimum Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := PrincipalFromContext(r.Context())
			if p == nil {
				http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
				return
			}
			if !p.IsAtLeast(minimum) {
				http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireScope returns an HTTP middleware that enforces an API token scope.
// For session-based principals, this always passes (role check is sufficient).
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := PrincipalFromContext(r.Context())
			if p == nil {
				http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
				return
			}
			if !p.HasScope(scope) {
				http.Error(w, `{"error":"insufficient token scope"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireMFA returns an HTTP middleware that requires MFA to be verified.
// Used for high-privilege operations even if the user is authenticated.
func RequireMFA() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := PrincipalFromContext(r.Context())
			if p == nil {
				http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
				return
			}
			if !p.MFAVerified {
				http.Error(w, `{"error":"MFA verification required"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ── Ownership Checks ──────────────────────────────────────────────────────────

// ErrOwnershipDenied is returned when a principal tries to access a resource they don't own.
var ErrOwnershipDenied = fmt.Errorf("access denied: resource not owned by requesting principal")

// OwnershipChecker provides server-side resource ownership verification.
// NEVER trust client-supplied owner_id — always verify in DB.
type OwnershipChecker struct {
	// GetResourceOwner returns the UUID of the owner of the resource.
	// Implementations query the database directly.
	GetResourceOwner func(ctx context.Context, resourceType string, resourceID uuid.UUID) (ownerID uuid.UUID, err error)
}

// CheckOwnership verifies that the principal owns the given resource.
// SuperAdmin and Admin can access any resource.
// Resellers can access resources in their customer tree.
// Customers can only access their own resources.
//
// CRITICAL: This function queries the database — it does NOT trust any
// client-supplied owner ID or resource data.
func (oc *OwnershipChecker) CheckOwnership(
	ctx context.Context,
	p *Principal,
	resourceType string,
	resourceID uuid.UUID,
) error {
	// SuperAdmin and Admin can access anything
	if p.Role == RoleSuperAdmin || p.Role == RoleAdmin {
		return nil
	}

	ownerID, err := oc.GetResourceOwner(ctx, resourceType, resourceID)
	if err != nil {
		return ErrOwnershipDenied
	}

	// Direct ownership
	if ownerID == p.UserID {
		return nil
	}

	// Reseller: check if resource belongs to one of their customers
	if p.Role == RoleReseller {
		// This would query whether ownerID is a customer under this reseller
		// Implementation in Phase 15 (Reseller module)
		// For now, return denied — conservative default
		return ErrOwnershipDenied
	}

	return ErrOwnershipDenied
}

// ── Role Validation ───────────────────────────────────────────────────────────

// IsValidRole returns true if the role string is a recognized role.
func IsValidRole(role string) bool {
	switch Role(role) {
	case RoleSuperAdmin, RoleAdmin, RoleReseller, RoleCustomer, RoleAPIIntegration:
		return true
	}
	return false
}

// CanGrantRole returns true if the granting principal can assign the given role.
// A principal can never grant a role higher than their own.
func CanGrantRole(granter *Principal, targetRole Role) bool {
	return roleHierarchy[granter.Role] > roleHierarchy[targetRole]
}
