package warden

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeKey returns a well-formed public key line of the given type.
func fakeKey(typ, comment string) string {
	blob := make([]byte, 4, 4+len(typ)+32)
	binary.BigEndian.PutUint32(blob, uint32(len(typ)))
	blob = append(blob, typ...)
	blob = append(blob, make([]byte, 32)...)
	line := typ + " " + base64.StdEncoding.EncodeToString(blob)
	if comment != "" {
		line += " " + comment
	}
	return line
}

func TestResolveKey(t *testing.T) {
	good := fakeKey("ssh-ed25519", "team@seer")
	dir := t.TempDir()
	goodFile := filepath.Join(dir, "team.pub")
	os.WriteFile(goodFile, []byte("# team key\n"+good+"\n"), 0o600)
	privFile := filepath.Join(dir, "id_ed25519")
	os.WriteFile(privFile, []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----\n"), 0o600)
	multiFile := filepath.Join(dir, "two.pub")
	os.WriteFile(multiFile, []byte(good+"\n"+fakeKey("ssh-rsa", "")+"\n"), 0o600)

	// A key whose base64 is valid but whose blob is cut short.
	blob, _ := base64.StdEncoding.DecodeString(strings.Fields(good)[1])
	truncated := "ssh-ed25519 " + base64.StdEncoding.EncodeToString(blob[:10])
	mismatch := "ssh-rsa " + strings.Fields(good)[1]

	cases := []struct {
		name, value, want string
	}{
		{name: "inline", value: good, want: good},
		{name: "inline with spaces", value: "  " + good + "  ", want: good},
		{name: "file", value: "@" + goodFile, want: good},
		{name: "missing file", value: "@" + filepath.Join(dir, "nope.pub"), want: "does not exist"},
		{name: "empty at", value: "@", want: "names no file"},
		{name: "private key file", value: "@" + privFile, want: "PRIVATE key"},
		{name: "empty", value: "   ", want: "is empty"},
		{name: "several keys", value: "@" + multiFile, want: "takes exactly one"},
		{name: "not a key", value: "hello world", want: "does not accept"},
		{name: "one field", value: "ssh-ed25519", want: "not a public key"},
		{name: "unknown type", value: "ssh-dss AAAA", want: "does not accept"},
		{name: "bad base64", value: "ssh-ed25519 !!!notbase64", want: "not base64"},
		{name: "truncated", value: truncated, want: "does not match its type"},
		{name: "type mismatch", value: mismatch, want: "does not match its type ssh-rsa"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ResolveKey(context.Background(), c.value)
			if c.want == good {
				if err != nil || got != good {
					t.Fatalf("got %q, %v; want %q", got, err, good)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %q, %v; want error containing %q", got, err, c.want)
			}
		})
	}
}

func TestResolveKeyURL(t *testing.T) {
	good := fakeKey("ecdsa-sha2-nistp256", "")
	useKeyServer := func(status int, body string) string {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			w.Write([]byte(body))
		}))
		t.Cleanup(srv.Close)
		old := httpClient
		httpClient = srv.Client()
		t.Cleanup(func() { httpClient = old })
		return srv.URL + "/team.pub"
	}

	if got, err := ResolveKey(context.Background(), useKeyServer(200, good+"\n")); err != nil || got != good {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := ResolveKey(context.Background(), useKeyServer(404, "")); err == nil || !strings.Contains(err.Error(), "returned 404") {
		t.Fatalf("404: %v", err)
	}
	if _, err := ResolveKey(context.Background(), useKeyServer(200, "<html>not a key</html>")); err == nil || !strings.Contains(err.Error(), "does not accept") {
		t.Fatalf("html: %v", err)
	}
}

func TestCheckAccessUsesEachVariantRepo(t *testing.T) {
	for _, v := range variants {
		var path string
		useAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path = r.URL.Path
			w.WriteHeader(http.StatusOK)
		}))
		if err := CheckAccess(context.Background(), v.Repo, "", ""); err != nil {
			t.Fatal(err)
		}
		if path != "/repos/"+v.Repo {
			t.Errorf("%s checked %s", v.Repo, path)
		}
	}
}
