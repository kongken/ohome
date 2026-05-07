package main

import (
	"context"
	"log/slog"
	"time"

	"butterfly.orx.me/core/app"
	"github.com/gin-gonic/gin"

	"github.com/kongken/ohome/internal/config"
	"github.com/kongken/ohome/internal/dao"
	apihttp "github.com/kongken/ohome/internal/http"
)

func main() {
	svcConfig := &config.ServiceConfig{}

	appConfig := &app.Config{
		Namespace: "auto",
		Service:   "ohome",
		Config:    svcConfig,
		Router: func(r *gin.Engine) {
			if err := apihttp.RegisterRoutes(r, svcConfig); err != nil {
				slog.Error("route registration failed", "error", err)
				panic(err)
			}
		},
		InitFunc: []func() error{
			initDAO,
			runMigrations,
		},
	}

	app.New(appConfig).Run()
}

func initDAO() error {
	if err := dao.Init(); err != nil {
		slog.Error("dao init failed", "error", err)
		return err
	}
	slog.Info("dao initialized via butterfly sqldb")
	return nil
}

func runMigrations() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := dao.Client().Schema.Create(ctx); err != nil {
		slog.Error("ent schema migrate failed", "error", err)
		return err
	}
	return nil
}
