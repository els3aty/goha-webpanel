package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"context"

	"github.com/hosting-panel/control-plane/internal/models"
	"github.com/hosting-panel/control-plane/internal/rbac"
)

type whmcsContextKey string
const whmcsTokenKey whmcsContextKey = "whmcs_token"

// RequireWHMCSAuth validates the Authorization header using WHMCS Scoped Tokens.
func RequireWHMCSAuth(store *models.WHMCSStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			
			// Hash the provided token (SHA-256)
			hash := sha256.Sum256([]byte(tokenStr))
			hashHex := hex.EncodeToString(hash[:])
			
			token, err := store.GetTokenByHash(r.Context(), hashHex)
			if err != nil || token == nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}

			// Validate IP if allowed_ips is populated
			if len(token.AllowedIPs) > 0 {
				clientIP := getRealIP(r)
				allowed := false
				for _, ip := range token.AllowedIPs {
					if ip == clientIP {
						allowed = true
						break
					}
				}
				if !allowed {
					http.Error(w, `{"error":"ip address not allowed"}`, http.StatusForbidden)
					return
				}
			}

			// Update last used asynchronously
			go store.UpdateLastUsed(context.Background(), token.ID)
			
			// Set Principal to indicate WHMCS system
			p := &rbac.Principal{
				UserID: token.ID, 
				Role:   rbac.RoleAdmin, // Grant scoped admin for WHMCS context, handlers must verify it's a WHMCS principal
			}

			ctx := context.WithValue(r.Context(), rbac.PrincipalContextKey, p)
			ctx = context.WithValue(ctx, whmcsTokenKey, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// CheckWHMCSContext ensures the route is only accessed by a WHMCS token, not a standard user JWT.
func RequireWHMCSContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Context().Value(whmcsTokenKey)
		if token == nil {
			http.Error(w, `{"error":"forbidden: whmcs token required"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
