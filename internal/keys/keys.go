package keys

import (
	"bufio"
	"fmt"
	"os"

	"github.com/gliderlabs/ssh"
)

// Returns a map of authorized keys from a file.
func LoadAuthorizedKeys(name string) (map[string]bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, fmt.Errorf("could not open authorized keys file: %w", err)
	}
	defer f.Close()

	keys := make(map[string]bool)

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Bytes()
		if len(line) == 0 {
			continue
		}

		key, _, _, _, err := ssh.ParseAuthorizedKey(line)
		if err != nil {
			return nil, fmt.Errorf("could not parse authorized key: %w", err)
		}

		keys[string(key.Marshal())] = true
	}

	return keys, nil
}
