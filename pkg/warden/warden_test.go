package warden

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func useAPI(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	old := apiBase
	apiBase = srv.URL
	t.Cleanup(func() { apiBase = old; srv.Close() })
	return srv
}

func TestCheckAccessErrors(t *testing.T) {
	cases := []struct {
		name, token, source string
		status              int
		header              map[string]string
		want                string
	}{
		{name: "ok", status: 200},
		{name: "private without token", status: 404, want: "private repository and no GitHub token was found"},
		{name: "token without access", token: "t", source: "GITHUB_TOKEN", status: 404, want: "GITHUB_TOKEN does not have access to github.com/cofcsecurity/quiet-warden"},
		{name: "token forbidden", token: "t", source: "GH_TOKEN", status: 403, want: "does not have access"},
		{name: "bad token", token: "t", source: "GITHUB_TOKEN", status: 401, want: "invalid, expired, or revoked"},
		{name: "rate limit", status: 403, header: map[string]string{"X-RateLimit-Remaining": "0"}, want: "rate limit reached"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var gotAuth string
			useAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				if r.URL.Path != "/repos/cofcsecurity/quiet-warden" {
					t.Errorf("path %s", r.URL.Path)
				}
				for k, v := range c.header {
					w.Header().Set(k, v)
				}
				w.WriteHeader(c.status)
				w.Write([]byte(`{"message":"Not Found"}`))
			}))
			err := CheckAccess(context.Background(), "cofcsecurity/quiet-warden", c.token, c.source)
			if c.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %v, want %q", err, c.want)
			}
			if c.token != "" && gotAuth != "Bearer "+c.token {
				t.Errorf("Authorization %q", gotAuth)
			}
			if c.token == "" && gotAuth != "" {
				t.Errorf("sent Authorization without a token")
			}
		})
	}
}

func TestCheckAccessNetwork(t *testing.T) {
	srv := useAPI(t, http.NotFoundHandler())
	srv.Close()
	err := CheckAccess(context.Background(), "cofcsecurity/warden", "", "")
	if err == nil || !strings.Contains(err.Error(), "cannot reach GitHub") {
		t.Fatalf("error %v", err)
	}
}

func TestDownloadRelease(t *testing.T) {
	binary := []byte("\x7fELF fake warden")
	sum := sha256.Sum256(binary)
	good := hex.EncodeToString(sum[:])
	sums := good + "  warden-linux-amd64\n" + strings.Repeat("0", 64) + "  warden-linux-arm64\n"

	var srv *httptest.Server
	srv = useAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/releases/latest":
			json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "v1.2.3",
				"assets": []map[string]string{
					{"name": "warden-linux-amd64", "url": srv.URL + "/assets/1"},
					{"name": "SHA256SUMS", "url": srv.URL + "/assets/2"},
				},
			})
		case "/assets/1":
			if r.Header.Get("Accept") != "application/octet-stream" {
				t.Errorf("asset Accept %q", r.Header.Get("Accept"))
			}
			w.Write(binary)
		case "/assets/2":
			w.Write([]byte(sums))
		default:
			http.NotFound(w, r)
		}
	}))

	dest := filepath.Join(t.TempDir(), "warden")
	tag, got, err := DownloadRelease(context.Background(), "o/r", "latest", "amd64", "", "", dest, "")
	if err != nil {
		t.Fatal(err)
	}
	if tag != "v1.2.3" || got != good {
		t.Fatalf("tag %s sum %s", tag, got)
	}
	if data, _ := os.ReadFile(dest); string(data) != string(binary) {
		t.Fatal("binary not saved")
	}

	if _, _, err := DownloadRelease(context.Background(), "o/r", "latest", "amd64", "", "", dest, "abc"); err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Fatalf("pin mismatch error %v", err)
	}
	if _, _, err := DownloadRelease(context.Background(), "o/r", "latest", "arm64", "", "", dest, ""); err == nil || !strings.Contains(err.Error(), "no warden-linux-arm64") {
		t.Fatalf("missing asset error %v", err)
	}
	if _, _, err := DownloadRelease(context.Background(), "o/r", "v9", "amd64", "", "", dest, ""); err == nil || !strings.Contains(err.Error(), `no release "v9"`) {
		t.Fatalf("missing release error %v", err)
	}
}

func TestChecksumFor(t *testing.T) {
	sums := "AAA  warden-linux-amd64\nbbb *warden-linux-arm64\n"
	if got := checksumFor(sums, "warden-linux-amd64"); got != "aaa" {
		t.Errorf("amd64 %q", got)
	}
	if got := checksumFor(sums, "warden-linux-arm64"); got != "bbb" {
		t.Errorf("arm64 %q", got)
	}
	if got := checksumFor(sums, "warden"); got != "" {
		t.Errorf("unlisted %q", got)
	}
}

func TestGitAuthEnvKeepsTokenOutOfArgs(t *testing.T) {
	env := strings.Join(gitAuthEnv("secret"), "\n")
	if strings.Contains(env, "secret") {
		t.Fatal("token should be base64 encoded in a header, not raw")
	}
	if !strings.Contains(env, "GIT_CONFIG_KEY_0=http.https://github.com/.extraheader") || !strings.Contains(env, "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("env %s", env)
	}
}

func useRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := root
	root = dir
	t.Cleanup(func() { root = old })
	return dir
}

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFindAndResolve(t *testing.T) {
	dir := useRoot(t)
	if _, err := Resolve(""); err == nil || !strings.Contains(err.Error(), "no Warden install") {
		t.Fatalf("empty box error %v", err)
	}

	touch(t, filepath.Join(dir, "var/lib/netmgr/config.json"))
	touch(t, filepath.Join(dir, "var/lib/netmgr/.spare"))
	touch(t, filepath.Join(dir, "usr/local/sbin/netmgr"))
	// A different app's config without a .spare copy is not Warden.
	touch(t, filepath.Join(dir, "var/lib/otherapp/config.json"))
	touch(t, filepath.Join(dir, "var/lib/netmgr-peers/box/x"))

	found, err := Find()
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Name != "netmgr" || found[0].BinaryMissing {
		t.Fatalf("found %+v", found)
	}
	in, err := Resolve("")
	if err != nil || in.Name != "netmgr" {
		t.Fatalf("resolve %+v %v", in, err)
	}

	touch(t, filepath.Join(dir, "var/lib/sysrelay/config.json"))
	touch(t, filepath.Join(dir, "var/lib/sysrelay/.spare"))
	found, _ = Find()
	if len(found) != 2 || !found[1].BinaryMissing {
		t.Fatalf("found %+v", found)
	}
	if _, err := Resolve(""); err == nil || !strings.Contains(err.Error(), "--name") {
		t.Fatalf("ambiguous error %v", err)
	}
	if in, err := Resolve("sysrelay"); err != nil || in.Name != "sysrelay" {
		t.Fatalf("named %+v %v", in, err)
	}
	if _, err := Resolve("../etc"); err == nil {
		t.Fatal("accepted a path as a name")
	}
}

func TestInstallArgs(t *testing.T) {
	o := InstallOptions{Key: "@team.pub", From: "10.0.0.0/24", Ban: "act", SafeAccounts: []string{"admin", "scorer"}, NoBreakGlass: true, Yes: true}
	// Warden's install has no --json flag; Seer must never pass one.
	want := []string{"install", "--key", "@team.pub", "--from", "10.0.0.0/24", "--ban", "act", "--safe-accounts", "admin,scorer", "--no-break-glass", "--yes"}
	if got := o.Args(); !reflect.DeepEqual(got, want) {
		t.Fatalf("args %q", got)
	}
}

func TestHealth(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 1, 0, 0, time.UTC)
	healthy := Status{
		Name: "netmgr", Installed: true, Armed: true, Peers: []string{"ssh+receiver://x"},
		Tiers: map[string]Tier{"config": {Generation: 42, Files: 318}, "data": {Generation: 7, Files: 5120}},
		Components: map[string]Component{
			"watch":     {Time: "2026-11-02T12:00:30Z", Action: "pass"},
			"sentinel":  {Time: "2026-11-02T12:00:05Z", Action: "pass"},
			"snapshot":  {Time: "2026-11-02T11:59:12Z", Action: "pass"},
			"replicate": {Time: "2026-11-02T11:47:40Z", Action: "pass"},
		},
	}
	if f := Health(healthy, now); Worst(f) != OK {
		t.Fatalf("healthy box: %+v", f)
	}

	s := healthy
	s.Armed = false
	s.Peers = nil
	s.Quarantined = 2
	s.Tiers = map[string]Tier{"config": {Generation: 0}, "data": {Generation: 3, MissingObjects: 4}}
	s.Components = map[string]Component{"sentinel": {Time: "2026-11-02T11:40:00Z", Action: "pass"}}
	f := Health(s, now)
	if Worst(f) != Bad {
		t.Fatalf("worst %v", Worst(f))
	}
	text := ""
	for _, x := range f {
		text += x.Message + " | " + x.Action + "\n"
	}
	for _, want := range []string{"not armed", "config tier has no snapshot", "missing 4 stored object", "retrieve <peer-url> --apply", "sentinel last ran 21m0s ago", "no peers", "2 quarantined"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}

	if f := Health(Status{Installed: false}, now); Worst(f) != Bad || !strings.Contains(f[0].Action, "seer warden install") {
		t.Fatalf("not installed: %+v", f)
	}
}

func TestLookupVariant(t *testing.T) {
	for in, repo := range map[string]string{"": "cofcsecurity/warden", "warden": "cofcsecurity/warden", "quiet": "cofcsecurity/quiet-warden", "Quiet-Warden": "cofcsecurity/quiet-warden"} {
		v, err := LookupVariant(in)
		if err != nil || v.Repo != repo {
			t.Errorf("%q -> %+v %v", in, v, err)
		}
	}
	if _, err := LookupVariant("loud"); err == nil {
		t.Error("accepted unknown variant")
	}
}

// fakeGo writes a go command that reports version v.
func fakeGo(t *testing.T, dir, v string) string {
	t.Helper()
	p := filepath.Join(dir, "go")
	os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho go version "+v+" linux/amd64\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFindGoPicksNewest(t *testing.T) {
	dir := t.TempDir()
	old := fakeGo(t, filepath.Join(dir, "usr/bin"), "go1.13.8")
	current := fakeGo(t, filepath.Join(dir, "usr/local/go/bin"), "go1.26.10")
	t.Setenv("PATH", filepath.Dir(old))
	saved := goCandidates
	t.Cleanup(func() { goCandidates = saved })

	goCandidates = []string{current}
	if got, err := findGo(); err != nil || got != current {
		t.Fatalf("got %q, %v; want the newer %s", got, err, current)
	}

	goCandidates = nil
	if _, err := findGo(); err == nil || !strings.Contains(err.Error(), "go1.13 at") || !strings.Contains(err.Error(), "1.21 or later") {
		t.Fatalf("old-only error %v", err)
	}

	t.Setenv("PATH", t.TempDir())
	if _, err := findGo(); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing error %v", err)
	}
}
