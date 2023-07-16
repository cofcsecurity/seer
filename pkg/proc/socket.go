package proc

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

// Socket States
// https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/tree/include/net/tcp_states.h

type State int

const (
	ESTABLISHED State = iota + 1
	SYN_SENT
	SYN_RECV
	FIN_WAIT1
	FIN_WAIT2
	TIME_WAIT
	CLOSE
	CLOSE_WAIT
	LAST_ACK
	LISTEN
	CLOSING
	NEW_SYN_RECV
)

func (s State) String() string {
	switch s {
	case ESTABLISHED:
		return "ESTABLISHED"
	case SYN_SENT:
		return "SYN_SENT"
	case SYN_RECV:
		return "SYN_RECV"
	case FIN_WAIT1:
		return "FIN_WAIT1"
	case FIN_WAIT2:
		return "FIN_WAIT2"
	case TIME_WAIT:
		return "TIME_WAIT"
	case CLOSE:
		return "CLOSE"
	case CLOSE_WAIT:
		return "CLOSE_WAIT"
	case LAST_ACK:
		return "LAST_ACK"
	case LISTEN:
		return "LISTEN"
	case CLOSING:
		return "CLOSING"
	case NEW_SYN_RECV:
		return "NEW_SYN_RECV"
	}
	return "?"
}

// https://www.kernel.org/doc/html/v6.2/networking/proc_net_tcp.html
type Socket struct {
	Protocol string // tcp, udp, raw, ...

	// Fields from /proc/net/*
	Sl           int // Slot number (in the socket hashtable)
	Local_addr   string
	Local_port   int
	Remote_addr  string
	Remote_port  int
	State        string
	Tx_queue     int // Size of the transmit queue in bytes
	Rx_queue     int // Size of the receive queue in bytes
	Timer_active int // 0 - no timer; 1,2,4 - timer pending; 3 - socket waiting
	Tm_when      int // Jiffies until timer expires
	Retrnsmt     int // Number of unrecovered RTO timeouts
	Uid          int
	Timeout      int // unanswered 0-window probes
	Inode        int
	References   int // Socket reference count
	Location     int // Address of the socket in memory
	// ... (Don't care about the rest)
}

func (s Socket) String() string {
	state, _ := strconv.ParseInt(s.State, 16, 0)

	arrow := "->"
	if State(state).String() == "LISTEN" {
		arrow = "<-"
	}

	return fmt.Sprintf("<%d> %s %s:%d %s %s:%d (%s) i:%d\n",
		s.Sl,
		s.Protocol,
		s.Local_addr, s.Local_port,
		arrow,
		s.Remote_addr, s.Remote_port,
		State(state).String(),
		s.Inode)
}

func (s Socket) Describe() string {
	desc := "┌[%d] (%s)\n"
	desc += "├ Local: %s:%d\n"
	desc += "├ Remote: %s:%d\n"
	desc += "├ State: %s\n"
	desc += "├ Inode: %d\n"
	desc += "├ References: %d\n"
	desc += "└ Location: %d\n"

	state, _ := strconv.ParseInt(s.State, 16, 0)

	return fmt.Sprintf(desc,
		s.Sl,
		s.Protocol,
		s.Local_addr, s.Local_port,
		s.Remote_addr, s.Remote_port,
		State(state).String(),
		s.Inode,
		s.References,
		s.Location)
}

func decodeAddr(hexAddr string) (ip string, port int) {
	ipHex := strings.Split(hexAddr, ":")[0]
	ip = ""
	for i := 0; i < len(ipHex); i += 2 {
		ipBytes, _ := hex.DecodeString(ipHex[i : i+2])
		ip = fmt.Sprintf("%d.", int(ipBytes[0])) + ip
	}
	ip = ip[:len(ip)-1]

	portBytes, _ := hex.DecodeString(strings.Split(hexAddr, ":")[1])
	port = int(binary.BigEndian.Uint16(portBytes))

	return ip, port
}

func GetSockets() (sockets []Socket) {
	// TODO: Handle ipv6
	// TODO: /proc/net/icmp seems to provide mostly useless data
	protocols := []string{"tcp", "udp", "udplite", "icmp", "raw"}

	for _, proto := range protocols {
		path := fmt.Sprintf("/proc/net/%s", proto)
		contents, err := os.ReadFile(path)
		if err != nil {
			log.Printf("Warning: Failed to read %s\n", path)
			continue
		}
		for n, line := range strings.Split(string(contents), "\n") {
			// Skip header and empty lines
			if n == 0 || strings.TrimSpace(line) == "" {
				continue
			}

			//fmt.Printf("%s socket: <%s>\n", proto, line)

			sock_data := make([]string, 0)
			for _, f := range strings.Split(line, " ") {
				if f == "" {
					continue
				}
				sock_data = append(sock_data, f)
			}

			// Map data into Socket

			// Example (tcp):
			// sl local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
			// 0: 00000000:002A 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 78428 1 0000000000000000 100 0 0 10 0

			socket := Socket{}
			socket.Protocol = proto

			socket.Sl, _ = strconv.Atoi(strings.Split(sock_data[0], ":")[0])
			socket.Local_addr, socket.Local_port = decodeAddr(sock_data[1])
			socket.Remote_addr, socket.Remote_port = decodeAddr(sock_data[2])
			socket.State = sock_data[3]
			socket.Tx_queue, _ = strconv.Atoi(strings.Split(sock_data[4], ":")[0])
			socket.Rx_queue, _ = strconv.Atoi(strings.Split(sock_data[4], ":")[1])
			socket.Timer_active, _ = strconv.Atoi(strings.Split(sock_data[5], ":")[0])
			socket.Tm_when, _ = strconv.Atoi(strings.Split(sock_data[5], ":")[1])
			socket.Retrnsmt, _ = strconv.Atoi(sock_data[6])
			socket.Uid, _ = strconv.Atoi(sock_data[7])
			socket.Timeout, _ = strconv.Atoi(sock_data[8])
			socket.Inode, _ = strconv.Atoi(sock_data[9])
			socket.References, _ = strconv.Atoi(sock_data[10])
			socket.Location, _ = strconv.Atoi(sock_data[11])

			sockets = append(sockets, socket)
		}
	}

	return sockets
}
