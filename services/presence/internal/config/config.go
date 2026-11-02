// Package config is the presence-service process configuration.
package config

import "errors"

// Config is loaded from the environment by pkg/config.
type Config struct {
	NATSURL  string `env:"NATS_URL,required"`
	RedisAddr string `env:"REDIS_ADDR,required"`
	HTTPAddr string `env:"HTTP_ADDR" envDefault:":8004"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
}

// Validate checks addresses after env parsing.
func (c Config) Validate() error {
	if c.HTTPAddr == "" {
		return errors.New("HTTP_ADDR is required")
	}
	return nil
}
