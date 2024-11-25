package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/alexflint/go-arg"
	"github.com/gliderlabs/ssh"
	"github.com/ksdme/beam/internal/beam"
	"github.com/ksdme/beam/internal/config"
)

// - Use Public Key based channel names
// - Support custom channel names
// - Add a progress meter.
// - Check what happens when the connection is interrupted during a transfer.
// - Configurable buffer size (min, max)
func handler(s ssh.Session, engine *beam.Engine) {
	// Calling s.Exit does not seem to cancel the context, so, we need to manually
	// store that intent and return early.
	exited := false
	exit := func(i int) {
		s.Exit(i)
	}

	// Parse the command passed.
	type role struct {
		Channel string `arg:"positional"`
	}
	var args struct {
		Quiet   bool
		Send    *role `arg:"subcommand:send"`
		Receive *role `arg:"subcommand:receive"`
	}

	parser, err := arg.NewParser(arg.Config{
		IgnoreEnv:     true,
		IgnoreDefault: true,
		Program:       "beam",
		Out:           s.Stderr(),
		Exit:          exit,
	}, &args)
	if err != nil {
		slog.Error("could not initialize arg parser", "err", err)
		io.WriteString(s.Stderr(), fmt.Sprintln("internal error"))
		return
	}

	parser.MustParse(s.Command())
	if exited {
		return
	}
	if parser.Subcommand() == nil {
		parser.Fail("missing subcommand")
	}
	if exited {
		return
	}

	// Handle commands.
	name := "hello"
	switch {
	case args.Send != nil:
		slog.Debug("sender connected", "channel", name)
		defer slog.Debug("sender disconnected", "channel", name)

		channel, err := engine.AddSender(name, s, s.Stderr())
		if err != nil {
			err = fmt.Errorf("could not connect to channel: %w", err)
			io.WriteString(s.Stderr(), fmt.Sprintln(err.Error()))
			return
		}
		if !args.Quiet {
			io.WriteString(s.Stderr(), fmt.Sprintf("<• connected to %s as sender\n", name))

			if channel.Receiver == nil {
				io.WriteString(
					s.Stderr(),
					"To receive this beam run: ssh beam.ssh.camp receive hello\n"+
						"You can pipe the output of that command or redirect it to a file to save it.\n\n",
				)
			}
		}

		// Block until beamer is done or the connection is aborted.
		select {
		case <-s.Context().Done():
			channel.Quit <- s.Context().Err()

		case err := <-channel.Sender.Done:
			if err != nil && !args.Quiet {
				io.WriteString(s.Stderr(), fmt.Sprintln(err.Error()))
			}
		}

	case args.Receive != nil:
		slog.Debug("receiver connected", "channel", name)
		defer slog.Debug("receiver disconnected", "channel", name)

		channel, err := engine.AddReceiver(name, s, s.Stderr())
		if err != nil {
			err = fmt.Errorf("could not connect to channel: %w", err)
			io.WriteString(s.Stderr(), fmt.Sprintln(err.Error()))
			return
		}
		if !args.Quiet {
			io.WriteString(s.Stderr(), fmt.Sprintf("•> connected to %s as receiver\n", name))

			if channel.Sender == nil {
				io.WriteString(
					s.Stderr(),
					"To send data on this beam pipe it into: ssh beam.ssh.camp send hello\n\n",
				)
			}
		}

		// Block until beamer is done or the connection is aborted.
		select {
		case <-s.Context().Done():
			channel.Quit <- s.Context().Err()

		case err := <-channel.Receiver.Done:
			if err != nil && !args.Quiet {
				io.WriteString(s.Stderr(), fmt.Sprintln(err.Error()))
			}
		}
	}
}

func run() error {
	engine := beam.NewEngine()

	config, err := config.LoadConfigFromEnv()
	if err != nil {
		return fmt.Errorf("could not load configuration: %w", err)
	}

	slog.Info("starting listening", "port", config.Addr)
	err = ssh.ListenAndServe(
		config.Addr,
		func(s ssh.Session) { handler(s, engine) },
		ssh.HostKeyFile(config.HostKeyFile),
		ssh.PasswordAuth(func(ctx ssh.Context, password string) bool { return false }),
		ssh.PublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool { return true }),
	)
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
