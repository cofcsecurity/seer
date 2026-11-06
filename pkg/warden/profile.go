package warden

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Tiers and classes in Warden's host profile.
const (
	TierConfig   = "config"
	TierData     = "data"
	AutoRestore  = "auto-restore"
	ConfirmFirst = "confirm-first"
)

// ProfileEntry is one path in Warden's host profile (/var/lib/<name>/profile.json).
// Path is exact, a single-level glob, or a dir/** tree. Services are systemd
// unit candidates; Warden reloads whichever exists after a restore.
type ProfileEntry struct {
	Path     string   `json:"path"`
	Tier     string   `json:"tier"`
	Class    string   `json:"class"`
	Services []string `json:"services,omitempty"`
}

// templateEntry is a role's suggestion, with the reason shown to the team.
type templateEntry struct {
	ProfileEntry
	Why string
	// weak entries ship on many boxes whether or not the service is used
	// (/etc/aliases, an empty /etc/exports). They never make a role apply on
	// their own in auto mode.
	weak bool
}

func weak(e templateEntry) templateEntry {
	e.weak = true
	return e
}

// Role is a template of the files that matter on one kind of box.
type Role struct {
	Name    string
	Summary string
	entries []templateEntry
	// discover finds role paths that have no fixed location, such as an
	// application directory or the folders a share serves.
	discover func() []templateEntry
}

func cfg(path, why string, services ...string) templateEntry {
	return templateEntry{ProfileEntry: ProfileEntry{Path: path, Tier: TierConfig, Class: AutoRestore, Services: services}, Why: why}
}

func cfgConfirm(path, why string, services ...string) templateEntry {
	return templateEntry{ProfileEntry: ProfileEntry{Path: path, Tier: TierConfig, Class: ConfirmFirst, Services: services}, Why: why}
}

func data(path, class, why string, services ...string) templateEntry {
	return templateEntry{ProfileEntry: ProfileEntry{Path: path, Tier: TierData, Class: class, Services: services}, Why: why}
}

