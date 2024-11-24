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
	sender   *sender
	receiver *receiver
}

type sender struct {
	reader io.Reader
	log    io.Writer
	done   chan int
}

type receiver struct {
	writer io.Writer
	log    io.Writer
	done   chan int
}

func NewEngine() *Engine {
	return &Engine{
		channels: make(map[string]*channel),
	}
}

func (e *Engine) getOrCreateChannel(name string) *channel {
	sess, exists := e.channels[name]
	if !exists {
		sess = &channel{}
		e.channels[name] = sess
	}
	return sess
}

// Adds a sender to a specific session if one doesn't already exist. And, returns
// a channel that yields an exit code when the beaming is complete.
func (e *Engine) AddSender(name string, reader io.Reader, log io.Writer) (chan int, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	channel := e.getOrCreateChannel(name)

	if channel.sender != nil {
		return nil, fmt.Errorf("this session has another active sender")
	}
	channel.sender = &sender{
		reader: reader,
		log:    log,
		done:   make(chan int),
	}

	if err := e.checkAndBeam(name, channel); err != nil {
		return nil, err
	}

	return channel.sender.done, nil
}

// Adds a receiver to a specific session if one doesn't already exist. And, returns
// a channel that yields an exit code when the beaming is complete.
func (e *Engine) AddReceiver(name string, writer io.Writer, log io.Writer) (chan int, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	channel := e.getOrCreateChannel(name)

	if channel.receiver != nil {
		return nil, fmt.Errorf("this session has another active receiver")
	}
	channel.receiver = &receiver{
		writer: writer,
		log:    log,
		done:   make(chan int),
	}

	if err := e.checkAndBeam(name, channel); err != nil {
		return nil, err
	}

	return channel.receiver.done, nil
}

func (e *Engine) checkAndBeam(name string, channel *channel) error {
	if channel.sender != nil && channel.receiver != nil {
		go e.beam(name, channel)
	}

	return nil
}

// Basically, reader a buffer from reader and write it to receiver. This is not
// done in parallel to keep the memory footprint low.
func (e *Engine) beam(name string, channel *channel) {
	defer e.clean(name)

	sent := uint64(0)
	received := uint64(0)
	lastSentReported := sent
	lastReceivedReported := received
	reportSize := uint64(5 * 1024 * 1024)

	for {
		buffer := make([]byte, 64*1024)
		s, err := channel.sender.reader.Read(buffer)
		if err != nil {
			if err == io.EOF {
				io.WriteString(channel.sender.log, fmt.Sprintf("Beaming Complete (%s)\n", humanize.Bytes(sent)))
				io.WriteString(channel.receiver.log, fmt.Sprintf("Beaming Complete (%s)\n", humanize.Bytes(received)))

				channel.sender.done <- 0
				channel.receiver.done <- 0
			} else {
				slog.Error("sender: could not read", "err", err)
				io.WriteString(channel.sender.log, "Something went wrong while sending\n")
				io.WriteString(channel.receiver.log, "Something went wrong on the sender end\n")

				channel.sender.done <- 1
				channel.receiver.done <- 1
			}
			return
		}
		sent += uint64(s)
		if sent-lastSentReported >= reportSize {
			io.WriteString(channel.sender.log, fmt.Sprintf("# Uploaded %s\n", humanize.Bytes(sent)))
			lastSentReported = sent
		}

		r, err := channel.receiver.writer.Write(buffer[:s])
		if err != nil {
			slog.Error("receiver: could not write", "err", err)
			io.WriteString(channel.receiver.log, "Something went wrong while receiving\n")
			io.WriteString(channel.sender.log, "Something went wrong on the receiver end\n")

			channel.sender.done <- 1
			channel.receiver.done <- 1
			return
		}
		received += uint64(r)
		if received-lastReceivedReported >= reportSize {
			io.WriteString(channel.receiver.log, fmt.Sprintf("# Downloaded %s\n", humanize.Bytes(received)))
			lastReceivedReported = received
		}
	}
}

func (e *Engine) clean(channel string) {
	e.lock.Lock()
	defer e.lock.Unlock()

	delete(e.channels, channel)
}
