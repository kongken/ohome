package config

import "log/slog"

// ServiceConfig holds ohome-specific config. Storage backends (redis, s3,
// mongo, db) are loaded by butterfly core from the same yaml file via
// `store.*` and accessed through `butterfly.orx.me/core/store/{redis,s3,...}`.
type ServiceConfig struct {
	Environment string     `yaml:"environment"`
	HTTPPort    int        `yaml:"http_port"`
	Auth        AuthConfig `yaml:"auth"`
}

type AuthConfig struct {
	JWTSecret       string `yaml:"jwt_secret"`
	AccessTokenTTL  string `yaml:"access_token_ttl"`
	RefreshTokenTTL string `yaml:"refresh_token_ttl"`
}

func (c *ServiceConfig) Print() {
	slog.Info("service config loaded",
		"environment", c.Environment,
		"http_port", c.HTTPPort,
	)
}
