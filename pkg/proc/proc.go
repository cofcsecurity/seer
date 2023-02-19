package proc

import (
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
)

type Process struct {
	// stats read from /proc/<pid>/stat
	Pid         int    // process id
	Comm        string // executable name
	State       rune   // process state
	Ppid        int    // parent process id
	Pgrp        int    // process group id
	Session     int    // session id
	Tty_nr      int    // controlling terminal
	Tpgid       int    // id of process group controlling tty
	Flags       uint   // kernel flags
	Minflt      uint64 // number of minor faults
	Cminflt     uint64 // number of minor faults by children
	Majflt      uint64 // number of major faults
	Cmajflt     uint64 // number of major faults by children
	Utime       uint64 // clock ticks proc has been scheduled in user mode
	Stime       uint64 // clock ticks proc has been scheduled in kernel mode
	Cutime      int64  // clock ticks children have been scheduled in user mode
	Cstime      int64  // clock ticks children have been scheduled in kernel mode
	Priority    int64  // scheduling priority
	Nice        int64  // the nice value
	Num_threads int64  // number of threads in the proc
	Itrealvalue int64  // jiffies before the next SIGALRM
	Starttime   uint64 // clock ticks since boot at proc start
	Vsize       uint64 // virtual memory size in bytes

	// other stats
	Exelink string // link to the executable
	Exesum  string // md5sum of the executable in memory
	Exedel  bool   // true if exe has been deleted from disk

	Cmdline string // command line arguments

	Uid  int // Real id of the user who started the process
	Euid int // Effective user id
	Suid int // Saved set user id
	Fuid int // Filesystem user id

	// fds/sockets
	//sockets []Socket // Sockets related to the process
	//fds []Fd // Open file descriptors
}

func (p Process) Describe() string {
	desc := "┌ [%d] (%s)\n"
	desc += "├ state: %c tty: %d session: %d ppid: %d\n"
	desc += "├ uid: %d euid: %d\n"
	desc += "└ link: %s md5: %s\n"

	return fmt.Sprintf(desc,
		p.Pid,
		p.Comm,
		p.State,
		p.Tty_nr,
		p.Session,
		p.Ppid,
		p.Uid,
		p.Euid,
		p.Exelink,
		p.Exesum,
	)
}

func GetProcess(pid int) (Process, error) {
	proc := Process{Pid: pid}
	procDir := fmt.Sprintf("/proc/%d", pid)

	if _, e := os.Stat(procDir); os.IsNotExist(e) {
		return proc, errors.New("Process does not exist")
	}

	// Read data from /proc/[pid]/stat

	statFile := procDir + "/stat"
	statData, _ := os.ReadFile(statFile)
	statStr := string(statData)

	// Read comm then slice past it
	// (File names can cause issues if comm is handled with Sscanf)
	commStart := strings.IndexRune(statStr, '(')
	commEnd := strings.LastIndex(statStr, ")")
	proc.Comm = statStr[commStart+1 : commEnd]
	statStr = statStr[commEnd+2:]
	fmtStr := "%c %d %d %d %d %d %d %d %d %d %d %d %d %d %d %d %d %d %d %d %d"
	//fmt.Printf("stat: %s\n", statStr)

	fmt.Sscanf(
		statStr,
		fmtStr,
		&proc.State,
		&proc.Ppid,
		&proc.Pgrp,
		&proc.Session,
		&proc.Tty_nr,
		&proc.Tpgid,
		&proc.Flags,
		&proc.Minflt,
		&proc.Cminflt,
		&proc.Majflt,
		&proc.Cmajflt,
		&proc.Utime,
		&proc.Stime,
		&proc.Cutime,
		&proc.Cstime,
		&proc.Priority,
		&proc.Nice,
		&proc.Num_threads,
		&proc.Itrealvalue,
		&proc.Starttime,
		&proc.Vsize)

	// Read /proc/[pid]/exe (often requires root)

	exeFile := procDir + "/exe"

	linkData, _ := os.Readlink(exeFile)
	proc.Exelink = linkData

	proc.Exedel = strings.Contains(linkData, "(deleted)")

	// Get the md5sum of the in memory executable
	exeData, _ := os.Open(exeFile)

	h := md5.New()
	if _, e := io.Copy(h, exeData); e == nil {
		sum := fmt.Sprintf("%x", h.Sum(nil))
		proc.Exesum = sum
	}

	// Read /proc/[pid]/cmdline

	cmdFile := procDir + "/cmdline"
	cmdData, _ := os.ReadFile(cmdFile)
	proc.Cmdline = string(cmdData)

	// Read UID info from /proc/[pid]/status

	statusFile := procDir + "/status"
	statusData, _ := os.ReadFile(statusFile)
	statusStr := string(statusData)
	uidStart := strings.Index(statusStr, "Uid:")
	uidEnd := strings.IndexRune(statusStr[uidStart:], '\n')
	fmt.Sscanf(
		statusStr[uidStart+4:uidStart+uidEnd],
		"%d %d %d %d",
		&proc.Uid,
		&proc.Euid,
		&proc.Suid,
		&proc.Fuid)

	return proc, nil
}

func GetProcesses() map[int]Process {
	procs := make(map[int]Process)
	contents, e := os.ReadDir("/proc")
	if e != nil {
		log.Print(e.Error())
	}
	for _, entry := range contents {
		ename := entry.Name()
		if ename[0] < '0' || ename[0] > '9' {
			continue
		}

		id, _ := strconv.Atoi(ename)
		proc, _ := GetProcess(id)
		procs[proc.Pid] = proc
	}

	return procs
}
