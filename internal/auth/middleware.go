package auth

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/kongken/ohome/internal/httpx"
)

const (
	ctxUserID   = "ohome.user_id"
	ctxUsername = "ohome.username"
)

// RequireAuth verifies a Bearer access token and stores `user_id` /
// `username` in the gin context. Aborts with 401 on missing / invalid /
// expired tokens.
func RequireAuth(issuer *Issuer) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := bearer(c.GetHeader("Authorization"))
		if raw == "" {
			httpx.Abort(c, httpx.New(401, httpx.CodeUnauthorized, "missing bearer token"))
			return
		}
		claims, err := issuer.Parse(raw, AccessToken)
		if err != nil {
			code := httpx.CodeAuthTokenInvalid
			msg := "invalid token"
			if errors.Is(err, jwt.ErrTokenExpired) {
				code = httpx.CodeAuthTokenExpired
				msg = "token expired"
			}
			httpx.Abort(c, httpx.New(401, code, msg))
			return
		}
		c.Set(ctxUserID, claims.Subject)
		c.Set(ctxUsername, claims.Username)
		c.Next()
	}
}

// UserID returns the authenticated user id from context, or "" when
// RequireAuth did not run.
func UserID(c *gin.Context) string {
	v, _ := c.Get(ctxUserID)
	s, _ := v.(string)
	return s
}

// Username returns the authenticated username from context.
func Username(c *gin.Context) string {
	v, _ := c.Get(ctxUsername)
	s, _ := v.(string)
	return s
}

func bearer(h string) string {
	const prefix = "Bearer "
	if len(h) < len(prefix) {
		return ""
	}
	if !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}
