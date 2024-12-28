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
	ready     chan bool
	Sender    *sender
	Receiver  *receiver
	Started   bool
	Interrput chan error
}

type sender struct {
	chunk      int
	reader     io.Reader
	Done       chan error
	TotalBytes uint64
}

type receiver struct {
	writer     io.Writer
	Done       chan error
	TotalBytes uint64
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
			Interrput: make(chan error),
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
func (e *Engine) AddSender(name string, reader io.Reader, chunk int) (*channel, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	channel := e.findOrCreateChannel(name)
	if channel.Sender != nil {
		return nil, fmt.Errorf("session has another active sender")
	}

	channel.Sender = &sender{
		chunk:  chunk,
		reader: reader,
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
		Done:   make(chan error),
	}
	e.createBeamer(name, channel)

	return channel, nil
}

func (e *Engine) beam(name string, channel *channel) {
	defer e.clean(name)
	slog.Info("started up beamer", "channel", name)
	defer slog.Info("closing up beamer", "channel", name)

	// Send a done signal to both the participant channels in a non-blocking
	// manner and, only if they exist.
	terminate := func(s, r error) {
		if channel.Sender != nil {
			nbsend(channel.Sender.Done, s)
		}

		if channel.Receiver != nil {
			nbsend(channel.Receiver.Done, r)
		}
	}

	// This is a blocking read that will only resolve when the entire channel is ready.
	// If one of the participants goes away while waiting, this worker will die.
	select {
	case <-channel.Interrput:
		terminate(nil, nil)
		return

	case <-channel.ready:
		channel.Started = true
	}

	// Sender loop.
	chunks := make(chan []byte, 4)
	senderErr := make(chan error)
	go func() {
		slog.Debug("sender loop started", "channel", name)
		defer slog.Debug("sender loop closed", "channel", name)

		for {
			buffer := make([]byte, channel.Sender.chunk)
			n, err := channel.Sender.reader.Read(buffer)
			if err != nil {
				if err != io.EOF {
					nbsend(senderErr, err)
				}
				close(chunks)
				return
			}

			channel.Sender.TotalBytes += uint64(n)
			chunks <- buffer[:n]
		}
	}()

	// Receiver loop.
	complete := make(chan bool)
	receiverErr := make(chan error)
	go func() {
		slog.Debug("receiver loop started", "channel", name)
		defer slog.Debug("receiver loop closed", "channel", name)

		for chunk := range chunks {
			n, err := channel.Receiver.writer.Write(chunk)
			if err != nil {
				nbsend(receiverErr, err)
				return
			}

			channel.Receiver.TotalBytes += uint64(n)
		}

		// Once we have processed all chunks and the chunks channel has closed,
		// we are done.
		nbsend(complete, true)
	}()

	slog.Debug("beaming", "channel", name, "chunk", channel.Sender.chunk)
	select {
	case <-complete:
		terminate(nil, nil)
		return

	case <-channel.Interrput:
		// When the channel is quit, we expect the streams to eventually be closed,
		// so, the sender channel from above should also die in a cycle or two.
		err := fmt.Errorf("connection interrupted")
		terminate(err, err)
		return

	// An error on either the sender or receiver will trigger the termination of
	// both the sender and the receiver. This should eventually cause the reader
	// loop to fail with an err and exit.
	case err := <-senderErr:
		slog.Info("err reading from sender", "channel", name, "err", err)
		terminate(fmt.Errorf("could not upload: connection terminated"), fmt.Errorf("sender interrupted"))
		return

	case err := <-receiverErr:
		slog.Info("err writing to receiver", "channel", name, "err", err)
		terminate(fmt.Errorf("error on the receiver end"), fmt.Errorf("error downloading"))
		return
	}
}

func (e *Engine) clean(channel string) {
	slog.Info("cleaning up", "channel", channel)

	e.lock.Lock()
	defer e.lock.Unlock()

	delete(e.channels, channel)
}

// Utilty for non blocking send to a channel.
func nbsend[K any](ch chan<- K, value K) {
	select {
	case ch <- value:
	default:
	}
}