// roles are the templates, in the order they are shown. "base" applies to
// every box. Warden's built-in list already covers accounts, sshd, sudoers,
// cron, units and the main config of common services; these add what it
// misses on typical competition images.
var roles = []Role{
	{
		Name: "base", Summary: "every box: login stack, login scripts, name resolution, firewall",
		entries: []templateEntry{
			cfg("/etc/pam.d/*", "authentication stack; one added line can let anyone in"),
			cfg("/etc/security/*.conf", "PAM access and limits rules"),
			cfg("/etc/sudoers.d/*", "sudo drop-ins, a common backdoor spot"),
			cfg("/etc/profile", "runs at every login"),
			cfg("/etc/profile.d/*", "runs at every login, a common persistence spot"),
			cfg("/etc/bash.bashrc", "runs for every interactive shell"),
			cfg("/etc/bashrc", "runs for every interactive shell"),
			cfg("/etc/rc.local", "runs at boot"),
			cfg("/etc/resolv.conf", "DNS servers; changing it redirects every lookup"),
			cfg("/etc/hosts.allow", "TCP wrappers allow list"),
			cfg("/etc/hosts.deny", "TCP wrappers deny list"),
			// Firewalls are confirm-first: the team and the scorer change them
			// on purpose, so Warden flags drift instead of undoing it.
			cfgConfirm("/etc/ufw/*.rules", "ufw rules", "ufw"),
			cfgConfirm("/etc/ufw/ufw.conf", "ufw on/off", "ufw"),
			cfgConfirm("/etc/firewalld/firewalld.conf", "firewalld settings", "firewalld"),
			cfgConfirm("/etc/firewalld/zones/*.xml", "firewalld zones", "firewalld"),
			cfgConfirm("/etc/nftables.conf", "nftables rules", "nftables"),
			cfgConfirm("/etc/sysconfig/iptables", "saved iptables rules", "iptables"),
			cfgConfirm("/etc/sysconfig/ip6tables", "saved ip6tables rules", "ip6tables"),
			cfgConfirm("/etc/iptables/rules.v4", "saved iptables rules", "netfilter-persistent"),
			cfgConfirm("/etc/iptables/rules.v6", "saved ip6tables rules", "netfilter-persistent"),
		},
	},
	{
		Name: "dns", Summary: "dnsmasq, BIND, unbound",
		entries: []templateEntry{
			cfg("/etc/dnsmasq.conf", "dnsmasq config", "dnsmasq"),
			cfg("/etc/dnsmasq.d/*", "dnsmasq drop-ins", "dnsmasq"),
			cfg("/etc/bind/**", "BIND config and zones", "bind9", "named"),
			cfg("/etc/named.conf", "BIND config", "named"),
			cfg("/etc/named/**", "BIND includes", "named"),
			cfg("/var/named/*", "BIND zone files", "named"),
			cfg("/etc/unbound/**", "unbound config", "unbound"),
		},
	},
	{
		Name: "web", Summary: "nginx, Apache, PHP, web roots, Python web apps",
		entries: []templateEntry{
			cfg("/etc/nginx/**", "nginx config and sites", "nginx"),
			cfg("/etc/apache2/**", "Apache config and sites", "apache2"),
			cfg("/etc/httpd/**", "Apache config and sites", "httpd"),
			cfg("/etc/php/**", "PHP settings"),
			data("/var/www/**", AutoRestore, "web root; defacement is put back"),
			data("/srv/www/**", AutoRestore, "web root; defacement is put back"),
		},
		discover: discoverWebApps,
	},
	{
		Name: "db", Summary: "MySQL/MariaDB, PostgreSQL drop-ins and settings (not live data files)",
		entries: []templateEntry{
			cfg("/etc/my.cnf.d/*", "MySQL/MariaDB server settings", "mariadb", "mysqld", "mysql"),
			cfg("/etc/mysql/**", "MySQL/MariaDB config", "mysql", "mariadb"),
			cfg("/etc/postgresql/**", "PostgreSQL config and pg_hba", "postgresql"),
			cfg("/var/lib/pgsql/data/*.conf", "PostgreSQL config and pg_hba", "postgresql"),
			cfg("/etc/redis/**", "Redis config", "redis-server", "redis"),
		},
	},
	{
		Name: "mail", Summary: "Postfix, Dovecot, Sendmail",
		entries: []templateEntry{
			cfg("/etc/postfix/**", "Postfix config and maps", "postfix"),
			cfg("/etc/dovecot/**", "Dovecot config", "dovecot"),
			weak(cfg("/etc/aliases", "mail aliases; a forward here leaks mail", "postfix")),
			cfg("/etc/mail/**", "Sendmail config", "sendmail"),
		},
	},
	{
		Name: "fileshare", Summary: "Samba, NFS, FTP, and the folders they share",
		entries: []templateEntry{
			cfg("/etc/samba/**", "Samba config", "smbd", "smb", "nmbd"),
			weak(cfg("/etc/exports", "NFS exports", "nfs-server")),
			cfg("/etc/exports.d/*", "NFS exports", "nfs-server"),
			cfg("/etc/vsftpd.conf", "vsftpd config", "vsftpd"),
			cfg("/etc/vsftpd/**", "vsftpd config", "vsftpd"),
			cfg("/etc/proftpd/**", "ProFTPD config", "proftpd"),
		},
		discover: discoverShares,
	},
}

// RoleNames lists the templates, base first.
func RoleNames() []string {
	names := make([]string, len(roles))
	for i, r := range roles {
		names[i] = r.Name
	}
	return names
}

// Roles returns the templates for help text.
func Roles() []Role { return roles }

// ProposedEntry is a profile entry Seer suggests, with its role and reason.
type ProposedEntry struct {
	ProfileEntry
	Role string
	Why  string
}

