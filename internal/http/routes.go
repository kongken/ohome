package http

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kongken/ohome/internal/auth"
	"github.com/kongken/ohome/internal/config"
)

// RegisterRoutes wires HTTP handlers onto the gin engine. Domain handlers
// are mounted under `/api/v1`. See api.md for the full route catalog.
func RegisterRoutes(r *gin.Engine, cfg *config.ServiceConfig) {
	r.Use(corsMiddleware())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})
	r.GET("/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	v1 := r.Group("/api/v1")
	v1.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	issuer, err := auth.NewIssuer(cfg.Auth)
	if err != nil {
		// Fatal at boot — without an issuer no auth-protected route can work.
		// Log loudly; fall through so /health still returns OK for probes.
		slog.Error("auth issuer init failed", "error", err)
	} else {
		auth.NewHandler(issuer).Register(v1.Group("/auth"))
	}

	// Future domain handlers (users, posts, ...) registered here.
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept, Accept-Encoding, Authorization, X-Requested-With")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
