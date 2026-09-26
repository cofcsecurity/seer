package ssh

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

type Account struct {
	Name string
	UID  string
	Home string
}

type AuthorizedKey struct {
	User        string
	Path        string
	Line        int
	Type        string
	Fingerprint string
	Comment     string
	Options     string
	content     string
}

func (k AuthorizedKey) String() string {
	return fmt.Sprintf("%s SHA256:%s %s %s:%d %s\n", k.User, k.Fingerprint, k.Type, k.Path, k.Line, k.Comment)
}

func (k AuthorizedKey) Describe() string {
	return fmt.Sprintf("┌ %s SHA256:%s\n├ Type: %s\n├ File: %s:%d\n├ Options: %s\n└ Comment: %s\n",
		k.User, k.Fingerprint, k.Type, k.Path, k.Line, k.Options, k.Comment)
}

func accounts() ([]Account, error) {
	var data []byte
	if path := systemCommand("getent"); path != "" {
		data, _ = exec.Command(path, "passwd").Output()
	}
	if len(data) == 0 {
		var err error
		data, err = os.ReadFile("/etc/passwd")
		if err != nil {
			return nil, err
		}
	}
	var result []Account
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Split(line, ":")
		if len(f) >= 7 && f[0] != "" && f[5] != "" {
			result = append(result, Account{Name: f[0], UID: f[2], Home: f[5]})
		}
	}
	return result, nil
}

func keyPaths(account Account, configPath, clientAddr string) ([]string, error) {
	patterns := []string{".ssh/authorized_keys", ".ssh/authorized_keys2"}
	criteria := map[string]string{"user": account.Name, "addr": clientAddr}
	effective, configErr := EffectiveConfig(configPath, criteria)
	if configErr == nil {
		if value, ok := effective["authorizedkeysfile"]; ok {
			patterns = strings.Fields(value)
		}
	}
	var paths []string
	seen := make(map[string]bool)
	for _, pattern := range patterns {
		if pattern == "none" {
			continue
		}
		pattern = strings.ReplaceAll(pattern, "%%", "\x00")
		pattern = strings.ReplaceAll(pattern, "%h", account.Home)
		pattern = strings.ReplaceAll(pattern, "%u", account.Name)
		pattern = strings.ReplaceAll(pattern, "%U", account.UID)
		pattern = strings.ReplaceAll(pattern, "\x00", "%")
		if strings.Contains(pattern, "%") {
			continue
		} // Unknown expansion.
		if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(account.Home, pattern)
		}
		if !seen[pattern] {
			seen[pattern] = true
			paths = append(paths, pattern)
		}
	}
	return paths, configErr
}

func keyType(value string) bool {
	return value == "ssh-rsa" || value == "ssh-ed25519" || value == "ssh-dss" ||
		strings.HasPrefix(value, "ecdsa-sha2-") || strings.HasPrefix(value, "sk-") ||
		strings.HasSuffix(value, "-cert-v01@openssh.com")
}

func parseKey(line string, account Account, path string, number int) (AuthorizedKey, bool) {
	fields := strings.Fields(line)
	for i := 0; i+1 < len(fields); i++ {
		if !keyType(fields[i]) {
			continue
		}
		blob, err := base64.StdEncoding.DecodeString(fields[i+1])
		if err != nil || len(blob) == 0 {
			continue
		}
		sum := sha256.Sum256(blob)
		key := AuthorizedKey{
			User: account.Name, Path: path, Line: number, Type: fields[i],
			Fingerprint: base64.RawStdEncoding.EncodeToString(sum[:]),
			Options:     strings.Join(fields[:i], " "),
			Comment:     strings.Join(fields[i+2:], " "), content: line,
		}
		return key, true
	}
	return AuthorizedKey{}, false
}

