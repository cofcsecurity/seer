// pkg/job/xattr_other.go
//go:build !linux

package job

func copyExtendedAttributes(from, to string) error { return nil }
