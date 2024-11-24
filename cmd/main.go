package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/gliderlabs/ssh"
	"github.com/ksdme/beam/internal/config"
)

func handler(s ssh.Session) {
	slog.Info("connected")
	io.WriteString(s, "Hello World\n")
	slog.Info("connection ended")
}

func run() error {
	config, err := config.LoadConfigFromEnv()
	if err != nil {
		return errors.Join(fmt.Errorf("could not load configuration"), err)
	}

	slog.Info("starting listening", "port", config.Addr)
	err = ssh.ListenAndServe(config.Addr, handler, ssh.HostKeyFile(config.HostKeyFile))
	if err != nil {
		return errors.Join(fmt.Errorf("could not serve"), err)
	}

	return nil
}

func main() {
	err := run()
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
