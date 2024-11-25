package beam

import (
	"fmt"
	"io"
	"log/slog"
	"sync"
)

type Engine struct {
	lock     sync.Mutex
	channels map[string]*channel
}

type channel struct {
	ready    chan bool
	Quit     chan error
	Sender   *sender
	Receiver *receiver
}

type sender struct {
	reader io.Reader
	log    io.Writer
	Done   chan error
}

type receiver struct {
	writer io.Writer
	log    io.Writer
	Done   chan error
}

func NewEngine() *Engine {
	return &Engine{
		channels: make(map[string]*channel),
	}
}

func (e *Engine) findOrCreateChannel(name string) *channel {
	ch, exists := e.channels[name]
	if !exists {
		ch = &channel{
			Quit: make(chan error),
		}
		e.channels[name] = ch
	}

	return ch
}

func (e *Engine) createBeamer(name string, channel *channel) {
	if channel.ready == nil {
		channel.ready = make(chan bool)
		go e.beam(name, channel)
	}

	if channel.Sender != nil && channel.Receiver != nil {
		channel.ready <- true
	}
}

// Adds a sender to a specific session if one doesn't already exist. And, returns
// a channel that yields an exit code when the beaming is complete.
func (e *Engine) AddSender(name string, reader io.Reader, log io.Writer) (*channel, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	channel := e.findOrCreateChannel(name)
	if channel.Sender != nil {
		return nil, fmt.Errorf("session has another active sender")
	}

	channel.Sender = &sender{
		reader: reader,
		log:    log,
		Done:   make(chan error),
	}
	e.createBeamer(name, channel)

	return channel, nil
}

// Adds a receiver to a specific session if one doesn't already exist. And, returns
// a channel that yields an exit code when the beaming is complete.
func (e *Engine) AddReceiver(name string, writer io.Writer, log io.Writer) (*channel, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	channel := e.findOrCreateChannel(name)
	if channel.Receiver != nil {
		return nil, fmt.Errorf("session has another active receiver")
	}

	channel.Receiver = &receiver{
		writer: writer,
		log:    log,
		Done:   make(chan error),
	}
	e.createBeamer(name, channel)

	return channel, nil
}

func (e *Engine) beam(name string, channel *channel) {
	slog.Debug("started up beamer", "channel", name)
	defer slog.Debug("closing up beamer", "channel", name)
	defer e.clean(name)

	// Send a done signal to both the participant channels in a non-blocking
	// manner and, only if they exist.
	done := func(s, r error) {
		if channel.Sender != nil {
			select {
			case channel.Sender.Done <- s:
			default:
			}
		}

		if channel.Receiver != nil {
			select {
			case channel.Receiver.Done <- r:
			default:
			}
		}
	}

	// This is a blocking read that will only run when the entire channel is ready.
	// If one of the participants goes away while waiting, this worker will die.
	select {
	case <-channel.Quit:
		done(nil, nil)
		return

	case <-channel.ready:
	}

	// Run until the termination of the worker is explicitly requested (mostly when
	// either participant unexpectedly goes away) or untilt the transfer is complete.
	for {
		select {
		case <-channel.Quit:
			err := fmt.Errorf("connection interrupted")
			done(err, err)
			return

		default:
			buffer := make([]byte, 64*1024)
			s, err := channel.Sender.reader.Read(buffer)
			if err != nil {
				if err == io.EOF {
					done(nil, nil)
					return
				}

				slog.Info("err reading from sender", "channel", name, "err", err)
				done(fmt.Errorf("error uploading"), fmt.Errorf("error on the sender end"))
				return
			}

			_, err = channel.Receiver.writer.Write(buffer[:s])
			if err != nil {
				slog.Info("err writing to receiver", "channel", name, "err", err)
				done(fmt.Errorf("error on the receiver end"), fmt.Errorf("error downloading"))
				return
			}
		}
	}
}

func (e *Engine) clean(channel string) {
	slog.Debug("cleaning up", "channel", channel)

	e.lock.Lock()
	defer e.lock.Unlock()

	delete(e.channels, channel)
}
