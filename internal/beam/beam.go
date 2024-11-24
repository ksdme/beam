package beam

import (
	"fmt"
	"io"
	"log/slog"
	"sync"
)

type Engine struct {
	lock     sync.Mutex
	sessions map[string]*session
}

type session struct {
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
		sessions: make(map[string]*session),
	}
}

func (b *Engine) getOrCreateSession(name string) *session {
	sess, exists := b.sessions[name]
	if !exists {
		sess = &session{}
		b.sessions[name] = sess
	}
	return sess
}

// Adds a sender to a specific session if one doesn't already exist. And, returns
// a channel that yields an exit code when the beaming is complete.
func (b *Engine) AddSender(session string, reader io.Reader, log io.Writer) (chan int, error) {
	b.lock.Lock()
	defer b.lock.Unlock()

	sess := b.getOrCreateSession(session)

	if sess.sender != nil {
		return nil, fmt.Errorf("this session has an active sender")
	}
	sess.sender = &sender{
		reader: reader,
		log:    log,
		done:   make(chan int),
	}

	b.checkAndBeam(sess)

	return sess.sender.done, nil
}

// Adds a receiver to a specific session if one doesn't already exist. And, returns
// a channel that yields an exit code when the beaming is complete.
func (b *Engine) AddReceiver(session string, writer io.Writer, log io.Writer) (chan int, error) {
	b.lock.Lock()
	defer b.lock.Unlock()

	sess := b.getOrCreateSession(session)

	if sess.receiver != nil {
		return nil, fmt.Errorf("this session has an active receiver")
	}
	sess.receiver = &receiver{
		writer: writer,
		log:    log,
		done:   make(chan int),
	}

	b.checkAndBeam(sess)

	return sess.receiver.done, nil
}

func (b *Engine) checkAndBeam(session *session) {
	if session.sender != nil && session.receiver != nil {
		go b.beam(session)
	}
}

// Basically, reader a buffer from reader and write it to receiver. This is not
// done in parallel to keep the memory footprint low.
func (b *Engine) beam(session *session) {
	for {
		buffer := make([]byte, 64*1024)
		n, err := session.sender.reader.Read(buffer)
		if err != nil {
			if err == io.EOF {
				io.WriteString(session.sender.log, "Beaming Complete\n")
				io.WriteString(session.receiver.log, "Beaming Complete\n")

				session.sender.done <- 0
				session.receiver.done <- 0
			} else {
				slog.Error("sender: could not read", "err", err)
				io.WriteString(session.sender.log, "Something went wrong while sending\n")
				io.WriteString(session.receiver.log, "Something went wrong on the sender end\n")

				session.sender.done <- 1
				session.receiver.done <- 1
			}
			return
		}

		_, err = session.receiver.writer.Write(buffer[:n])
		if err != nil {
			slog.Error("receiver: could not write", "err", err)
			io.WriteString(session.receiver.log, "Something went wrong while receiving\n")
			io.WriteString(session.sender.log, "Something went wrong on the receiver end\n")

			session.sender.done <- 1
			session.receiver.done <- 1
			return
		}
	}
}
