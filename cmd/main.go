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
	"github.com/dustin/go-humanize"
	"github.com/gliderlabs/ssh"
	"github.com/ksdme/beam/internal/beam"
	"github.com/ksdme/beam/internal/config"
	"github.com/ksdme/beam/internal/keys"
	"github.com/ksdme/beam/internal/spinner"
)

// Handle a connection.
func handler(config *config.Config, engine *beam.Engine, s ssh.Session) {
	// Block interactive calls.
	if _, _, active := s.Pty(); active {
		_, _ = io.WriteString(s, "This server does not support interactive terminal sessions.\n")
		_ = s.Exit(1)
		return
	}

	// Calling s.Exit does not seem to cancel the context, so, we need to manually
	// store that intent and return early if parsing arguments fail.
	exited := false
	exit := func(i int) {
		_ = s.Exit(i)
		exited = true
	}

	// Parse the command passed.
	type send struct {
		RandomChannel bool `arg:"--random-channel,-r" help:"use a random channel name"`
		BufferSize    int  `arg:"--buffer-size,-b" default:"8192" help:"buffer size in bytes (between 64 and 65536)"`
		Progress      bool `arg:"--progress,-p" default:"true" help:"show channel and progress log"`
	}
	type receive struct {
		Channel  string `arg:"positional"`
		Progress bool   `arg:"--progress,-p" default:"false" help:"show channel and progress log"`
	}
	var args struct {
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
		_, _ = io.WriteString(s.Stderr(), fmt.Sprintln("internal error"))
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
		if args.Send.BufferSize < 64 {
			err = parser.FailSubcommand("buffer size needs to be between 512 and 65536", "send")
			if err != nil {
				return
			}
		}
		if args.Send.BufferSize > 65536 {
			err = parser.FailSubcommand("buffer size needs to be between 512 and 65536", "send")
			if err != nil {
				return
			}
		}
	}
	if exited {
		return
	}

	switch {
	case args.Send != nil:
		// You are not allowed to send to any channel.
		name, err := resolveTargetChannel(config, s.PublicKey(), args.Send.RandomChannel)
		if err != nil {
			slog.Debug("could not determine channel name", "err", err)
			err = fmt.Errorf("could not connect to channel: %w", err)
			_, _ = io.WriteString(s.Stderr(), fmt.Sprintln(err.Error()))
			return
		}

		slog.Debug("sender connected", "channel", name)
		defer slog.Debug("sender disconnected", "channel", name)

		channel, err := engine.AddSender(name, s, args.Send.BufferSize)
		if err != nil {
			err = fmt.Errorf("could not connect to channel: %w", err)
			_, _ = io.WriteString(s.Stderr(), fmt.Sprintln(err.Error()))
			return
		}
		if args.Send.Progress {
			_, _ = io.WriteString(s.Stderr(), fmt.Sprintf("<- connected to %s as sender\n\n", name))

			if channel.Receiver == nil {
				_, _ = io.WriteString(
					s.Stderr(),
					fmt.Sprintf(
						"To receive this beam use:\n"+
							"$ ssh %s receive %s\n\n",
						config.Host,
						name,
					),
				)
			}
		}

		spin := spinner.NewSpinner(s.Stderr())
	sloop:
		// Block until beamer is done or the connection is aborted.
		// Push the update while that happens though.
		for {
			select {
			case err := <-channel.Sender.Done:
				if args.Send.Progress {
					if err == nil {
						_, _ = io.WriteString(
							s.Stderr(),
							fmt.Sprintf(
								"beaming up complete (%s)\n",
								humanize.Bytes(channel.Sender.TotalBytes),
							),
						)
					} else {
						_, _ = io.WriteString(s.Stderr(), fmt.Sprintln(err.Error()))
					}
				}
				break sloop

			case <-s.Context().Done():
				channel.Interrput <- s.Context().Err()

			case <-time.After(200 * time.Millisecond):
				if args.Send.Progress {
					if channel.Started {
						_ = spin.Render(fmt.Sprintf(
							"uploaded %s",
							humanize.Bytes(channel.Sender.TotalBytes),
						))
					} else {
						_ = spin.Render("waiting for receiver")
					}
				}
			}
		}

	case args.Receive != nil:
		name := strings.TrimSpace(args.Receive.Channel)
		if name == "" {
			name, err = resolveTargetChannel(config, s.PublicKey(), false)
			if err != nil {
				slog.Debug("could not determine channel name", "err", err)
				err = fmt.Errorf("could not connect to channel: %w", err)
				_, _ = io.WriteString(s.Stderr(), fmt.Sprintln(err.Error()))
				return
			}
		}

		slog.Debug("receiver connected", "channel", name)
		defer slog.Debug("receiver disconnected", "channel", name)

		channel, err := engine.AddReceiver(name, s, s.Stderr())
		if err != nil {
			err = fmt.Errorf("could not connect to channel: %w", err)
			_, _ = io.WriteString(s.Stderr(), fmt.Sprintln(err.Error()))
			return
		}
		if args.Receive.Progress {
			_, _ = io.WriteString(s.Stderr(), fmt.Sprintf("-> connected to %s as receiver\n\n", name))
		}

		spin := spinner.NewSpinner(s.Stderr())
	rloop:
		for {
			// Block until beamer is done or the connection is aborted.
			select {
			case err := <-channel.Receiver.Done:
				if args.Receive.Progress {
					if err == nil {
						_, _ = io.WriteString(
							s.Stderr(),
							fmt.Sprintf(
								"beaming down complete (%s)\n",
								humanize.Bytes(channel.Receiver.TotalBytes),
							),
						)
					} else {
						_, _ = io.WriteString(s.Stderr(), fmt.Sprintln(err.Error()))
					}
				}
				break rloop

			case <-s.Context().Done():
				channel.Interrput <- s.Context().Err()

			case <-time.After(200 * time.Millisecond):
				if args.Receive.Progress {
					if channel.Started {
						_ = spin.Render(fmt.Sprintf(
							"downloaded %s",
							humanize.Bytes(channel.Receiver.TotalBytes),
						))
					} else {
						_ = spin.Render("waiting for sender")
					}
				}
			}
		}
	}
}

// Generate a random channel name or a name based on the public key
// signature of the participant.
func resolveTargetChannel(config *config.Config, key ssh.PublicKey, random bool) (string, error) {
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

	var authorized map[string]bool
	if config.AuthorizedKeysFile != "" {
		authorized, err = keys.LoadAuthorizedKeys(config.AuthorizedKeysFile)
		if err != nil {
			return fmt.Errorf("could not load authorized keys: %w", err)
		}
		slog.Info("loaded authorized keys", "count", len(authorized))
	}

	server := &ssh.Server{
		Addr:        config.BindAddr,
		MaxTimeout:  time.Duration(config.MaxTimeout) * time.Second,
		IdleTimeout: time.Duration(config.IdleTimeout) * time.Second,
		Handler:     func(s ssh.Session) { handler(config, engine, s) },
		PublicKeyHandler: func(ctx ssh.Context, key ssh.PublicKey) bool {
			if authorized != nil {
				_, ok := authorized[string(key.Marshal())]
				return ok
			}
			return true
		},
	}
	err = ssh.HostKeyFile(config.HostKeyFile)(server)
	if err != nil {
		return fmt.Errorf("could not add hostkey to the server: %w", err)
	}

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
