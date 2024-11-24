package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/gliderlabs/ssh"
	"github.com/ksdme/beam/internal/beam"
	"github.com/ksdme/beam/internal/config"
)

func handler(s ssh.Session, engine *beam.Engine) {
	slog.Info("connected")
	defer fmt.Println("disconnect")

	switch strings.TrimSpace(s.RawCommand()) {
	case "send":
		done, err := engine.AddSender("hello", s, s.Stderr())
		if err != nil {
			io.WriteString(s.Stderr(), fmt.Sprintf("Could not connect to the channel: %s", err.Error()))
			return
		}

		io.WriteString(s.Stderr(), "Connected to hello channel\n")
		if exit := <-done; exit != 0 {
			s.Exit(exit)
		}

		return

	case "receive":
		done, err := engine.AddReceiver("hello", s, s.Stderr())
		if err != nil {
			io.WriteString(s.Stderr(), fmt.Sprintf("Could not connect to the channel: %s", err.Error()))
			return
		}

		io.WriteString(s.Stderr(), "Connected to hello channel\n")
		if exit := <-done; exit != 0 {
			s.Exit(exit)
		}

		return

	default:
		io.WriteString(s.Stderr(), "Unknown command")
		return
	}
}

func run() error {
	engine := beam.NewEngine()

	config, err := config.LoadConfigFromEnv()
	if err != nil {
		return errors.Join(fmt.Errorf("could not load configuration"), err)
	}

	slog.Info("starting listening", "port", config.Addr)
	err = ssh.ListenAndServe(config.Addr, func(s ssh.Session) { handler(s, engine) }, ssh.HostKeyFile(config.HostKeyFile))
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
