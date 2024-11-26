package config

import (
	"fmt"
	"os"

	"github.com/alexflint/go-arg"
)

type Config struct {
	Secret             string `arg:"-"`
	BindAddr           string `arg:"--bind-addr" default:"127.0.0.1:2222"`
	HostKeyFile        string `arg:"--host-key-file,required"`
	AuthorizedKeysFile string `arg:"--authorized-keys-file" help:"if set, only connections from these keys will be accepted"`
	Host               string `arg:"--host" default:"beam.ssh.camp" help:"public host name for this service"`
	MaxTimeout         uint   `arg:"--max-timeout" default:"86400" help:"max connection lifetime in seconds, 0 for no limit"`
	IdleTimeout        uint   `arg:"--idle-timeout" default:"43200" help:"idle connection timeout in seconds, 0 for no limit"`
}

func LoadConfig() (*Config, error) {
	var config Config
	arg.MustParse(&config)

	secret := os.Getenv("BEAM_SECRET")
	if secret == "" {
		return nil, fmt.Errorf("BEAM_SECRET environment variable missing")
	}
	config.Secret = secret

	return &config, nil
}
