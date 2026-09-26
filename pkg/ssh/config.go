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
		for _, pattern := range fields[1:] {
			if !filepath.IsAbs(pattern) {
				pattern = filepath.Join("/etc/ssh", pattern)
			}
			matches, _ := filepath.Glob(pattern)
			sort.Strings(matches)
			for _, match := range matches {
				included, err := readConfigRules(match, seen)
				if err == nil {
					rules = append(rules, included...)
				}
			}
		}
	}
	return rules, scanner.Err()
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
	result := make(map[string]string)
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 1 {
			result[strings.ToLower(fields[0])] = strings.Join(fields[1:], " ")
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
		{"permitemptypasswords", "yes", "warning", "SSH permits accounts with empty passwords"},
		{"permituserenvironment", "yes", "warning", "users may supply SSH environment settings"},
		{"gatewayports", "yes", "review", "remote forwards may bind beyond loopback"},
		{"passwordauthentication", "yes", "review", "password login is enabled; confirm this is intended"},
		{"allowtcpforwarding", "yes", "review", "TCP forwarding is enabled; confirm this is intended"},
	}
	for _, check := range checks {
		if strings.EqualFold(effective[check.key], check.value) {
			result = append(result, ConfigFinding{check.level, check.key, effective[check.key], check.reason})
		}
	}
	return result
}