// ProposeProfile returns the template entries whose files exist on this box.
// With "auto" (or no roles), every role with something present applies.
// base always applies.
func ProposeProfile(names []string) ([]ProposedEntry, []string, error) {
	want := map[string]bool{"base": true}
	auto := len(names) == 0
	for _, n := range names {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == "auto" {
			auto = true
			continue
		}
		if !hasRole(n) {
			return nil, nil, fmt.Errorf("unknown role %q: use auto or %s", n, strings.Join(RoleNames(), ", "))
		}
		want[n] = true
	}

	var out []ProposedEntry
	var applied []string
	seen := map[string]bool{}
	for _, r := range roles {
		if !want[r.Name] && !auto {
			continue
		}
		candidates := append([]templateEntry(nil), r.entries...)
		if r.discover != nil {
			candidates = append(candidates, r.discover()...)
		}
		var found []ProposedEntry
		strong := false
		for _, e := range candidates {
			if seen[e.Path] || !present(e.Path) {
				continue
			}
			strong = strong || !e.weak
			found = append(found, ProposedEntry{ProfileEntry: e.ProfileEntry, Role: r.Name, Why: e.Why})
		}
		if !strong && !want[r.Name] {
			continue // only incidental files: the service is not on this box
		}
		for _, e := range found {
			seen[e.Path] = true
		}
		applied = append(applied, r.Name)
		out = append(out, found...)
	}
	return out, applied, nil
}

// CheckRoles reports an unknown role name before any work starts.
func CheckRoles(names []string) error {
	for _, n := range names {
		n = strings.ToLower(strings.TrimSpace(n))
		if n != "auto" && !hasRole(n) {
			return fmt.Errorf("unknown role %q: use auto or %s", n, strings.Join(RoleNames(), ", "))
		}
	}
	return nil
}

func hasRole(name string) bool {
	for _, r := range roles {
		if r.Name == name {
			return true
		}
	}
	return false
}

// present reports whether a profile path matches something on this box.
// Symlinks are skipped (resolv.conf is often a link into /run, which Warden
// should not snapshot).
func present(p string) bool {
	if strings.HasSuffix(p, "/**") {
		fi, err := os.Lstat(path(strings.TrimSuffix(p, "/**")))
		return err == nil && fi.IsDir()
	}
	if strings.ContainsAny(p, "*?[") {
		matches, _ := filepath.Glob(path(p))
		for _, m := range matches {
			if fi, err := os.Lstat(m); err == nil && fi.Mode().IsRegular() {
				return true
			}
		}
		return false
	}
	fi, err := os.Lstat(path(p))
	return err == nil && fi.Mode().IsRegular()
}

// appRoots are where Python web apps usually live on competition images.
var appRoots = []string{"/opt", "/srv", "/home", "/var/www"}

// skipDirs are never application code worth watching.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "venv": true, ".venv": true, "env": true,
	"__pycache__": true, "site-packages": true, ".cache": true, "static": true, "media": true,
}

