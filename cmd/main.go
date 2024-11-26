package main

import (
	"crypto/sha1"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/btcsuite/btcutil/base58"
	"github.com/gliderlabs/ssh"
	"github.com/ksdme/beam/internal/beam"
	"github.com/ksdme/beam/internal/config"
)

// - Add a progress meter.
// - Check what happens when the connection is interrupted during a transfer.
// - Authorized Keys
func handler(config *config.Config, engine *beam.Engine, s ssh.Session) {
	// Calling s.Exit does not seem to cancel the context, so, we need to manually
	// store that intent and return early if parsing arguments fail.
	exited := false
	exit := func(i int) {
		s.Exit(i)
	}

	// Parse the command passed.
	type send struct {
		BufferSize int    `arg:"--buffer-size,-b" default:"64" help:"buffer size in kB (between 1 and 64)"`
		Channel    string `arg:"positional"`
	}
	type receive struct {
		Channel string `arg:"positional"`
	}
	var args struct {
		Quiet   bool
		Send    *send    `arg:"subcommand:send"`
		Receive *receive `arg:"subcommand:receive"`
	}

	parser, err := arg.NewParser(arg.Config{
		IgnoreEnv:     true,
		IgnoreDefault: false,
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
	if args.Send != nil {
		if args.Send.BufferSize < 1 {
			parser.FailSubcommand("buffer size needs to be between 1 and 64", "send")
		}
		if args.Send.BufferSize > 64 {
			parser.FailSubcommand("buffer size needs to be between 1 and 64", "send")
		}
		fmt.Println(args.Send.BufferSize)
	}
	if exited {
		return
	}

	switch {
	case args.Send != nil:
		name, err := makeChannel(config, args.Send.Channel, s.PublicKey())
		if err != nil {
			slog.Debug("could not determine channel name", "err", err)
			err = fmt.Errorf("could not connect to channel: %w", err)
			io.WriteString(s.Stderr(), fmt.Sprintln(err.Error()))
			return
		}

		slog.Debug("sender connected", "channel", name)
		defer slog.Debug("sender disconnected", "channel", name)

		channel, err := engine.AddSender(name, s, args.Send.BufferSize, nil)
		if err != nil {
			err = fmt.Errorf("could not connect to channel: %w", err)
			io.WriteString(s.Stderr(), fmt.Sprintln(err.Error()))
			return
		}
		if !args.Quiet {
			io.WriteString(s.Stderr(), fmt.Sprintf("<- connected to %s as sender\n\n", name))

			if channel.Receiver == nil {
				io.WriteString(
					s.Stderr(),
					fmt.Sprintf(
						"To receive this beam run: ssh %s receive %s\n"+
							"You can pipe the output of that command or redirect it to a file to save it.\n\n",
						config.Host,
						name,
					),
				)
			}
		}

		// Block until beamer is done or the connection is aborted.
		select {
		case <-s.Context().Done():
			channel.Quit <- s.Context().Err()

		case err := <-channel.Sender.Done:
			if !args.Quiet {
				if err == nil {
					io.WriteString(s.Stderr(), "beaming up complete\n")
				} else {
					io.WriteString(s.Stderr(), fmt.Sprintln(err.Error()))
				}
			}
		}

	case args.Receive != nil:
		name, err := makeChannel(config, args.Receive.Channel, s.PublicKey())
		if err != nil {
			slog.Debug("could not determine channel name", "err", err)
			err = fmt.Errorf("could not connect to channel: %w", err)
			io.WriteString(s.Stderr(), fmt.Sprintln(err.Error()))
			return
		}

		slog.Debug("receiver connected", "channel", name)
		defer slog.Debug("receiver disconnected", "channel", name)

		channel, err := engine.AddReceiver(name, s, s.Stderr())
		if err != nil {
			err = fmt.Errorf("could not connect to channel: %w", err)
			io.WriteString(s.Stderr(), fmt.Sprintln(err.Error()))
			return
		}
		if !args.Quiet {
			io.WriteString(s.Stderr(), fmt.Sprintf("-> connected to %s as receiver\n\n", name))

			if channel.Sender == nil {
				io.WriteString(
					s.Stderr(),
					fmt.Sprintf("To send data on this beam pipe it into: ssh %s send %s\n\n", config.Host, name),
				)
			}
		}

		// Block until beamer is done or the connection is aborted.
		select {
		case <-s.Context().Done():
			channel.Quit <- s.Context().Err()

		case err := <-channel.Receiver.Done:
			if !args.Quiet {
				if err == nil {
					io.WriteString(s.Stderr(), "beaming down complete\n")
				} else {
					io.WriteString(s.Stderr(), fmt.Sprintln(err.Error()))
				}
			}
		}
	}
}

// Select the name of the channel based on the explicit option or an implicit
// channel based on the public key.
func makeChannel(config *config.Config, name string, key ssh.PublicKey) (string, error) {
	name = strings.TrimSpace(name)
	if name != "" {
		if len(name) < 6 {
			return "", fmt.Errorf("channel names needs to between 6 to 64 characters long")
		}
		if len(name) > 64 {
			return "", fmt.Errorf("channel names needs to between 6 to 64 characters long")
		}
		if ok, _ := regexp.MatchString(`^\w+$`, name); !ok {
			return "", fmt.Errorf("channel name can only contain lowercase, uppercase letters, digits and underscores")
		}
		return name, nil
	}

	h := sha1.New()
	h.Write(key.Marshal())
	h.Write([]byte(config.Secret))
	digest := h.Sum(nil)
	return base58.Encode(digest), nil
}

func run() error {
	engine := beam.NewEngine()

	config, err := config.LoadConfigFromEnv()
	if err != nil {
		return fmt.Errorf("could not load configuration: %w", err)
	}

	slog.Info("starting listening", "port", config.BindAddr)
	err = ssh.ListenAndServe(
		config.BindAddr,
		func(s ssh.Session) { handler(config, engine, s) },
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
