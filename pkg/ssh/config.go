package ssh

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type ConfigRule struct {
	Path  string
	Line  int
	Key   string
	Value string
}

type ConfigFinding struct {
	Level  string
	Key    string
	Value  string
	Reason string
}

func DefaultConfigPath() string {
	return "/etc/ssh/sshd_config"
}

func readConfigRules(path string, seen map[string]bool) ([]ConfigRule, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if seen[abs] {
		return nil, nil
	}
	seen[abs] = true
	defer delete(seen, abs)
	file, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var rules []ConfigRule
	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		value := strings.TrimSpace(scanner.Text())
		if value == "" || strings.HasPrefix(value, "#") {
			continue
		}
		fields := strings.Fields(value)
		if len(fields) < 2 {
			continue
		}
		key := fields[0]
		rule := ConfigRule{Path: abs, Line: line, Key: key, Value: strings.TrimSpace(value[len(key):])}
		rules = append(rules, rule)
		if !strings.EqualFold(key, "Include") {
			continue
		}
		patterns, err := configArguments(rule.Value)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: invalid Include arguments: %w", abs, line, err)
		}
		for _, pattern := range patterns {
			if !filepath.IsAbs(pattern) {
				pattern = filepath.Join("/etc/ssh", pattern)
			}
			matches, err := filepath.Glob(pattern)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: invalid Include pattern %q: %w", abs, line, pattern, err)
			}
			sort.Strings(matches)
			for _, match := range matches {
				included, err := readConfigRules(match, seen)
				if err != nil {
					return nil, fmt.Errorf("%s:%d: could not read Include %q: %w", abs, line, match, err)
				}
				rules = append(rules, included...)
			}
		}
	}
	return rules, scanner.Err()
}

// configArguments handles the quoted paths accepted by sshd_config Include.
// It is only used to locate source files; sshd remains authoritative for
// validation and effective rule evaluation.
func configArguments(value string) ([]string, error) {
	var args []string
	var current strings.Builder
	quoted, escaped, started := false, false, false
	for _, r := range value {
		if escaped {
			current.WriteRune(r)
			escaped = false
			started = true
			continue
		}
		switch r {
		case '\\':
			escaped = true
		case '"':
			quoted = !quoted
			started = true
		case ' ', '\t':
			if quoted {
				current.WriteRune(r)
			} else if started {
				args = append(args, current.String())
				current.Reset()
				started = false
			}
		default:
			current.WriteRune(r)
			started = true
		}
	}
	if quoted || escaped {
		return nil, fmt.Errorf("unterminated quote or escape")
	}
	if started {
		args = append(args, current.String())
	}
	return args, nil
}

func ConfigRules(path string) ([]ConfigRule, error) {
	return readConfigRules(path, make(map[string]bool))
}

func findSSHD() (string, error) {
	for _, path := range []string{"/usr/sbin/sshd", "/sbin/sshd", "/usr/local/sbin/sshd"} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	if path, err := exec.LookPath("sshd"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("sshd executable not found")
}

func EffectiveConfig(path string, criteria map[string]string) (map[string]string, error) {
	values, err := EffectiveConfigValues(path, criteria)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(values))
	for key, entries := range values {
		result[key] = entries[len(entries)-1]
	}
	return result, nil
}

// EffectiveConfigValues retains repeated settings such as HostKey.
func EffectiveConfigValues(path string, criteria map[string]string) (map[string][]string, error) {
	sshd, err := findSSHD()
	if err != nil {
		return nil, err
	}
	args := []string{"-T", "-f", path}
	var c []string
	for _, key := range []string{"user", "addr", "host", "laddr", "lport"} {
		if value := criteria[key]; value != "" {
			c = append(c, key+"="+value)
		}
	}
	if len(c) > 0 {
		args = append(args, "-C", strings.Join(c, ","))
	}
	output, err := exec.Command(sshd, args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("sshd -T failed: %s: %w", strings.TrimSpace(string(output)), err)
	}
	result := make(map[string][]string)
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 1 {
			key := strings.ToLower(fields[0])
			result[key] = append(result[key], strings.Join(fields[1:], " "))
		}
	}
	return result, nil
}

func ValidateConfig(path string) error {
	sshd, err := findSSHD()
	if err != nil {
		return err
	}
	output, err := exec.Command(sshd, "-t", "-f", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("sshd -t failed: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func CheckConfig(effective map[string]string) []ConfigFinding {
	var result []ConfigFinding
	checks := []struct{ key, value, level, reason string }{
		{"permitrootlogin", "yes", "warning", "direct root SSH login is permitted"},
		{"permitrootlogin", "prohibit-password", "review", "root SSH login by public key is permitted"},
		{"permitrootlogin", "without-password", "review", "root SSH login by public key is permitted"},
		{"permitemptypasswords", "yes", "warning", "SSH permits accounts with empty passwords"},
		{"permituserenvironment", "yes", "warning", "users may supply SSH environment settings"},
		{"gatewayports", "yes", "review", "remote forwards may bind beyond loopback"},
		{"gatewayports", "clientspecified", "review", "clients may choose remote forward bind addresses"},
		{"passwordauthentication", "yes", "review", "password login is enabled; confirm this is intended"},
		{"kbdinteractiveauthentication", "yes", "review", "keyboard-interactive login is enabled"},
		{"allowtcpforwarding", "yes", "review", "TCP forwarding is enabled; confirm this is intended"},
		{"allowtcpforwarding", "all", "review", "TCP forwarding is enabled; confirm this is intended"},
		{"allowtcpforwarding", "local", "review", "local TCP forwarding is enabled"},
		{"allowtcpforwarding", "remote", "review", "remote TCP forwarding is enabled"},
		{"allowagentforwarding", "yes", "review", "agent forwarding is enabled"},
		{"x11forwarding", "yes", "review", "X11 forwarding is enabled"},
		{"permittunnel", "yes", "review", "SSH tunnel devices are permitted"},
		{"permittunnel", "point-to-point", "review", "point-to-point SSH tunnels are permitted"},
		{"permittunnel", "ethernet", "review", "Ethernet SSH tunnels are permitted"},
		{"permituserrc", "yes", "review", "user SSH rc scripts are permitted"},
		{"hostbasedauthentication", "yes", "review", "host-based authentication is enabled"},
	}
	for _, check := range checks {
		if strings.EqualFold(effective[check.key], check.value) {
			result = append(result, ConfigFinding{check.level, check.key, effective[check.key], check.reason})
		}
	}
	return result
}
