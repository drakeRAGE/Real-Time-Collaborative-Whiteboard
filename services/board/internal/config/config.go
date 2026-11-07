// Package config is the board-service process configuration.
package config

import "errors"

// Config is loaded from the environment by pkg/config.
type Config struct {
	DatabaseURL  string `env:"DATABASE_URL,required"`
	NATSURL      string `env:"NATS_URL,required"`
	NATSCreds    string `env:"NATS_CREDS"`
	RedisAddr    string `env:"REDIS_ADDR,required"`
	RoomGRPCAddr string `env:"ROOM_GRPC_ADDR,required"`
	GRPCAddr     string `env:"GRPC_ADDR" envDefault:":7002"`
	HTTPAddr     string `env:"HTTP_ADDR" envDefault:":8002"`
	LogLevel     string `env:"LOG_LEVEL" envDefault:"info"`
}

// Validate checks addresses after env parsing.
func (c Config) Validate() error {
	if c.GRPCAddr == "" || c.HTTPAddr == "" || c.RoomGRPCAddr == "" {
		return errors.New("GRPC_ADDR, HTTP_ADDR, and ROOM_GRPC_ADDR are required")
	}
	return nil
}
