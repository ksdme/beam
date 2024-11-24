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

var a = beam.NewBeam()

func handler(s ssh.Session) {
	slog.Info("connected")

	switch strings.TrimSpace(s.RawCommand()) {
	case "send":
		done, err := a.AddSender("hello", s, s.Stderr())
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
		done, err := a.AddReceiver("hello", s, s.Stderr())
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
