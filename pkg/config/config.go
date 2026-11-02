// Package config loads process configuration from environment variables.
package config

import (
	"fmt"
	"os"

	"github.com/caarlos0/env/v11"
)

// validator is implemented by config structs that need checks beyond parsing.
type validator interface {
	Validate() error
}

// Load parses environment variables into T and runs Validate when T implements it.
// Missing required variables are named in the returned error.
func Load[T any]() (T, error) {
	var cfg T
	if err := env.Parse(&cfg); err != nil {
		return cfg, fmt.Errorf("config: %w", err)
	}
	if err := runValidate(&cfg); err != nil {
		return cfg, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}

// MustLoad is Load for service main. On failure it writes the error, including
// missing variable names, to stderr and exits.
func MustLoad[T any]() T {
	cfg, err := Load[T]()
	if err != nil {
		if _, werr := fmt.Fprintf(os.Stderr, "config: %v\n", err); werr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}
	return cfg
}

func runValidate(cfg any) error {
	v, ok := cfg.(validator)
	if !ok {
		return nil
	}
	return v.Validate()
}
