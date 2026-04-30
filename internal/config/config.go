package config

import "log/slog"

// ServiceConfig holds ohome-specific config. All storage backends
// (postgres / redis / mongo / s3) are loaded by butterfly core from the
// `store.*` block of the same yaml file and accessed via
// `butterfly.orx.me/core/store/{sqldb,redis,mongo,s3}`.
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
