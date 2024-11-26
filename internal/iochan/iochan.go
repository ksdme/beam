package iochan

import "io"

type ReadResult struct {
	Data []byte
	Err  error
}

// Read from the reader and yield chunks into the returned channel. The channel
// terminates when there is an error on the reader.
func ReadToChannel(reader io.Reader, b int) chan ReadResult {
	ch := make(chan ReadResult)
	go read(reader, b, ch)
	return ch
}

// Loop the reader and write to the result channel.
func read(reader io.Reader, b int, r chan ReadResult) {
	buffer := make([]byte, b)
	for {
		n, err := reader.Read(buffer)
		r <- ReadResult{
			Data: buffer[:n],
			Err:  err,
		}
		if err != nil {
			close(r)
			return
		}
	}
}