func keysInFile(path string, account Account) ([]AuthorizedKey, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var keys []AuthorizedKey
	for i, line := range strings.Split(string(data), "\n") {
		if key, ok := parseKey(line, account, path, i+1); ok {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func ListKeys(username, configPath, clientAddr string) ([]AuthorizedKey, error) {
	all, err := accounts()
	if err != nil {
		return nil, err
	}
	var result []AuthorizedKey
	found := username == ""
	warnedConfig := false
	for _, account := range all {
		if username != "" && account.Name != username {
			continue
		}
		found = true
		paths, configErr := keyPaths(account, configPath, clientAddr)
		if configErr != nil && !warnedConfig {
			slog.Warn("Could not read effective SSH key paths; using standard authorized_keys locations", "error", configErr.Error())
			warnedConfig = true
		}
		for _, path := range paths {
			keys, err := keysInFile(path, account)
			if err != nil {
				if username != "" {
					return nil, fmt.Errorf("%s: %w", path, err)
				}
				slog.Warn("Could not read authorized keys", "path", path, "error", err.Error())
				continue
			}
			result = append(result, keys...)
		}
	}
	if !found {
		return nil, fmt.Errorf("user %q not found", username)
	}
	return result, nil
}

// RemoveKey removes one exact authorized_keys entry after rechecking its
// fingerprint, line, and content. It returns a secured backup path.
func RemoveKey(expected AuthorizedKey, configPath, clientAddr string) (string, error) {
	keys, err := ListKeys(expected.User, configPath, clientAddr)
	if err != nil {
		return "", err
	}
	var match *AuthorizedKey
	for i := range keys {
		k := &keys[i]
		if k.Path == expected.Path && k.Line == expected.Line && k.Fingerprint == expected.Fingerprint && k.content == expected.content {
			match = k
			break
		}
	}
	if match == nil {
		return "", fmt.Errorf("key changed; list keys again")
	}
	return removeKeyFile(*match)
}

func removeKeyFile(key AuthorizedKey) (string, error) {
	info, err := os.Lstat(key.Path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("authorized_keys path is not a regular file")
	}
	data, err := os.ReadFile(key.Path)
	if err != nil {
		return "", err
	}
	lines := strings.SplitAfter(string(data), "\n")
	if key.Line < 1 || key.Line > len(lines) || strings.TrimSuffix(lines[key.Line-1], "\n") != key.content {
		return "", fmt.Errorf("key file changed; no removal performed")
	}
	lines = append(lines[:key.Line-1], lines[key.Line:]...)
	newData := []byte(strings.Join(lines, ""))
	backup, err := os.CreateTemp(filepath.Dir(key.Path), ".seer-keys-backup-*")
	if err != nil {
		return "", err
	}
	backupPath := backup.Name()
	if _, err := backup.Write(data); err != nil {
		backup.Close()
		os.Remove(backupPath)
		return "", err
	}
	if err := backup.Sync(); err != nil {
		backup.Close()
		os.Remove(backupPath)
		return "", err
	}
	if err := backup.Close(); err != nil {
		os.Remove(backupPath)
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(key.Path), ".seer-keys-*")
	if err != nil {
		return backupPath, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		return backupPath, err
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if int(stat.Uid) != os.Geteuid() || int(stat.Gid) != os.Getegid() {
			if err := tmp.Chown(int(stat.Uid), int(stat.Gid)); err != nil {
				return backupPath, err
			}
		}
	}
	if err := copyExtendedAttributes(key.Path, tmp.Name()); err != nil {
		return backupPath, fmt.Errorf("could not preserve key file attributes: %w", err)
	}
	if _, err := tmp.Write(newData); err != nil {
		return backupPath, err
	}
	if err := tmp.Sync(); err != nil {
		return backupPath, err
	}
	if err := tmp.Close(); err != nil {
		return backupPath, err
	}
	// Recheck the original immediately before replacement.
	latest, err := os.ReadFile(key.Path)
	if err != nil || string(latest) != string(data) {
		return backupPath, fmt.Errorf("key file changed; no removal performed")
	}
	latestInfo, err := os.Lstat(key.Path)
	if err != nil || !latestInfo.Mode().IsRegular() || !os.SameFile(info, latestInfo) {
		return backupPath, fmt.Errorf("key file identity changed; no removal performed")
	}
	if err := os.Rename(tmp.Name(), key.Path); err != nil {
		return backupPath, err
	}
	remaining, err := keysInFile(key.Path, Account{Name: key.User, UID: strconv.Itoa(os.Getuid())})
	if err != nil {
		return backupPath, err
	}
	for _, remainingKey := range remaining {
		if remainingKey.Fingerprint == key.Fingerprint && remainingKey.content == key.content {
			return backupPath, fmt.Errorf("key still present after replacement")
		}
	}
	return backupPath, nil
}
