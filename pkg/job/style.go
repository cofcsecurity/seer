package job

import (
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// Role identifies a part of a job so callers can color it.
type Role string

const (
	RoleID          Role = "id"
	RoleCommand     Role = "command"
	RoleDisabled    Role = "disabled"
	RoleUserRoot    Role = "user-root"
	RoleUserSystem  Role = "user-system"
	RoleUserRegular Role = "user-regular"
	RoleUserUnknown Role = "user-unknown"
	RoleReadOnly    Role = "read-only"
	RoleRisk        Role = "risk"
	RoleReboot      Role = "schedule-reboot"
	RoleMacro       Role = "schedule-macro"
	RoleFrequent    Role = "schedule-frequent"
	RoleStandard    Role = "schedule-standard"
)

// Style wraps text for a role; a nil Style leaves text unchanged.
type Style func(role Role, text string) string

// Apply wraps text for role; a nil Style returns text unchanged.
func (s Style) Apply(role Role, text string) string { return s.apply(role, text) }

func (s Style) apply(role Role, text string) string {
	if s == nil {
		return text
	}
	return s(role, text)
}

// UserRole classifies the owner: root, a system account, a regular login
// user, or unknown when the account can't be resolved.
func (j Job) UserRole() Role {
	roleMu.Lock()
	defer roleMu.Unlock()

	if role, ok := roleCache[j.User]; ok {
		return role
	}

	role := lookupUserRole(j.User)
	roleCache[j.User] = role

	return role
}

var (
	roleMu    sync.Mutex
	roleCache = map[string]Role{}
)

func lookupUserRole(name string) Role {
	if name == "root" {
		return RoleUserRoot
	}
	u, err := user.Lookup(name)
	if err != nil {
		return RoleUserUnknown
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return RoleUserUnknown
	}
	switch {
	case uid == 0:
		return RoleUserRoot
	case uid >= 65534 || (uid < 1000 && !isMacUser(uid)):
		return RoleUserSystem
	default:
		return RoleUserRegular
	}
}

// isMacUser reports whether uid is a regular macOS account (starts at 501).
func isMacUser(uid int) bool {
	return uid >= 500 && !linuxHost
}

// ScheduleRole classifies the schedule: @reboot, other @macros, jobs that
// run every minute or every few minutes, and ordinary fixed schedules.
func (j Job) ScheduleRole() Role {
	fields := strings.Fields(j.Schedule)
	switch {
	case len(fields) == 1 && fields[0] == "@reboot":
		return RoleReboot
	case len(fields) == 1:
		return RoleMacro
	case len(fields) == 5 && fields[0] == "*" && fields[1] == "*",
		len(fields) == 5 && strings.HasPrefix(fields[0], "*/") && fields[1] == "*":
		return RoleFrequent
	default:
		return RoleStandard
	}
}

var linuxHost = runtime.GOOS == "linux"
