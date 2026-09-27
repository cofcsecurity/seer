package ssh

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Session describes a live SSH transport. Login records are supplementary:
// they are not required for a connection to appear here.
type Session struct {
	ID            string
	Direction     string
	Local         netip.AddrPort
	Remote        netip.AddrPort
	PID           int
	StartTime     uint64
	SocketInode   uint64
	NetNS         string
	User          string
	ProcessUser   string
	Command       string
	LoginTime     string
	Authenticated bool
	ListenerMatch bool
	SharedOwners  int
	Current       bool
	Sources       []string
}

// ScanStatus records whether missing process information may have hidden an
// SSH connection, including the connection that invoked Seer.
type ScanStatus struct {
	Incomplete bool
	Reasons    []string
}

func (s *ScanStatus) add(reason string) {
	s.Incomplete = true
	for _, existing := range s.Reasons {
		if existing == reason {
			return
		}
	}
	s.Reasons = append(s.Reasons, reason)
}

type process struct {
	pid         int
	ppid        int
	startTime   uint64
	uid         string
	exe         string
	command     string
	netNS       string
	sockets     map[uint64]bool
	fdsReadable bool
	fdReadError bool
}

type socket struct {
	local  netip.AddrPort
	remote netip.AddrPort
	inode  uint64
	netNS  string
	state  string
}

func sessionID(p process, s socket) string {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s|%s|%s", s.netNS, s.local, s.remote)
	return fmt.Sprintf("%d:%d:%d:%016x", p.pid, p.startTime, s.inode, h.Sum64())
}

func (s Session) String() string {
	current := ""
	if s.Current {
		current = " [CURRENT SESSION]"
	}
	auth := "auth unknown"
	if s.Authenticated {
		auth = "authenticated"
	}
	from, to := s.Remote, s.Local
	if s.Direction == "outbound" {
		from, to = s.Local, s.Remote
	}
	shared := ""
	if s.SharedOwners > 1 {
		shared = fmt.Sprintf(" owners:%d", s.SharedOwners)
	}
	return fmt.Sprintf("[%s] %s %s -> %s %s pid:%d %s%s%s\n",
		s.ID, s.Direction, from, to, s.User, s.PID, auth, shared, current)
}

func (s Session) Describe() string {
	return fmt.Sprintf("┌ %s (%s)\n├ Remote: %s\n├ Local: %s\n├ Login user: %s\n├ Process user: %s\n├ Login time: %s\n├ PID: %d (start ticks: %d)\n├ Network namespace: %s\n├ Socket inode: %d\n├ Socket owners: %d\n├ Command: %s\n├ Authenticated: %t\n├ SSH listener matched: %t\n├ Current session: %t\n└ Sources: %s\n",
		s.ID, s.Direction, s.Remote, s.Local, s.User, s.ProcessUser, s.LoginTime, s.PID, s.StartTime,
		s.NetNS, s.SocketInode, s.SharedOwners, s.Command, s.Authenticated, s.ListenerMatch, s.Current,
		strings.Join(s.Sources, ", "))
}

func parseAddress(value string, ipv6 bool) (netip.AddrPort, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return netip.AddrPort{}, fmt.Errorf("invalid socket address %q", value)
	}
	bytes, err := hex.DecodeString(parts[0])
	if err != nil {
		return netip.AddrPort{}, err
	}
	port, err := strconv.ParseUint(parts[1], 16, 16)
	if err != nil {
		return netip.AddrPort{}, err
	}
	if ipv6 {
		if len(bytes) != 16 {
			return netip.AddrPort{}, fmt.Errorf("invalid IPv6 socket address")
		}
		var addr [16]byte
		for i := 0; i < 16; i += 4 {
			binary.BigEndian.PutUint32(addr[i:i+4], binary.LittleEndian.Uint32(bytes[i:i+4]))
		}
		return netip.AddrPortFrom(netip.AddrFrom16(addr), uint16(port)), nil
	}
	if len(bytes) != 4 {
		return netip.AddrPort{}, fmt.Errorf("invalid IPv4 socket address")
	}
	return netip.AddrPortFrom(netip.AddrFrom4([4]byte{bytes[3], bytes[2], bytes[1], bytes[0]}), uint16(port)), nil
}

