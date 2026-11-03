package warden

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// Status is the part of `status --json` Seer reads. The contract is
// additive-only, so unknown fields are ignored.
type Status struct {
	Host           string               `json:"host"`
	Name           string               `json:"name"`
	Installed      bool                 `json:"installed"`
	Armed          bool                 `json:"armed"`
	ArmedAt        string               `json:"armed_at,omitempty"`
	AllowFrom      string               `json:"allow_from,omitempty"`
	Ban            string               `json:"ban"`
	Lock           string               `json:"lock"`
	Peers          []string             `json:"peers"`
	Tiers          map[string]Tier      `json:"tiers"`
	Components     map[string]Component `json:"components"`
	PendingReloads int                  `json:"pending_reloads"`
	Quarantined    int                  `json:"quarantined"`
}

type Tier struct {
	Generation     int    `json:"generation"`
	CreatedAt      string `json:"created_at,omitempty"`
	Files          int    `json:"files"`
	MissingObjects int    `json:"missing_objects"`
	Error          string `json:"error,omitempty"`
}

type Component struct {
	Time   string `json:"time"`
	Action string `json:"action"`
}

// ParseStatus decodes `status --json` output.
func ParseStatus(data []byte) (Status, error) {
	var s Status
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("warden status printed unexpected output: %w", err)
	}
	return s, nil
}

// Severity orders findings from fine to broken.
type Severity int

const (
	OK Severity = iota
	Warn
	Bad
)

// Finding is one health observation and what to do about it.
type Finding struct {
	Severity Severity
	Message  string
	Action   string
}

// componentTimers gives each component's timer interval plus jitter. A
// component is stale when it last ran more than twice that long ago.
var componentTimers = map[string]time.Duration{
	"sentinel":  time.Minute + 30*time.Second,
	"watch":     2*time.Minute + 30*time.Second,
	"snapshot":  5*time.Minute + time.Minute,
	"replicate": 15*time.Minute + 3*time.Minute,
}

// Health applies the Warden interface's decision rules to a status report.
func Health(s Status, now time.Time) []Finding {
	var f []Finding
	add := func(sev Severity, msg, action string) { f = append(f, Finding{sev, msg, action}) }
	cmd := func(sub string) string { return s.Name + " " + sub }

	if !s.Installed {
		add(Bad, "Warden reports it is not installed or its config is gone", "re-run: seer warden install --key <team-pub>")
		return f
	}
	if s.Armed {
		add(OK, "armed: auto-restore is on", "")
	} else {
		add(Warn, "not armed: drift is only flagged, not restored", "when the box is configured: "+cmd("arm"))
	}

	for _, name := range []string{"config", "data"} {
		t, ok := s.Tiers[name]
		switch {
		case !ok:
			add(Warn, name+" tier missing from status", "")
		case t.Error != "":
			add(Bad, name+" tier error: "+t.Error, "")
		case t.MissingObjects > 0:
			add(Bad, fmt.Sprintf("%s tier is missing %d stored object(s)", name, t.MissingObjects), "pull from a peer: "+cmd("retrieve <peer-url> --apply"))
		case t.Generation == 0:
			add(Warn, name+" tier has no snapshot yet", "wait for the timer, or run: "+cmd("snapshot --tier "+name))
		default:
			add(OK, fmt.Sprintf("%s tier at generation %d (%d files)", name, t.Generation, t.Files), "")
		}
	}

	names := make([]string, 0, len(s.Components))
	for n := range s.Components {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		c := s.Components[n]
		limit, known := componentTimers[n]
		if n == "replicate" && len(s.Peers) == 0 {
			continue
		}
		ran, err := time.Parse(time.RFC3339, c.Time)
		switch {
		case err != nil:
			add(Warn, n+" has no valid last-run time", "")
		case known && now.Sub(ran) > 2*limit:
			add(Warn, fmt.Sprintf("%s last ran %s ago; its timer may not be running", n, now.Sub(ran).Round(time.Second)),
				"run "+cmd("sentinel-check")+", then check: systemctl list-timers '"+s.Name+"-*'")
		case c.Action != "" && c.Action != "pass":
			add(Warn, fmt.Sprintf("%s last result: %s", n, c.Action), "review: "+cmd("alerts"))
		default:
			add(OK, n+" ran "+now.Sub(ran).Round(time.Second).String()+" ago", "")
		}
	}

	if len(s.Peers) == 0 {
		add(Warn, "no peers: backups are not kept off this box", "pair with another box: "+cmd("pair key")+" (see Warden docs)")
	}
	if s.Quarantined > 0 || s.PendingReloads > 0 {
		add(Warn, fmt.Sprintf("%d quarantined file(s), %d pending reload(s)", s.Quarantined, s.PendingReloads), "review: "+cmd("alerts"))
	}
	return f
}

// Worst returns the highest severity among findings.
func Worst(findings []Finding) Severity {
	worst := OK
	for _, f := range findings {
		if f.Severity > worst {
			worst = f.Severity
		}
	}
	return worst
}
