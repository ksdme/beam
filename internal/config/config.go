package config

import (
	"fmt"
	"os"
)

type Config struct {
	Addr        string
	HostKeyFile string
}

func LoadConfigFromEnv() (*Config, error) {
	addr := os.Getenv("BEAM_ADDR")
	if addr == "" {
		addr = ":2222"
	}

	key := os.Getenv("BEAM_HOST_KEY_FILE")
	if key == "" {
		return nil, fmt.Errorf("BEAM_HOST_KEY_FILE unavailable")
	}

	return &Config{
		Addr:        addr,
		HostKeyFile: key,
	}, nil
}
