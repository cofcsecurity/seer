package ssh

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// RunningServer describes a visible sshd listener or inetd-style instance.
// Command-line flags can be hidden by process title rewriting, so an empty
// ConfigPath does not prove that the default file was used.
type RunningServer struct {
	PID        int
	Executable string
	Command    string
	ConfigPath string
	Overrides  []string
	Mode       string
	Listeners  []string
}

func RunningServers() ([]RunningServer, error) {
	return runningServers("/proc")
}

func runningServers(root string) ([]RunningServer, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var servers []RunningServer
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		p, err := readProcess(root, pid)
		if err != nil || filepath.Base(strings.TrimSuffix(p.exe, " (deleted)")) != "sshd" {
			continue
		}
		listeners := processListeners(root, p)
		args, _ := os.ReadFile(filepath.Join(root, entry.Name(), "cmdline"))
		configPath, overrides, inetd := runtimeArguments(args)
		mode := "listener"
		if len(listeners) == 0 {
			if !inetd {
				continue
			}
			mode = "inetd"
		}
		servers = append(servers, RunningServer{
			PID: pid, Executable: p.exe, Command: p.command, ConfigPath: configPath,
			Overrides: overrides, Mode: mode, Listeners: listeners,
		})
	}
	sort.Slice(servers, func(i, j int) bool { return servers[i].PID < servers[j].PID })
	return servers, nil
}

func processListeners(root string, p process) []string {
	var result []string
	for _, table := range []struct {
		name string
		ipv6 bool
	}{{"tcp", false}, {"tcp6", true}} {
		data, err := os.ReadFile(filepath.Join(root, strconv.Itoa(p.pid), "net", table.name))
		if err != nil {
			continue
		}
		for _, s := range parseSocketTable(data, table.ipv6, p.netNS) {
			if s.state == "0A" && p.sockets[s.inode] {
				result = append(result, s.local.String())
			}
		}
	}
	sort.Strings(result)
	return result
}

func runtimeArguments(data []byte) (configPath string, overrides []string, inetd bool) {
	var args []string
	for _, item := range strings.Split(string(data), "\x00") {
		if item != "" {
			args = append(args, item)
		}
	}
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-i":
			inetd = true
		case arg == "-f" && i+1 < len(args):
			i++
			configPath = args[i]
		case strings.HasPrefix(arg, "-f") && len(arg) > 2:
			configPath = arg[2:]
		case (arg == "-o" || arg == "-p") && i+1 < len(args):
			i++
			overrides = append(overrides, fmt.Sprintf("%s %s", arg, args[i]))
		case (strings.HasPrefix(arg, "-o") || strings.HasPrefix(arg, "-p")) && len(arg) > 2:
			overrides = append(overrides, arg)
		}
	}
	return
}
