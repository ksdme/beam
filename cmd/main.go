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
	slog.Debug("connected")
	defer slog.Debug("disconnected")

	switch strings.TrimSpace(s.RawCommand()) {
	case "send":
		channel, err := engine.AddSender("hello", s, s.Stderr())
		if err != nil {
			err = errors.Join(fmt.Errorf("could not connect to channel"), err)
			io.WriteString(s.Stderr(), fmt.Sprintln(err.Error()))
			return
		}
		io.WriteString(s.Stderr(), "connected to hello channel\n")

		select {
		case <-s.Context().Done():
			channel.Quit <- s.Context().Err()

		case <-channel.Sender.Done:
		}

	case "receive":
		channel, err := engine.AddReceiver("hello", s, s.Stderr())
		if err != nil {
			err = errors.Join(fmt.Errorf("could not connect to channel"), err)
			io.WriteString(s.Stderr(), fmt.Sprintln(err.Error()))
			return
		}
		io.WriteString(s.Stderr(), "connected to hello channel\n")

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
	slog.SetLogLoggerLevel(slog.LevelDebug)

	err := run()
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
