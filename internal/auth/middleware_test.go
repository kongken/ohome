package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kongken/ohome/internal/config"
)

func TestOptionalAuthAllowsAnonymousRequests(t *testing.T) {
	issuer := testIssuer(t)
	router := gin.New()
	router.GET("/profile", OptionalAuth(issuer), func(c *gin.Context) {
		if got := UserID(c); got != "" {
			t.Fatalf("UserID() = %q, want empty", got)
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestOptionalAuthSetsViewerForBearerToken(t *testing.T) {
	issuer := testIssuer(t)
	token, _, err := issuer.IssueAccess("user_123", "alex")
	if err != nil {
		t.Fatalf("IssueAccess() error = %v", err)
	}

	router := gin.New()
	router.GET("/profile", OptionalAuth(issuer), func(c *gin.Context) {
		if got := UserID(c); got != "user_123" {
			t.Fatalf("UserID() = %q, want %q", got, "user_123")
		}
		if got := Username(c); got != "alex" {
			t.Fatalf("Username() = %q, want %q", got, "alex")
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestOptionalAuthRejectsInvalidBearerToken(t *testing.T) {
	issuer := testIssuer(t)
	router := gin.New()
	router.GET("/profile", OptionalAuth(issuer), func(c *gin.Context) {
		t.Fatal("handler should not run")
	})

	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	req.Header.Set("Authorization", "Bearer invalid")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func testIssuer(t *testing.T) *Issuer {
	t.Helper()
	issuer, err := NewIssuer(config.AuthConfig{
		JWTSecret:      "01234567890123456789012345678901",
		AccessTokenTTL: "1h",
	})
	if err != nil {
		t.Fatalf("NewIssuer() error = %v", err)
	}
	return issuer
}
