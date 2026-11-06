package warden

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func paths(entries []ProposedEntry) map[string]ProposedEntry {
	m := map[string]ProposedEntry{}
	for _, e := range entries {
		m[e.Path] = e
	}
	return m
}

func TestProposeProfileOnlyPresentFiles(t *testing.T) {
	dir := useRoot(t)
	touch(t, filepath.Join(dir, "etc/pam.d/sshd"))
	touch(t, filepath.Join(dir, "etc/firewalld/zones/public.xml"))
	touch(t, filepath.Join(dir, "etc/firewalld/zones/public.xml.old"))
	touch(t, filepath.Join(dir, "etc/my.cnf.d/mysql-server.cnf"))
	touch(t, filepath.Join(dir, "etc/dnsmasq.conf"))
	// Present on most CentOS boxes whether or not mail or NFS is used.
	touch(t, filepath.Join(dir, "etc/aliases"))
	touch(t, filepath.Join(dir, "etc/exports"))
	// resolv.conf as a symlink into /run is skipped.
	os.Symlink("/run/systemd/resolve/stub-resolv.conf", filepath.Join(dir, "etc/resolv.conf"))

	got, applied, err := ProposeProfile(nil)
	if err != nil {
		t.Fatal(err)
	}
	m := paths(got)
	for _, want := range []string{"/etc/pam.d/*", "/etc/firewalld/zones/*.xml", "/etc/my.cnf.d/*", "/etc/dnsmasq.conf"} {
		if _, ok := m[want]; !ok {
			t.Errorf("missing %s in %v", want, m)
		}
	}
	for _, absent := range []string{"/etc/resolv.conf", "/etc/nginx/**", "/etc/postfix/**", "/etc/dnsmasq.d/*", "/etc/aliases", "/etc/exports"} {
		if _, ok := m[absent]; ok {
			t.Errorf("proposed %s, which is not on the box", absent)
		}
	}
	if fw := m["/etc/firewalld/zones/*.xml"]; fw.Class != ConfirmFirst || !reflect.DeepEqual(fw.Services, []string{"firewalld"}) {
		t.Errorf("firewall entry %+v", fw)
	}
	if !reflect.DeepEqual(applied, []string{"base", "dns", "db"}) {
		t.Errorf("auto roles %v", applied)
	}

	// An explicit role limits the templates to base plus that role.
	got, applied, _ = ProposeProfile([]string{"db"})
	if _, ok := paths(got)["/etc/dnsmasq.conf"]; ok || !reflect.DeepEqual(applied, []string{"base", "db"}) {
		t.Errorf("--role db gave %v %v", applied, got)
	}
	// Asking for a role includes its incidental files.
	got, _, _ = ProposeProfile([]string{"mail"})
	if _, ok := paths(got)["/etc/aliases"]; !ok {
		t.Errorf("--role mail left out /etc/aliases: %v", got)
	}
	if _, _, err := ProposeProfile([]string{"webserver"}); err == nil || !strings.Contains(err.Error(), "unknown role") {
		t.Errorf("bad role error %v", err)
	}
}

func TestDiscoverSharesAndApps(t *testing.T) {
	dir := useRoot(t)
	smb := filepath.Join(dir, "etc/samba/smb.conf")
	touch(t, smb)
	os.WriteFile(smb, []byte(`[global]
   workgroup = BLUE
[homes]
   path = /home/%S
[finance]
   path = /srv/shares/finance
; path = /commented/out
[printers]
   path = /var/spool/samba
`), 0o644)
	touch(t, filepath.Join(dir, "srv/shares/finance/q3.xlsx"))
	touch(t, filepath.Join(dir, "opt/blog/manage.py"))
	touch(t, filepath.Join(dir, "opt/blog/blog/wsgi.py"))
	touch(t, filepath.Join(dir, "opt/blog/venv/lib/app.py"))

	got, _, err := ProposeProfile([]string{"fileshare", "web"})
	if err != nil {
		t.Fatal(err)
	}
	m := paths(got)
	if e, ok := m["/srv/shares/finance/**"]; !ok || e.Tier != TierData || e.Class != ConfirmFirst {
		t.Errorf("share entry %+v in %v", e, m)
	}
	if e, ok := m["/opt/blog/**"]; !ok || e.Class != ConfirmFirst {
		t.Errorf("app entry %+v in %v", e, m)
	}
	for p := range m {
		if strings.Contains(p, "spool") || strings.Contains(p, "%") || strings.Contains(p, "commented") || strings.Contains(p, "venv") || p == "/opt/blog/blog/**" {
			t.Errorf("unexpected %s", p)
		}
	}
}

func TestMergeProfileKeepsTeamEntries(t *testing.T) {
	file := filepath.Join(t.TempDir(), "profile.json")
	os.WriteFile(file, []byte(`{
  "exclude": ["/etc/hostname"],
  "ignored_processes": ["scorebot"],
  "paths": [
    {"path": "/etc/pam.d/*", "tier": "config", "class": "confirm-first", "note": "team choice"}
  ]
}`), 0o600)
	proposed := []ProposedEntry{
		{ProfileEntry: ProfileEntry{Path: "/etc/pam.d/*", Tier: TierConfig, Class: AutoRestore}, Role: "base"},
		{ProfileEntry: ProfileEntry{Path: "/etc/postfix/**", Tier: TierConfig, Class: AutoRestore, Services: []string{"postfix"}}, Role: "mail"},
	}
	added, content, err := MergeProfile(file, proposed)
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 || added[0].Path != "/etc/postfix/**" {
		t.Fatalf("added %+v", added)
	}
	var doc struct {
		Exclude          []string          `json:"exclude"`
		IgnoredProcesses []string          `json:"ignored_processes"`
		Paths            []json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(content, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Exclude) != 1 || len(doc.IgnoredProcesses) != 1 || len(doc.Paths) != 2 {
		t.Fatalf("lost fields: %s", content)
	}
	if !strings.Contains(string(doc.Paths[0]), `"confirm-first"`) || !strings.Contains(string(doc.Paths[0]), `"note"`) {
		t.Fatalf("team entry changed: %s", doc.Paths[0])
	}

	backup, err := WriteProfile(file, content)
	if err != nil || backup != file+".bak" {
		t.Fatalf("write %q %v", backup, err)
	}
	if fi, _ := os.Stat(file); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode())
	}

	os.WriteFile(file, []byte("{not json"), 0o600)
	if _, _, err := MergeProfile(file, proposed); err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("broken profile error %v", err)
	}
	if added, _, err := MergeProfile(filepath.Join(t.TempDir(), "none.json"), proposed); err != nil || len(added) != 2 {
		t.Errorf("new profile %v %v", added, err)
	}
}

func TestInstallArgsPassword(t *testing.T) {
	o := InstallOptions{Key: "ssh-ed25519 AAAA", PasswordHash: "$6$s$h"}
	want := []string{"install", "--key", "ssh-ed25519 AAAA", "--password-hash", "$6$s$h"}
	if got := o.Args(); !reflect.DeepEqual(got, want) {
		t.Fatalf("args %q", got)
	}
	o = InstallOptions{NoPassword: true}
	if got := o.Args(); !reflect.DeepEqual(got, []string{"install", "--no-password"}) {
		t.Fatalf("args %q", got)
	}
}
