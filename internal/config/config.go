package config

import "log/slog"

type ServiceConfig struct {
	Environment string      `yaml:"environment"`
	HTTPPort    int         `yaml:"http_port"`
	Auth        AuthConfig  `yaml:"auth"`
	Store       StoreConfig `yaml:"store"`
}

type AuthConfig struct {
	JWTSecret       string `yaml:"jwt_secret"`
	AccessTokenTTL  string `yaml:"access_token_ttl"`
	RefreshTokenTTL string `yaml:"refresh_token_ttl"`
}

type StoreConfig struct {
	Redis map[string]RedisConfig `yaml:"redis"`
	S3    map[string]S3Config    `yaml:"s3"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type S3Config struct {
	Endpoint        string `yaml:"endpoint"`
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
	Region          string `yaml:"region"`
	Bucket          string `yaml:"bucket"`
	UseSSL          bool   `yaml:"use_ssl"`
}

func (c *ServiceConfig) Print() {
	slog.Info("service config loaded",
		"environment", c.Environment,
		"http_port", c.HTTPPort,
	)
}
