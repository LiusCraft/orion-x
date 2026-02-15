package httpapi

import (
	"context"
	"net/http"

	"github.com/liuscraft/orion-x/internal/manager/auth"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
)

type AuthMiddleware struct {
	service *auth.Service
}

func NewAuthMiddleware(service *auth.Service) *AuthMiddleware {
	return &AuthMiddleware{service: service}
}

func (m *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.service == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"code":    "ERR_INTERNAL",
				"message": "internal server error",
			})
			return
		}

		token := extractBearerToken(r.Header.Get("Authorization"))
		if token == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"code":    "ERR_UNAUTHORIZED",
				"message": "unauthorized",
			})
			return
		}

		principal, err := m.service.Authenticate(r.Context(), token)
		if err != nil {
			writeAuthServiceError(w, err)
			return
		}

		ctx := withPrincipal(r.Context(), principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *AuthMiddleware) RequireRole(roles ...contracts.UserRole) func(http.Handler) http.Handler {
	allowed := make(map[contracts.UserRole]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFromContext(r.Context())
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]any{
					"code":    "ERR_UNAUTHORIZED",
					"message": "unauthorized",
				})
				return
			}

			if _, exists := allowed[principal.Role]; !exists {
				writeJSON(w, http.StatusForbidden, map[string]any{
					"code":    "ERR_FORBIDDEN",
					"message": "permission denied",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

type principalContextKey struct{}

func withPrincipal(ctx context.Context, principal auth.Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (auth.Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(auth.Principal)
	return principal, ok
}
