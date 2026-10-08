package job

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Locations of cron's access control files. Variables so tests can redirect them.
var (
	cronAllowPath = "/etc/cron.allow"
	cronDenyPath  = "/etc/cron.deny"
)

// cronAccess holds cron.allow and cron.deny. Following crontab(1): when
// cron.allow exists only the users in it may have a crontab; otherwise users
// in cron.deny may not; root is always allowed.
type cronAccess struct {
	allow, deny       map[string]bool
	hasAllow, hasDeny bool
	reason            string
}

func loadCronAccess() cronAccess {
	a := cronAccess{}
	a.allow, a.hasAllow = readUserList(cronAllowPath)
	a.deny, a.hasDeny = readUserList(cronDenyPath)

	switch {
	case a.hasAllow:
		a.reason = fmt.Sprintf("not listed in %s, so cron may skip this crontab", cronAllowPath)
	case a.hasDeny:
		a.reason = fmt.Sprintf("listed in %s, so cron may skip this crontab", cronDenyPath)
	}

	return a
}

func (a cronAccess) denies(user string) bool {
	if user == "root" {
		return false
	}
	if a.hasAllow {
		return !a.allow[user]
	}

	return a.hasDeny && a.deny[user]
}

func readUserList(path string) (map[string]bool, bool) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer file.Close()

	users := map[string]bool{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" && !strings.HasPrefix(line, "#") {
			users[line] = true
		}
	}

	return users, true
}