func parseSocketTable(data []byte, ipv6 bool, netNS string) []socket {
	var result []socket
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	if !scanner.Scan() { // header
		return result
	}
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			continue
		}
		local, e1 := parseAddress(fields[1], ipv6)
		remote, e2 := parseAddress(fields[2], ipv6)
		inode, e3 := strconv.ParseUint(fields[9], 10, 64)
		if e1 != nil || e2 != nil || e3 != nil {
			continue
		}
		result = append(result, socket{local: local, remote: remote, inode: inode, netNS: netNS, state: fields[3]})
	}
	return result
}

func readProcess(root string, pid int) (process, error) {
	base := filepath.Join(root, strconv.Itoa(pid))
	stat, err := os.ReadFile(filepath.Join(base, "stat"))
	if err != nil {
		return process{}, err
	}
	// comm is parenthesized and may contain spaces or closing parentheses.
	end := strings.LastIndexByte(string(stat), ')')
	if end < 0 || end+2 >= len(stat) {
		return process{}, fmt.Errorf("invalid stat for PID %d", pid)
	}
	fields := strings.Fields(string(stat[end+2:]))
	if len(fields) <= 19 {
		return process{}, fmt.Errorf("short stat for PID %d", pid)
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return process{}, err
	}
	startTime, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return process{}, err
	}
	p := process{pid: pid, ppid: ppid, startTime: startTime, sockets: make(map[uint64]bool)}
	p.netNS, _ = os.Readlink(filepath.Join(base, "ns/net"))
	p.exe, _ = os.Readlink(filepath.Join(base, "exe"))
	cmdline, _ := os.ReadFile(filepath.Join(base, "cmdline"))
	p.command = strings.TrimSpace(strings.ReplaceAll(string(cmdline), "\x00", " "))
	status, _ := os.ReadFile(filepath.Join(base, "status"))
	for _, line := range strings.Split(string(status), "\n") {
		if strings.HasPrefix(line, "Uid:") {
			items := strings.Fields(line)
			if len(items) > 1 {
				p.uid = items[1]
			}
			break
		}
	}
	fds, err := os.ReadDir(filepath.Join(base, "fd"))
	if err == nil {
		p.fdsReadable = true
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join(base, "fd", fd.Name()))
			if err != nil {
				p.fdReadError = true
				continue
			}
			if !strings.HasPrefix(target, "socket:[") || !strings.HasSuffix(target, "]") {
				continue
			}
			inode, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]"), 10, 64)
			if err == nil {
				p.sockets[inode] = true
			}
		}
	}
	return p, nil
}

func sshProcess(p process) string {
	name := filepath.Base(strings.TrimSuffix(p.exe, " (deleted)"))
	switch name {
	case "sshd", "sshd-session", "sshd-auth":
		return "inbound"
	case "ssh":
		return "outbound"
	}
	return ""
}

func connectionValue(data []byte) string {
	for _, item := range strings.Split(string(data), "\x00") {
		if strings.HasPrefix(item, "SSH_CONNECTION=") {
			return strings.TrimPrefix(item, "SSH_CONNECTION=")
		}
	}
	return ""
}

func connectionMatches(value string, s socket) bool {
	f := strings.Fields(value)
	if len(f) != 4 {
		return false
	}
	client, e1 := netip.ParseAddrPort(net.JoinHostPort(f[0], f[1]))
	server, e2 := netip.ParseAddrPort(net.JoinHostPort(f[2], f[3]))
	return e1 == nil && e2 == nil &&
		client.Port() == s.remote.Port() && client.Addr().Unmap() == s.remote.Addr().Unmap() &&
		server.Port() == s.local.Port() && server.Addr().Unmap() == s.local.Addr().Unmap()
}

