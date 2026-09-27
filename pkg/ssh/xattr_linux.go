//go:build linux

package ssh

import (
	"errors"
	"syscall"
)

// Preserve labels and ACLs carried as extended attributes before replacing
// authorized_keys. If they cannot be copied, leave the original untouched.
func copyExtendedAttributes(from, to string) error {
	size, err := syscall.Listxattr(from, nil)
	if errors.Is(err, syscall.ENOTSUP) {
		return nil
	}
	if err != nil || size == 0 {
		return err
	}
	names := make([]byte, size)
	n, err := syscall.Listxattr(from, names)
	if err != nil {
		return err
	}
	for _, raw := range splitNUL(names[:n]) {
		name := string(raw)
		valueSize, err := syscall.Getxattr(from, name, nil)
		if err != nil {
			return err
		}
		value := make([]byte, valueSize)
		if _, err := syscall.Getxattr(from, name, value); err != nil {
			return err
		}
		if err := syscall.Setxattr(to, name, value, 0); err != nil {
			return err
		}
	}
	return nil
}

func splitNUL(data []byte) [][]byte {
	var result [][]byte
	start := 0
	for i, b := range data {
		if b == 0 {
			if i > start {
				result = append(result, data[start:i])
			}
			start = i + 1
		}
	}
	return result
}
