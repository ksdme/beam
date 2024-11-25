package main

import (
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
	name := "hello"

	switch strings.TrimSpace(s.RawCommand()) {
	case "send":
		slog.Debug("sender connected", "channel", name)
		defer slog.Debug("sender disconnected", "channel", name)

		channel, err := engine.AddSender(name, s, s.Stderr())
		if err != nil {
			err = fmt.Errorf("could not connect to channel: %w", err)
			io.WriteString(s.Stderr(), fmt.Sprintln(err.Error()))
			return
		}
		io.WriteString(s.Stderr(), fmt.Sprintf("connected to %s channel as sender\n", name))

		// Block until the beamer is done or the connection is aborted.
		select {
		case <-s.Context().Done():
			channel.Quit <- s.Context().Err()

		case <-channel.Sender.Done:
		}

	case "receive":
		slog.Debug("receiver connected", "channel", name)
		defer slog.Debug("receiver disconnected", "channel", name)

		channel, err := engine.AddReceiver(name, s, s.Stderr())
		if err != nil {
			err = fmt.Errorf("could not connect to channel: %w", err)
			io.WriteString(s.Stderr(), fmt.Sprintln(err.Error()))
			return
		}
		io.WriteString(s.Stderr(), fmt.Sprintf("connected to %s channel\n", name))

		// Block until the beamer is done or the connection is aborted.
		select {
		case <-s.Context().Done():
			channel.Quit <- s.Context().Err()

		case <-channel.Receiver.Done:
		}

	default:
		io.WriteString(s.Stderr(), "unknown command")
		return
	}
}

func run() error {
	engine := beam.NewEngine()

	config, err := config.LoadConfigFromEnv()
	if err != nil {
		return fmt.Errorf("could not load configuration: %w", err)
	}

	slog.Info("starting listening", "port", config.Addr)
	err = ssh.ListenAndServe(config.Addr, func(s ssh.Session) { handler(s, engine) }, ssh.HostKeyFile(config.HostKeyFile))
	if err != nil {
		return fmt.Errorf("could not start server: %w", err)
	}

	return nil
}

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	err := run()
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
