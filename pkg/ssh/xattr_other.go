//go:build !linux

package ssh

func copyExtendedAttributes(from, to string) error { return nil }
