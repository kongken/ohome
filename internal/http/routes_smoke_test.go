package http

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kongken/ohome/internal/config"
)

func TestRegisterRoutesNoConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	cfg := &config.ServiceConfig{Auth: config.AuthConfig{JWTSecret: "test-secret-for-route-smoke-0123456789"}}
	if err := RegisterRoutes(r, cfg); err != nil {
		t.Fatalf("RegisterRoutes: %v", err)
	}
	count := 0
	for _, ri := range r.Routes() {
		t.Logf("%-6s %s", ri.Method, ri.Path)
		count++
	}
	if count < 25 {
		t.Fatalf("expected >=25 routes, got %d", count)
	}
}
