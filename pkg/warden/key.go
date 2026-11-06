package warden

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"
)

// keyTypes are the OpenSSH public key types Warden accepts for the team key.
var keyTypes = map[string]bool{
	"ssh-ed25519":                        true,
	"ssh-rsa":                            true,
	"ecdsa-sha2-nistp256":                true,
	"ecdsa-sha2-nistp384":                true,
	"ecdsa-sha2-nistp521":                true,
	"sk-ssh-ed25519@openssh.com":         true,
	"sk-ecdsa-sha2-nistp256@openssh.com": true,
}

// maxKeyBytes bounds a key downloaded from a URL; a public key is far smaller.
const maxKeyBytes = 64 << 10

// keyFetchTimeout bounds the download of a team key from a URL.
const keyFetchTimeout = 30 * time.Second

// ResolveKey reads the team public key given to --key and returns its single
// OpenSSH public key line. The value may be the key itself, @file, or an https
// URL. Anything Warden would reject is reported here, before any clone or
// download, with the fix to apply.
func ResolveKey(ctx context.Context, value string) (string, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", errors.New("--key is empty: give the team public key line, @file, or https URL")
	}
	var text, source string
	switch {
	case strings.HasPrefix(v, "@"):
		path := strings.TrimPrefix(v, "@")
		if path == "" {
			return "", errors.New("--key @ names no file: use @path/to/team.pub")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("cannot read team key file %s: %s", path, describeReadError(err))
		}
		text, source = string(data), "key file "+path
	case strings.HasPrefix(v, "https://"):
		data, err := fetchKey(ctx, v)
		if err != nil {
			return "", err
		}
		text, source = data, "key URL "+v
	default:
		text, source = v, "--key value"
	}
	return parseTeamKey(text, source)
}

func describeReadError(err error) string {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "the file does not exist; check the path (relative paths are from the current directory)"
	case errors.Is(err, fs.ErrPermission):
		return "permission denied; run as root or make the file readable"
	case errors.Is(err, fs.ErrInvalid):
		return "it is not a regular file"
	}
	return err.Error()
}

func fetchKey(ctx context.Context, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, keyFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("invalid --key URL %s: %w", url, err)
	}
	req.Header.Set("User-Agent", "seer")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot download the team key from %s: %w; check this box's network, or give the key inline or as @file", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the team key URL %s returned %d; check the URL points at the raw public key", url, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxKeyBytes+1))
	if err != nil {
		return "", fmt.Errorf("reading the team key from %s: %w", url, err)
	}
	if len(data) > maxKeyBytes {
		return "", fmt.Errorf("the file at %s is larger than a public key; check the URL", url)
	}
	return string(data), nil
}

// parseTeamKey checks that text holds exactly one OpenSSH public key and
// returns its line ("<type> <base64>", with any comment kept).
func parseTeamKey(text, source string) (string, error) {
	if strings.Contains(text, "-----BEGIN") {
		return "", fmt.Errorf("the %s is a PRIVATE key; --key needs the public key, the one line in the .pub file that starts with ssh-ed25519, ssh-rsa, or ecdsa-sha2-", source)
	}
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	switch len(lines) {
	case 0:
		return "", fmt.Errorf("the %s is empty; --key needs the team public key line", source)
	case 1:
	default:
		return "", fmt.Errorf("the %s has %d key lines; --key takes exactly one team public key", source, len(lines))
	}

	line := lines[0]
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", fmt.Errorf("the %s is not a public key: expected \"<key type> <base64 key>\" such as \"ssh-ed25519 AAAA...\"", source)
	}
	typ, data := fields[0], fields[1]
	if !keyTypes[typ] {
		return "", fmt.Errorf("the %s has key type %q, which Warden does not accept; use an ssh-ed25519, ssh-rsa, or ecdsa-sha2-nistp256/384/521 public key (options such as from=\"...\" before the type are not supported)", source, typ)
	}
	blob, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", fmt.Errorf("the %s is not a valid public key: its key data is not base64; copy the .pub line again", source)
	}
	// The blob starts with its own type as a length-prefixed string. A
	// truncated or mangled key fails this check even when it is valid base64.
	if len(blob) < 4 {
		return "", fmt.Errorf("the %s is truncated; copy the .pub line again", source)
	}
	n := binary.BigEndian.Uint32(blob[:4])
	if uint64(n) > uint64(len(blob)-4) || string(blob[4:4+n]) != typ {
		return "", fmt.Errorf("the key data in the %s does not match its type %s; the key is truncated or corrupted, copy the .pub line again", source, typ)
	}
	return line, nil
}
