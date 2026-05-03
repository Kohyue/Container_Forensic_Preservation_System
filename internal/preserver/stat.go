package preserver

import (
	"fmt"
	"os"
)

func statFile(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat %q: %w", path, err)
	}
	return fi.Size(), nil
}
