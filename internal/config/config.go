package config

import (
	"fmt"
	"os"
)

type Config struct {
	Secret      string
	BindAddr    string
	HostKeyFile string
	Host        string
}

func LoadConfigFromEnv() (*Config, error) {
	secret := os.Getenv("BEAM_SECRET")
	if secret == "" {
		return nil, fmt.Errorf("BEAM_SECRET missing")
	}

	addr := os.Getenv("BEAM_BIND_ADDR")
	if addr == "" {
		addr = ":2222"
	}

	key := os.Getenv("BEAM_HOST_KEY_FILE")
	if key == "" {
		return nil, fmt.Errorf("BEAM_HOST_KEY_FILE missing")
	}

	host := os.Getenv("BEAM_HOST")
	if host == "" {
		host = "beam.ssh.camp"
	}

	return &Config{
		Secret:      secret,
		BindAddr:    addr,
		HostKeyFile: key,
		Host:        host,
	}, nil
}
