package iochan

import "io"

type ReadResult struct {
	N    int
	Data []byte
	Err  error
}

// Read from the reader and yield chunks into the returned channel.
// The channel terminates when there is an error on the reader.
func ReadToBufferedChannel(reader io.Reader, chunk int, buffers int) chan ReadResult {
	ch := make(chan ReadResult, buffers)
	go read(reader, chunk, ch)
	return ch
}

// Loop the reader and write to the result channel.
func read(reader io.Reader, chunk int, result chan ReadResult) {
	for {
		buffer := make([]byte, chunk)
		n, err := reader.Read(buffer)
		result <- ReadResult{
			N:    n,
			Data: buffer[:n],
			Err:  err,
		}
		if err != nil {
			close(result)
			return
		}
	}
}
