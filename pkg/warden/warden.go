// Package warden obtains, installs, finds, and checks Warden and Quiet
// Warden on the box Seer runs on. Both builds share one command interface;
// they differ only in the repository they come from.
package warden

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Variant is one build of Warden.
type Variant struct {
	Name string // display name, such as "quiet-warden"
	Repo string // GitHub owner/name
}

var variants = []Variant{
	{Name: "warden", Repo: "cofcsecurity/warden"},
	{Name: "quiet-warden", Repo: "cofcsecurity/quiet-warden"},
}

// LookupVariant accepts "warden" (or "standard") and "quiet-warden" (or
// "quiet").
func LookupVariant(name string) (Variant, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "warden", "standard":
		return variants[0], nil
	case "quiet", "quiet-warden", "quietwarden":
		return variants[1], nil
	}
	return Variant{}, fmt.Errorf("unknown variant %q: use warden or quiet-warden", name)
}

// VariantNames lists the accepted variant names, for completion and help.
func VariantNames() []string {
	names := make([]string, len(variants))
	for i, v := range variants {
		names[i] = v.Name
	}
	return names
}

// root prefixes every path Warden owns, so tests can use a temporary tree.
var root = "/"

func path(parts ...string) string {
	return filepath.Join(append([]string{root}, parts...)...)
}

// Install is a Warden install found on this box.
type Install struct {
	Name          string
	Binary        string
	DataDir       string
	Config        string
	BinaryMissing bool // the sentinel restores it from .spare within ~2 minutes
}

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$`)

// ValidName reports whether name is safe to use as an install name in paths.
func ValidName(name string) bool {
	return namePattern.MatchString(name) && name != "." && name != ".."
}

func installFor(name string) Install {
	in := Install{
		Name:    name,
		Binary:  path("usr", "local", "sbin", name),
		DataDir: path("var", "lib", name),
		Config:  path("var", "lib", name, "config.json"),
	}
	if _, err := os.Stat(in.Binary); err != nil {
		in.BinaryMissing = true
	}
	return in
}

// Find lists Warden installs. Install names are random, so it looks for
// /var/lib/<name>/config.json beside a .spare binary copy, as the Warden
// interface specifies.
func Find() ([]Install, error) {
	matches, err := filepath.Glob(path("var", "lib", "*", "config.json"))
	if err != nil {
		return nil, err
	}
	var found []Install
	for _, config := range matches {
		dir := filepath.Dir(config)
		name := filepath.Base(dir)
		if !ValidName(name) {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, ".spare")); err != nil {
			continue
		}
		found = append(found, installFor(name))
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	return found, nil
}

// Resolve picks the install to act on: the named one, or the only one.
func Resolve(name string) (Install, error) {
	if name != "" {
		if !ValidName(name) {
			return Install{}, fmt.Errorf("invalid install name %q", name)
		}
		in := installFor(name)
		if _, err := os.Stat(in.Config); err != nil {
			return Install{}, fmt.Errorf("no Warden install named %q (%s not found)", name, in.Config)
		}
		return in, nil
	}
	found, err := Find()
	if err != nil {
		return Install{}, err
	}
	switch len(found) {
	case 0:
		return Install{}, fmt.Errorf("no Warden install found on this box (looked for /var/lib/*/config.json with a .spare copy)")
	case 1:
		return found[0], nil
	}
	names := make([]string, len(found))
	for i, in := range found {
		names[i] = in.Name
	}
	return Install{}, fmt.Errorf("found %d Warden installs (%s); choose one with --name", len(found), strings.Join(names, ", "))
}
