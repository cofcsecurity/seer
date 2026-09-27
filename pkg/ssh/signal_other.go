//go:build !linux || (!amd64 && !arm64)

package ssh

import "fmt"

func signalVerifiedSession(target Session) error {
	return fmt.Errorf("ending SSH sessions requires Linux amd64 or arm64 with pidfd support")
}
