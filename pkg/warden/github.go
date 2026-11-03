package warden

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// apiBase is the GitHub API root; tests point it at a local server.
var apiBase = "https://api.github.com"

var httpClient = &http.Client{Timeout: 5 * time.Minute}

// Token returns a GitHub token from GITHUB_TOKEN, GH_TOKEN, or the gh CLI,
// and where it came from. Both are empty when none is available.
func Token() (token, source string) {
	for _, name := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v, name
		}
	}
	if gh, err := exec.LookPath("gh"); err == nil {
		out, err := exec.Command(gh, "auth", "token").Output()
		if v := strings.TrimSpace(string(out)); err == nil && v != "" {
			return v, "gh auth token"
		}
	}
	return "", ""
}

// AccessError explains why a repository cannot be read, in terms of what the
// user can do about it.
type AccessError struct {
	Repo        string
	TokenSource string // empty when no token was used
	Status      int    // HTTP status; 0 for a network failure
	Detail      string
}

func (e *AccessError) Error() string {
	repo := "github.com/" + e.Repo
	tokenHelp := "Set GITHUB_TOKEN (or GH_TOKEN) to a token that can read it, or log in with 'gh auth login'.\n" +
		"Under sudo the variable is dropped; keep it with: sudo --preserve-env=GITHUB_TOKEN seer warden install ..."
	accessHelp := "Ask a " + owner(e.Repo) + " organization owner to give your GitHub account read access to " + e.Repo + ".\n" +
		"A fine-grained token must list this repository with \"Contents: read\"; a classic token needs the \"repo\" scope.\n" +
		"If the organization uses SSO, authorize the token for it on GitHub."
	switch {
	case e.Status == 0:
		return fmt.Sprintf("cannot reach GitHub to check access to %s: %s\n"+
			"Check this box's network and DNS, or copy a Warden binary here and use --binary.", repo, e.Detail)
	case e.Status == http.StatusUnauthorized:
		return fmt.Sprintf("GitHub rejected the token from %s while checking %s: it is invalid, expired, or revoked.\n%s",
			e.TokenSource, repo, "Create a new token, or run 'gh auth login' again.")
	case e.Status == http.StatusForbidden && strings.Contains(e.Detail, "rate limit"):
		hint := "Wait and retry."
		if e.TokenSource == "" {
			hint = "Unauthenticated requests are limited to 60 an hour. " + tokenHelp
		}
		return fmt.Sprintf("GitHub rate limit reached while checking %s.\n%s", repo, hint)
	case (e.Status == http.StatusNotFound || e.Status == http.StatusForbidden) && e.TokenSource == "":
		return fmt.Sprintf("cannot access %s: it is a private repository and no GitHub token was found.\n%s", repo, tokenHelp)
	case e.Status == http.StatusNotFound || e.Status == http.StatusForbidden:
		return fmt.Sprintf("the GitHub token from %s does not have access to %s (GitHub returned %d).\n%s",
			e.TokenSource, repo, e.Status, accessHelp)
	}
	return fmt.Sprintf("GitHub returned %d while checking access to %s: %s", e.Status, repo, e.Detail)
}

func owner(repo string) string {
	o, _, _ := strings.Cut(repo, "/")
	return o
}

func apiGet(ctx context.Context, url, token, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "seer")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	// Go drops the Authorization header when GitHub redirects an asset
	// download to its storage host, which is what that host expects.
	return httpClient.Do(req)
}

func responseError(repo, tokenSource string, resp *http.Response) *AccessError {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var msg struct {
		Message string `json:"message"`
	}
	detail := strings.TrimSpace(string(body))
	if json.Unmarshal(body, &msg) == nil && msg.Message != "" {
		detail = msg.Message
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		detail = "rate limit: " + detail
	}
	return &AccessError{Repo: repo, TokenSource: tokenSource, Status: resp.StatusCode, Detail: detail}
}

