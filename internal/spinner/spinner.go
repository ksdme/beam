package spinner

import (
	"fmt"
	"io"

	"github.com/muesli/termenv"
)

var (
	// https://github.com/briandowns/spinner?tab=readme-ov-file#available-character-sets
	pattern = [...]string{"⠁", "⠂", "⠄", "⡀", "⢀", "⠠", "⠐", "⠈"}
)

// A spinner that renders when asked and allows you to clear its trail on demand.
type Spinner struct {
	tick   uint
	output *termenv.Output
}

func NewSpinner(writer io.Writer) *Spinner {
	return &Spinner{
		output: termenv.NewOutput(writer),
	}
}

func (s *Spinner) next() {
	s.tick += 1
}

func (s *Spinner) Clear() {
	if s.tick > 0 {
		s.output.CursorPrevLine(2)
		s.output.ClearLine()
	}
}

func (s *Spinner) Render(message string) error {
	defer s.next()

	current := pattern[s.tick%uint(len(pattern))]
	line := fmt.Sprintf("%s %s\n\n", current, message)

	s.Clear()
	if _, err := s.output.WriteString(line); err != nil {
		return err
	}
	return nil
}
