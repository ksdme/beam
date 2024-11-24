package config

import (
	"fmt"
	"os"
)

type Config struct {
	Secret      string
	Addr        string
	HostKeyFile string
}

func LoadConfigFromEnv() (*Config, error) {
	secret := os.Getenv("BEAM_SECRET")
	if secret == "" {
		return nil, fmt.Errorf("BEAM_SECRET missing")
	}

	addr := os.Getenv("BEAM_ADDR")
	if addr == "" {
		addr = ":2222"
	}

	key := os.Getenv("BEAM_HOST_KEY_FILE")
	if key == "" {
		return nil, fmt.Errorf("BEAM_HOST_KEY_FILE missing")
	}

	return &Config{
		Secret:      secret,
		Addr:        addr,
		HostKeyFile: key,
	}, nil
}