func ancestorOf(candidate, self int, procs map[int]process) bool {
	seen := make(map[int]bool)
	for self > 0 && !seen[self] {
		if self == candidate {
			return true
		}
		seen[self] = true
		p, ok := procs[self]
		if !ok {
			return false
		}
		self = p.ppid
	}
	return false
}

// ListSessions returns live SSH transports visible to the caller. Reading all
// processes and namespaces is usually possible only as root.
func ListSessions() ([]Session, error) {
	sessions, _, err := ListSessionsWithStatus()
	return sessions, err
}

func ListSessionsWithStatus() ([]Session, ScanStatus, error) {
	sessions, status, err := scanSessions("/proc", os.Getpid(), os.Getenv("SSH_CONNECTION"))
	if err != nil {
		return nil, status, err
	}
	enrichSessions(sessions)
	return sessions, status, nil
}

func listSessions(root string, self int, ownConnection string) ([]Session, error) {
	sessions, _, err := scanSessions(root, self, ownConnection)
	return sessions, err
}

func scanSessions(root string, self int, ownConnection string) ([]Session, ScanStatus, error) {
	var status ScanStatus
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, status, err
	}
	procs := make(map[int]process)
	representatives := make(map[string][]int)
	unreadableFDs := 0
	unreadableNamespaces := 0
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		p, err := readProcess(root, pid)
		if err != nil {
			if os.IsPermission(err) {
				status.add("some process records were not readable")
			}
			continue // A process may exit during enumeration.
		}
		procs[pid] = p
		if sshProcess(p) != "" && (!p.fdsReadable || p.fdReadError) {
			status.add("an SSH process had unreadable file descriptors")
		}
		if sshProcess(p) != "" && p.netNS == "" {
			status.add("an SSH process had an unreadable network namespace")
		}
		if p.exe == "" && len(p.sockets) > 0 {
			status.add("a socket owner had an unreadable executable")
		}
		if !p.fdsReadable {
			unreadableFDs++
		}
		if p.netNS == "" {
			unreadableNamespaces++
		}
		if p.netNS != "" {
			representatives[p.netNS] = append(representatives[p.netNS], pid)
		}
	}
	if self > 0 {
		for pid, seen := self, make(map[int]bool); pid > 1 && !seen[pid]; {
			seen[pid] = true
			p, ok := procs[pid]
			if !ok {
				status.add("current process ancestry was incomplete")
				break
			}
			pid = p.ppid
		}
	}
	if unreadableFDs > 0 {
		slog.Warn("Some process file descriptors were not readable; SSH sessions may be missing", "processes", unreadableFDs)
	}
	if unreadableNamespaces > 0 {
		slog.Warn("Some process network namespaces were not readable; SSH sessions may be missing", "processes", unreadableNamespaces)
	}
	var sockets []socket
	for ns, pids := range representatives {
		for _, kind := range []struct {
			name string
			ipv6 bool
		}{{"tcp", false}, {"tcp6", true}} {
			read, denied := false, false
			for _, pid := range pids {
				data, err := os.ReadFile(filepath.Join(root, strconv.Itoa(pid), "net", kind.name))
				if err == nil {
					sockets = append(sockets, parseSocketTable(data, kind.ipv6, ns)...)
					read = true
					break
				}
				if os.IsPermission(err) {
					denied = true
				}
			}
			if !read && denied {
				status.add("a TCP socket table was not readable")
			}
			if !read && kind.name == "tcp" {
				for _, pid := range pids {
					if sshProcess(procs[pid]) != "" {
						status.add("an SSH network namespace had no readable TCP table")
						break
					}
				}
			}
		}
	}
	owners := make(map[string][]process)
	for _, p := range procs {
		for inode := range p.sockets {
			key := fmt.Sprintf("%s:%d", p.netNS, inode)
			owners[key] = append(owners[key], p)
		}
	}
	listeners := make(map[string]bool)
	for _, sock := range sockets {
		if sock.state != "0A" {
			continue
		}
		key := fmt.Sprintf("%s:%d", sock.netNS, sock.inode)
		for _, p := range owners[key] {
			if sshProcess(p) == "inbound" {
				listeners[fmt.Sprintf("%s:%d", sock.netNS, sock.local.Port())] = true
			}
		}
	}
	var result []Session
	for _, sock := range sockets {
		if sock.state != "01" { // TCP_ESTABLISHED
			continue
		}
		key := fmt.Sprintf("%s:%d", sock.netNS, sock.inode)
		ownerCount := len(owners[key])
		for _, p := range owners[key] {
			direction := sshProcess(p)
			if direction == "" {
				continue
			}
			listenerMatch := direction == "inbound" && listeners[fmt.Sprintf("%s:%d", sock.netNS, sock.local.Port())]
			uid, _ := user.LookupId(p.uid)
			processUser := p.uid
			if uid != nil {
				processUser = uid.Username
			}
			username := "unknown"
			authenticated := false
			for _, child := range procs {
				if child.pid != p.pid && ancestorOf(p.pid, child.pid, procs) {
					env, err := os.ReadFile(filepath.Join(root, strconv.Itoa(child.pid), "environ"))
					if err == nil && connectionMatches(connectionValue(env), sock) {
						authenticated = true
						if child.uid != "" {
							if u, err := user.LookupId(child.uid); err == nil {
								username = u.Username
							} else {
								username = child.uid
							}
						}
						break
					}
				}
			}
			current := direction == "inbound" &&
				(connectionMatches(ownConnection, sock) || (ownConnection == "" && ancestorOf(p.pid, self, procs)))
			sources := []string{"TCP socket", "/proc/PID/fd", "/proc/PID/stat"}
			if authenticated {
				sources = append(sources, "child SSH_CONNECTION")
			}
			if listenerMatch {
				sources = append(sources, "sshd listening socket")
			}
			if current {
				sources = append(sources, "current process ancestry or SSH_CONNECTION")
			}
			result = append(result, Session{
				ID:        sessionID(p, sock),
				Direction: direction, Local: sock.local, Remote: sock.remote,
				PID: p.pid, StartTime: p.startTime, SocketInode: sock.inode,
				NetNS: sock.netNS, User: username, ProcessUser: processUser, Command: p.command,
				Authenticated: authenticated, ListenerMatch: listenerMatch, SharedOwners: ownerCount,
				Current: current, Sources: sources,
			})
		}
	}
	// A privileged monitor and its child can both hold one SSH socket.
	// Display one transport while retaining the owner count for action safety.
	unique := make(map[string]Session)
	for _, s := range result {
		key := fmt.Sprintf("%s:%d", s.NetNS, s.SocketInode)
		previous, exists := unique[key]
		if !exists || ancestorOf(s.PID, previous.PID, procs) {
			if exists {
				if !s.Authenticated && previous.Authenticated {
					s.User = previous.User
				}
				s.Current = s.Current || previous.Current
				s.Authenticated = s.Authenticated || previous.Authenticated
				for _, source := range previous.Sources {
					addSource(&s, source)
				}
			}
			unique[key] = s
		} else {
			if !previous.Authenticated && s.Authenticated {
				previous.User = s.User
			}
			previous.Current = previous.Current || s.Current
			previous.Authenticated = previous.Authenticated || s.Authenticated
			for _, source := range s.Sources {
				addSource(&previous, source)
			}
			unique[key] = previous
		}
	}
	result = result[:0]
	for _, s := range unique {
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].PID == result[j].PID {
			return result[i].SocketInode < result[j].SocketInode
		}
		return result[i].PID < result[j].PID
	})
	return result, status, nil
}
