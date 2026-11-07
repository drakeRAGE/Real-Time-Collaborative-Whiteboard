// Package config is the gateway process configuration.
package config

import (
	"errors"
	"net/url"
	"strings"
)

// Config is loaded from the environment by pkg/config.
type Config struct {
	HTTPAddr      string `env:"HTTP_ADDR" envDefault:":8080"`
	NATSURL       string `env:"NATS_URL,required"`
	NATSCreds     string `env:"NATS_CREDS"`
	RedisAddr     string `env:"REDIS_ADDR,required"`
	RoomGRPCAddr  string `env:"ROOM_GRPC_ADDR" envDefault:"localhost:7001"`
	BoardGRPCAddr string `env:"BOARD_GRPC_ADDR" envDefault:"localhost:7002"`
	ChatGRPCAddr  string `env:"CHAT_GRPC_ADDR" envDefault:"localhost:7003"`
	FrontendURL   string `env:"FRONTEND_URL,required"`
	JWKSURL       string `env:"SUPABASE_JWKS_URL"`
	Issuer        string `env:"SUPABASE_JWT_ISSUER,required"`
	Audience      string `env:"SUPABASE_JWT_AUDIENCE" envDefault:"authenticated"`
	AllowHS256    bool   `env:"SUPABASE_JWT_ALLOW_HS256" envDefault:"false"`
	HMACSecret    string `env:"SUPABASE_JWT_SECRET"`
	LogLevel      string `env:"LOG_LEVEL" envDefault:"info"`
}

// Validate checks the public origin and the JWT mode.
func (c Config) Validate() error {
	if c.HTTPAddr == "" || c.RoomGRPCAddr == "" || c.BoardGRPCAddr == "" || c.ChatGRPCAddr == "" {
		return errors.New("HTTP_ADDR and ROOM/BOARD/CHAT gRPC addresses are required")
	}
	if err := checkOrigin(c.FrontendURL); err != nil {
		return err
	}
	if strings.TrimSpace(c.Issuer) == "" {
		return errors.New("SUPABASE_JWT_ISSUER is required")
	}
	if c.AllowHS256 && strings.TrimSpace(c.HMACSecret) == "" {
		return errors.New("SUPABASE_JWT_SECRET is required when HS256 is enabled")
	}
	if !c.AllowHS256 && strings.TrimSpace(c.JWKSURL) == "" {
		return errors.New("SUPABASE_JWKS_URL is required")
	}
	return nil
}

func checkOrigin(raw string) error {
	if strings.Contains(raw, "*") {
		return errors.New("FRONTEND_URL must not contain a wildcard")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("FRONTEND_URL must be an absolute http or https origin")
	}
	return nil
}
