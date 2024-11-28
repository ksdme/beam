package iochan

import "io"

type ReadResult struct {
	N    int
	Data []byte
	Err  error
}

// Read from the reader and yield chunks into the returned channel. The channel
// terminates when there is an error on the reader.
func ReadToChannel(reader io.Reader, b int) chan ReadResult {
	ch := make(chan ReadResult, 3)
	go read(reader, b, ch)
	return ch
}

// Loop the reader and write to the result channel.
func read(reader io.Reader, size int, result chan ReadResult) {
	for {
		buffer := make([]byte, size)
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