// CheckAccess confirms the repository can be read with the given token (or
// none), and returns an AccessError that says what to fix when it cannot.
func CheckAccess(ctx context.Context, repo, token, tokenSource string) error {
	resp, err := apiGet(ctx, apiBase+"/repos/"+repo, token, "application/vnd.github+json")
	if err != nil {
		return &AccessError{Repo: repo, TokenSource: tokenSource, Detail: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return responseError(repo, tokenSource, resp)
	}
	return nil
}

type release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"assets"`
}

// DownloadRelease saves the release's warden-linux-<arch> binary to dest and
// verifies it against the release's SHA256SUMS and, when given, pin. tag may
// be "latest". It returns the resolved tag and the binary's checksum.
func DownloadRelease(ctx context.Context, repo, tag, arch, token, tokenSource, dest, pin string) (string, string, error) {
	url := apiBase + "/repos/" + repo + "/releases/tags/" + tag
	if tag == "latest" {
		url = apiBase + "/repos/" + repo + "/releases/latest"
	}
	resp, err := apiGet(ctx, url, token, "application/vnd.github+json")
	if err != nil {
		return "", "", &AccessError{Repo: repo, TokenSource: tokenSource, Detail: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		// Access was already checked, so a 404 here means the release.
		return "", "", fmt.Errorf("%s has no release %q", repo, tag)
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", responseError(repo, tokenSource, resp)
	}
	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", "", fmt.Errorf("reading release %s: %w", tag, err)
	}

	binName := "warden-linux-" + arch
	assets := map[string]string{}
	for _, a := range rel.Assets {
		assets[a.Name] = a.URL
	}
	if assets[binName] == "" {
		return "", "", fmt.Errorf("release %s of %s has no %s asset", rel.TagName, repo, binName)
	}
	if assets["SHA256SUMS"] == "" {
		return "", "", fmt.Errorf("release %s of %s has no SHA256SUMS; refusing an unverified binary", rel.TagName, repo)
	}

	var sums strings.Builder
	if err := downloadAsset(ctx, repo, assets["SHA256SUMS"], token, tokenSource, &sums); err != nil {
		return "", "", err
	}
	want := checksumFor(sums.String(), binName)
	if want == "" {
		return "", "", fmt.Errorf("SHA256SUMS in release %s does not list %s", rel.TagName, binName)
	}

	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
	if err != nil {
		return "", "", err
	}
	hash := newHasher()
	err = downloadAsset(ctx, repo, assets[binName], token, tokenSource, io.MultiWriter(f, hash))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", "", err
	}
	got := hash.sum()
	if got != want {
		return "", "", fmt.Errorf("checksum mismatch for %s: got %s, release says %s", binName, got, want)
	}
	if pin != "" && !strings.EqualFold(got, pin) {
		return "", "", fmt.Errorf("checksum %s does not match the pinned --sha256 %s", got, pin)
	}
	return rel.TagName, got, nil
}

func downloadAsset(ctx context.Context, repo, url, token, tokenSource string, w io.Writer) error {
	resp, err := apiGet(ctx, url, token, "application/octet-stream")
	if err != nil {
		return &AccessError{Repo: repo, TokenSource: tokenSource, Detail: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return responseError(repo, tokenSource, resp)
	}
	_, err = io.Copy(w, resp.Body)
	return err
}

// checksumFor finds file in sha256sum output ("<hash>  <file>", or
// "<hash> *<file>" in binary mode).
func checksumFor(sums, file string) string {
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == file {
			return strings.ToLower(fields[0])
		}
	}
	return ""
}

// gitAuthEnv passes the token to git as an HTTP header through environment
// configuration, so it never appears in a process's arguments.
func gitAuthEnv(token string) []string {
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if token == "" {
		return env
	}
	basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	return append(env,
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.https://github.com/.extraheader",
		"GIT_CONFIG_VALUE_0=Authorization: Basic "+basic,
	)
}

// Clone makes a shallow clone of repo (at ref, when given) in dest.
func Clone(ctx context.Context, repo, ref, token, tokenSource, dest string) error {
	git, err := exec.LookPath("git")
	if err != nil {
		return errors.New("git is not installed; install it, or use --release or --binary")
	}
	args := []string{"clone", "--quiet", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, "https://github.com/"+repo+".git", dest)
	cmd := exec.CommandContext(ctx, git, args...)
	cmd.Env = gitAuthEnv(token)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	text := strings.TrimSpace(string(out))
	lower := strings.ToLower(text)
	for _, s := range []string{"could not read username", "authentication failed", "repository not found", "403"} {
		if strings.Contains(lower, s) {
			return &AccessError{Repo: repo, TokenSource: tokenSource, Status: http.StatusNotFound, Detail: text}
		}
	}
	return fmt.Errorf("git clone of %s failed: %s", repo, text)
}

// Build compiles cmd/warden in a clone with the flags Warden's own build
// uses. GOTOOLCHAIN=auto lets an older Go fetch the version go.mod requires.
func Build(ctx context.Context, srcDir, dest, workDir string, log io.Writer) error {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return errors.New("go is not installed; install Go (https://go.dev/doc/install), or use --release or --binary")
	}
	if _, err := os.Stat(srcDir + "/cmd/warden"); err != nil {
		return fmt.Errorf("%s is not a Warden source tree (no cmd/warden)", srcDir)
	}
	cmd := exec.CommandContext(ctx, goBin, "build", "-trimpath", "-ldflags=-s -w", "-o", dest, "./cmd/warden")
	cmd.Dir = srcDir
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0", "GOOS=linux", "GOTOOLCHAIN=auto", "GOFLAGS=-modcacherw",
		"GOPATH="+workDir+"/gopath", "GOCACHE="+workDir+"/cache",
	)
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("building Warden failed: %w", err)
	}
	return nil
}
