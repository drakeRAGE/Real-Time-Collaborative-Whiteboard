// Package config is the room-service process configuration.
package config

import "errors"

// Config is loaded from the environment by pkg/config.
type Config struct {
	DatabaseURL string `env:"DATABASE_URL,required"`
	NATSURL     string `env:"NATS_URL,required"`
	NATSCreds   string `env:"NATS_CREDS"`
	GRPCAddr    string `env:"GRPC_ADDR" envDefault:":7001"`
	HTTPAddr    string `env:"HTTP_ADDR" envDefault:":8001"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
}

// Validate checks addresses after env parsing.
func (c Config) Validate() error {
	if c.GRPCAddr == "" || c.HTTPAddr == "" {
		return errors.New("GRPC_ADDR and HTTP_ADDR are required")
	}
	return nil
}