// discoverWebApps finds Django and Flask apps by their manage.py or wsgi.py,
// up to four levels under the usual roots. The app directory is data,
// confirm-first: it may hold a SQLite database or uploads that change on
// purpose, which auto-restore would roll back.
func discoverWebApps() []templateEntry {
	var out []templateEntry
	seen := map[string]bool{}
	for _, r := range appRoots {
		base := path(r)
		filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(base, p)
			depth := len(strings.Split(rel, string(filepath.Separator)))
			if d.IsDir() {
				if skipDirs[d.Name()] || depth > 4 {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Name() != "manage.py" && d.Name() != "wsgi.py" && d.Name() != "app.py" {
				return nil
			}
			dir := filepath.Dir(p)
			if d.Name() == "wsgi.py" && fileExists(filepath.Join(filepath.Dir(dir), "manage.py")) {
				dir = filepath.Dir(dir) // a Django project's inner package
			}
			if d.Name() == "app.py" && !looksLikeFlask(p) {
				return nil
			}
			abs := "/" + strings.TrimPrefix(strings.TrimPrefix(dir, path("")), "/")
			if seen[abs] || strings.HasPrefix(abs, "/var/www") {
				return nil // web roots are already covered as a whole
			}
			seen[abs] = true
			out = append(out, data(abs+"/**", ConfirmFirst, "Python web app found by its "+d.Name()))
			return nil
		})
	}
	return out
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func looksLikeFlask(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for n := 0; s.Scan() && n < 200; n++ {
		if strings.Contains(s.Text(), "Flask(") || strings.Contains(s.Text(), "from flask") {
			return true
		}
	}
	return false
}

// discoverShares reads the path of each share in smb.conf. Share contents
// are data, confirm-first: users change them on purpose.
func discoverShares() []templateEntry {
	f, err := os.Open(path("/etc/samba/smb.conf"))
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []templateEntry
	section := ""
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.Trim(line, "[]"))
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.ToLower(strings.TrimSpace(key)) != "path" {
			continue
		}
		dir := filepath.Clean(strings.TrimSpace(value))
		// [homes] and [printers] paths are per-user or spool, not shared data.
		if section == "global" || section == "homes" || section == "printers" || !filepath.IsAbs(dir) || strings.Contains(dir, "%") {
			continue
		}
		out = append(out, data(dir+"/**", ConfirmFirst, "files shared as ["+section+"]"))
	}
	return out
}

// ProfilePath is where an install reads its host profile.
func (in Install) ProfilePath() string { return filepath.Join(in.DataDir, "profile.json") }

// MergeProfile adds the proposed entries to the profile at file, keeping
// everything already there; a path the team already listed is left as they
// set it. It returns the entries added and the new file content.
func MergeProfile(file string, proposed []ProposedEntry) ([]ProposedEntry, []byte, error) {
	doc := map[string]json.RawMessage{}
	var existing []json.RawMessage
	raw, err := os.ReadFile(file)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, nil, fmt.Errorf("%s is not valid JSON (%v); fix it or move it aside, then run this again", file, err)
		}
		if p, ok := doc["paths"]; ok {
			if err := json.Unmarshal(p, &existing); err != nil {
				return nil, nil, fmt.Errorf("%s: \"paths\" is not a list: %v", file, err)
			}
		}
	case !os.IsNotExist(err):
		return nil, nil, err
	}

	have := map[string]bool{}
	for _, e := range existing {
		var p struct {
			Path string `json:"path"`
		}
		json.Unmarshal(e, &p)
		have[p.Path] = true
	}
	var added []ProposedEntry
	for _, e := range proposed {
		if have[e.Path] {
			continue
		}
		have[e.Path] = true
		b, err := json.Marshal(e.ProfileEntry)
		if err != nil {
			return nil, nil, err
		}
		existing = append(existing, b)
		added = append(added, e)
	}
	paths, err := json.Marshal(existing)
	if err != nil {
		return nil, nil, err
	}
	doc["paths"] = paths
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return added, append(out, '\n'), nil
}

// WriteProfile replaces file with content atomically, keeping the previous
// version as file.bak. It returns the backup path, or "" when there was none.
func WriteProfile(file string, content []byte) (string, error) {
	backup := ""
	if old, err := os.ReadFile(file); err == nil {
		backup = file + ".bak"
		if err := os.WriteFile(backup, old, 0o600); err != nil {
			return "", fmt.Errorf("backing up %s: %w", file, err)
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), ".profile-*.json")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	return backup, os.Rename(tmp.Name(), file)
}

// ValidationIssues runs the install's 'profile validate' and returns the
// issues it reports.
func ValidationIssues(in Install) ([]string, error) {
	out, err := Run(in.Binary, []string{"profile", "validate"}, nil, nil)
	var report struct {
		Issues []string `json:"issues"`
	}
	if jerr := json.Unmarshal(out, &report); jerr != nil {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("profile validate printed unexpected output: %s", strings.TrimSpace(string(out)))
	}
	sort.Strings(report.Issues)
	return report.Issues, nil
}
