package job

import (
	"encoding/json"
	"strings"
	"time"
)

type jsonJob struct {
	ID          string   `json:"id"`
	Kind        Kind     `json:"kind"`
	Source      string   `json:"source"`
	Line        int      `json:"line,omitempty"`
	User        string   `json:"user"`
	UserType    string   `json:"user_type"`
	Schedule    string   `json:"schedule"`
	Description string   `json:"description,omitempty"`
	NextRun     string   `json:"next_run,omitempty"`
	Command     string   `json:"command"`
	Raw         string   `json:"raw"`
	Enabled     bool     `json:"enabled"`
	ReadOnly    bool     `json:"read_only"`
	Note        string   `json:"note,omitempty"`
	Risks       []string `json:"risks"`
}

// MarshalJSON gives scripts the same derived fields the text output shows.
func (j Job) MarshalJSON() ([]byte, error) {
	out := jsonJob{
		ID: j.ID, Kind: j.Kind, Source: j.Source, Line: j.LineNumber,
		User:        j.User,
		UserType:    strings.TrimPrefix(string(j.UserRole()), "user-"),
		Schedule:    j.Schedule,
		Description: j.humanSchedule(),
		Command:     j.Command, Raw: j.Raw,
		Enabled: j.Enabled, ReadOnly: j.ReadOnly, Note: j.Note,
		Risks: j.Risks,
	}
	if out.Risks == nil {
		out.Risks = []string{}
	}

	if j.Enabled && !j.ReadOnly {
		if next, ok := NextRun(j.Schedule, now()); ok {
			out.NextRun = next.Format(time.RFC3339)
		}
	}

	return json.Marshal(out)
}
