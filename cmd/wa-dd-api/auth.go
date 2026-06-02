package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

type adminRole string

const (
	adminRoleViewer   adminRole = "viewer"
	adminRoleReviewer adminRole = "reviewer"
	adminRoleAdmin    adminRole = "admin"
)

var adminRoleRank = map[adminRole]int{
	adminRoleViewer:   1,
	adminRoleReviewer: 2,
	adminRoleAdmin:    3,
}

type adminAuthConfig struct {
	Issuer   string
	Audience string
	JWKSURL  string
}

type adminUser struct {
	ID    string
	Email string
	Name  string
	Role  adminRole
}

type adminUserContextKey struct{}

type clerkClaims struct {
	Email       string `json:"email"`
	Name        string `json:"name"`
	Role        string `json:"role"`
	AdminRole   string `json:"admin_role"`
	WADDRole    string `json:"wadd_role"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	PrimaryMail string `json:"primary_email_address"`
	jwt.RegisteredClaims
}

func adminAuthConfigFromEnv() adminAuthConfig {
	issuer := strings.TrimRight(env("CLERK_JWT_ISSUER", env("NEXT_PUBLIC_CLERK_FRONTEND_API_URL", "")), "/")
	if issuer == "" {
		if domain := env("CLERK_DOMAIN", ""); domain != "" {
			issuer = "https://" + strings.Trim(domain, "/")
		}
	}
	jwksURL := env("CLERK_JWKS_URL", "")
	if jwksURL == "" && issuer != "" {
		jwksURL = issuer + "/.well-known/jwks.json"
	}
	return adminAuthConfig{
		Issuer:   issuer,
		Audience: env("WADD_ADMIN_JWT_AUDIENCE", "wa-dd-admin"),
		JWKSURL:  jwksURL,
	}
}

func newAdminAuthenticator(ctx context.Context, cfg adminAuthConfig) (func(http.Handler) http.Handler, error) {
	if cfg.Issuer == "" || cfg.JWKSURL == "" {
		return nil, fmt.Errorf("CLERK_JWT_ISSUER or CLERK_DOMAIN is required for admin auth")
	}
	jwks, err := keyfunc.NewDefaultCtx(ctx, []string{cfg.JWKSURL})
	if err != nil {
		return nil, fmt.Errorf("load Clerk JWKS: %w", err)
	}
	return adminAuthMiddleware(cfg, jwks.Keyfunc), nil
}

func localDevAdminBypass() func(http.Handler) http.Handler {
	stub := adminUser{ID: "local-dev", Email: "local-dev@wa-dd.local", Name: "Local Dev", Role: adminRoleAdmin}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := context.WithValue(req.Context(), adminUserContextKey{}, stub)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	}
}

func adminAuthMiddleware(cfg adminAuthConfig, keys jwt.Keyfunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			user, err := validateAdminBearer(req.Header.Get("X-WADD-Admin-Auth"), cfg, keys)
			if err != nil {
				log.Printf("admin auth rejected path=%s reason=%v", req.URL.Path, err)
				w.Header().Set("WWW-Authenticate", `Bearer realm="wa-dd-admin"`)
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "admin authentication required"})
				return
			}
			ctx := context.WithValue(req.Context(), adminUserContextKey{}, user)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	}
}

func requireAdminRole(minimum adminRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			user, ok := adminUserFromContext(req.Context())
			if !ok || !adminRoleAtLeast(user.Role, minimum) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

func adminUserFromContext(ctx context.Context) (adminUser, bool) {
	user, ok := ctx.Value(adminUserContextKey{}).(adminUser)
	return user, ok
}

func adminRoleAtLeast(role, minimum adminRole) bool {
	return adminRoleRank[role] >= adminRoleRank[minimum]
}

func validateAdminBearer(header string, cfg adminAuthConfig, keys jwt.Keyfunc) (adminUser, error) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return adminUser{}, fmt.Errorf("missing bearer")
	}
	tokenString := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if tokenString == "" {
		return adminUser{}, fmt.Errorf("empty bearer")
	}
	claims := &clerkClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, keys,
		jwt.WithIssuer(cfg.Issuer),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithValidMethods([]string{"RS256"}),
	)
	if err != nil {
		return adminUser{}, err
	}
	if !token.Valid {
		return adminUser{}, fmt.Errorf("invalid token")
	}
	if cfg.Audience != "" && !slices.Contains(claims.Audience, cfg.Audience) {
		return adminUser{}, fmt.Errorf("invalid audience")
	}
	role := parseAdminRole(firstNonEmpty(claims.Role, claims.AdminRole, claims.WADDRole))
	if role == "" {
		return adminUser{}, fmt.Errorf("missing admin role")
	}
	email := firstNonEmpty(claims.Email, claims.PrimaryMail)
	if email == "" {
		email = "unknown"
	}
	name := claims.Name
	if name == "" {
		name = strings.TrimSpace(claims.FirstName + " " + claims.LastName)
	}
	return adminUser{ID: claims.Subject, Email: email, Name: name, Role: role}, nil
}

func parseAdminRole(v string) adminRole {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "viewer":
		return adminRoleViewer
	case "reviewer":
		return adminRoleReviewer
	case "admin":
		return adminRoleAdmin
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func adminMutationAudit(store *db.Store, req *http.Request, action, targetType, targetID string, previousState, newState any, notes string) {
	user, ok := adminUserFromContext(req.Context())
	if !ok {
		return
	}
	previousJSON, _ := json.Marshal(previousState)
	newJSON, _ := json.Marshal(newState)
	ip := strings.TrimSpace(req.Header.Get("X-Forwarded-For"))
	if i := strings.Index(ip, ","); i >= 0 {
		ip = strings.TrimSpace(ip[:i])
	}
	if ip == "" {
		ip = strings.TrimSpace(req.RemoteAddr)
		if host, _, found := strings.Cut(ip, ":"); found {
			ip = host
		}
	}
	if _, err := store.InsertAdminAuditLog(req.Context(), db.AdminAuditLogParams{
		ActorUserID:   user.ID,
		ActorEmail:    user.Email,
		ActorName:     user.Name,
		ActorRole:     string(user.Role),
		Route:         req.URL.Path,
		Action:        action,
		TargetType:    targetType,
		TargetID:      targetID,
		PreviousState: previousJSON,
		NewState:      newJSON,
		ReviewerNotes: notes,
		RequestID:     middleware.GetReqID(req.Context()),
		IPAddress:     ip,
		UserAgent:     req.UserAgent(),
	}); err != nil {
		log.Printf("admin audit log failed path=%s action=%s target=%s/%s err=%v", req.URL.Path, action, targetType, targetID, err)
	}
}

func internalAPIAuthMiddleware(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if token == "" {
				log.Printf("api auth rejected path=%s reason=missing_config", req.URL.Path)
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "api authentication is not configured"})
				return
			}
			if !validBearerToken(req.Header.Get("Authorization"), token) {
				log.Printf("api auth rejected path=%s reason=invalid_token", req.URL.Path)
				w.Header().Set("WWW-Authenticate", `Bearer realm="wa-dd-api"`)
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

func validBearerToken(header, token string) bool {
	if token == "" {
		return false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}
