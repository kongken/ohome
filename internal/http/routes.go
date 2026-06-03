package http

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kongken/ohome/internal/auth"
	"github.com/kongken/ohome/internal/config"
	"github.com/kongken/ohome/internal/connections"
	"github.com/kongken/ohome/internal/users"
)

// RegisterRoutes wires HTTP handlers onto the gin engine. Domain handlers
// are mounted under `/api/v1`. See api.md for the full route catalog.
// Returns an error if critical subsystems (auth) fail to initialise.
func RegisterRoutes(r *gin.Engine, cfg *config.ServiceConfig) error {
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
		return fmt.Errorf("auth issuer init: %w", err)
	}
	auth.NewHandler(issuer).Register(v1.Group("/auth"))

	usersGroup := v1.Group("/users")
	users.NewHandler(issuer).Register(usersGroup)
	connections.NewHandler(issuer).RegisterOnUsers(usersGroup)

	// Future domain handlers (posts, media, ...) registered here.
	return nil
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
