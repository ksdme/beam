package main

import (
	"crypto/rand"
	"crypto/sha1"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/btcsuite/btcutil/base58"
	"github.com/gliderlabs/ssh"
	"github.com/ksdme/beam/internal/beam"
	"github.com/ksdme/beam/internal/config"

	gossh "golang.org/x/crypto/ssh"
)

// - Add a progress meter.
// - Authorized Keys
func handler(config *config.Config, engine *beam.Engine, s ssh.Session) {
	// Calling s.Exit does not seem to cancel the context, so, we need to manually
	// store that intent and return early if parsing arguments fail.
	exited := false
	exit := func(i int) {
		s.Exit(i)
		exited = true
	}

	// Parse the command passed.
	type send struct {
		RandomChannel bool `arg:"--random-channel,-r" help:"use a random channel name"`
		BufferSize    int  `arg:"--buffer-size,-b" default:"64" help:"buffer size in kB (between 1 and 64)"`
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
	}
	if exited {
		return
	}

	switch {
	case args.Send != nil:
		// You are not allowed to send to any channel.
		name, err := makeChannelName(config, s.PublicKey(), args.Send.RandomChannel)
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
		// You are allowed to receive on any channel though.
		name := strings.TrimSpace(args.Receive.Channel)
		if name == "" {
			name, err = makeChannelName(config, s.PublicKey(), false)
			if err != nil {
				slog.Debug("could not determine channel name", "err", err)
				err = fmt.Errorf("could not connect to channel: %w", err)
				io.WriteString(s.Stderr(), fmt.Sprintln(err.Error()))
				return
			}
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
			io.WriteString(s.Stderr(), fmt.Sprintf("-> connected to %s as receiver\n", name))
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

// Generate a random channel name or a name based on the public key
// signature of the participant.
func makeChannelName(config *config.Config, key ssh.PublicKey, random bool) (string, error) {
	var base []byte
	if random {
		b := make([]byte, 512)
		if n, err := rand.Read(b); err != nil {
			return "", fmt.Errorf("could not generate random channel name: %w", err)
		} else {
			base = b[:n]
		}
	} else {
		base = key.Marshal()
	}

	h := sha1.New()
	h.Write(base)
	h.Write([]byte(config.Secret))
	digest := h.Sum(nil)
	return base58.Encode(digest), nil
}

func run() error {
	engine := beam.NewEngine()

	config, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("could not load configuration: %w", err)
	}

	server := &ssh.Server{
		Addr:                       config.BindAddr,
		MaxTimeout:                 time.Duration(config.MaxTimeout) * time.Second,
		IdleTimeout:                time.Duration(config.IdleTimeout) * time.Second,
		Handler:                    func(s ssh.Session) { handler(config, engine, s) },
		PasswordHandler:            func(ctx ssh.Context, password string) bool { return false },
		PublicKeyHandler:           func(ctx ssh.Context, key ssh.PublicKey) bool { return true },
		KeyboardInteractiveHandler: func(ctx ssh.Context, challenger gossh.KeyboardInteractiveChallenge) bool { return false },
	}
	ssh.HostKeyFile(config.HostKeyFile)(server)

	slog.Info("listening", "addr", config.BindAddr)
	if err = server.ListenAndServe(); err != nil {
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
