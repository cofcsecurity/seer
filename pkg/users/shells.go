package users

import (
	"bufio"
	"errors"
	"os"
	"strings"
)

func GetShells() (shells []string, err error) {
	shell_db, err := os.Open("/etc/shells")
	if err != nil {
		return nil, errors.New("failed to read /etc/shells")
	}
	defer shell_db.Close()

	scanner := bufio.NewScanner(shell_db)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 1 || line[0] == '#' {
			continue
		}

		line = strings.ReplaceAll(line, "\n", "")
		line = strings.ReplaceAll(line, "\t", "")

		shells = append(shells, line)
	}

	return shells, nil
}
