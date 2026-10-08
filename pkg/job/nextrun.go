package job

import (
	"strconv"
	"strings"
	"time"
)

var macroFields = map[string]string{
	"@hourly":   "0 * * * *",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@weekly":   "0 0 * * 0",
	"@monthly":  "0 0 1 * *",
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
}

// NextRun returns the first time strictly after from that schedule fires, in
// from's location. It reports false for @reboot, anacron periods, and
// schedules that can't be parsed or never fire (such as February 30th).
func NextRun(schedule string, from time.Time) (time.Time, bool) {
	fields := strings.Fields(schedule)
	if len(fields) == 1 {
		expanded, ok := macroFields[fields[0]]
		if !ok {
			return time.Time{}, false
		}
		fields = strings.Fields(expanded)
	}
	if len(fields) != 5 {
		return time.Time{}, false
	}

	minutes, ok1 := parseCronField(fields[0], 0, 59, nil)
	hours, ok2 := parseCronField(fields[1], 0, 23, nil)
	doms, ok3 := parseCronField(fields[2], 1, 31, nil)
	months, ok4 := parseCronField(fields[3], 1, 12, monthNames)
	dows, ok5 := parseCronField(fields[4], 0, 7, weekdayNames)
	if !(ok1 && ok2 && ok3 && ok4 && ok5) {
		return time.Time{}, false
	}
	if dows[7] {
		dows[0] = true
	}

	// Cron fires on day-of-month OR day-of-week when neither starts with "*".
	domAny, dowAny := strings.HasPrefix(fields[2], "*"), strings.HasPrefix(fields[4], "*")
	dayMatches := func(t time.Time) bool {
		domOK, dowOK := doms[t.Day()], dows[int(t.Weekday())]
		switch {
		case domAny || dowAny:
			// A leading "*" in either field means both must match.
			return domOK && dowOK
		default:
			return domOK || dowOK
		}
	}

	start := from.Truncate(time.Minute).Add(time.Minute)

	// 9 years covers leap-day schedules.
	day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	for i := 0; i < 9*366; i++ {
		d := day.AddDate(0, 0, i)
		if !months[int(d.Month())] || !dayMatches(d) {
			continue
		}
		for h := 0; h < 24; h++ {
			if !hours[h] {
				continue
			}
			for m := 0; m < 60; m++ {
				if !minutes[m] {
					continue
				}
				t := time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, d.Location())
				if !t.Before(start) {
					return t, true
				}
			}
		}
	}

	return time.Time{}, false
}

// parseCronField expands one field into the set of values it matches.
func parseCronField(field string, lo, hi int, names []string) (map[int]bool, bool) {
	set := make(map[int]bool)

	for _, item := range strings.Split(field, ",") {
		base, stepText, hasStep := strings.Cut(item, "/")

		step := 1
		if hasStep {
			n, err := strconv.Atoi(stepText)
			if err != nil || n < 1 {
				return nil, false
			}
			step = n
		}

		from, to := lo, hi
		switch {
		case base == "*":
		case strings.Contains(base, "-"):
			a, b, _ := strings.Cut(base, "-")
			var ok1, ok2 bool
			from, ok1 = cronValue(a, names)
			to, ok2 = cronValue(b, names)
			if !ok1 || !ok2 {
				return nil, false
			}
		default:
			v, ok := cronValue(base, names)
			if !ok {
				return nil, false
			}
			from, to = v, v
			if hasStep {
				to = hi
			}
		}

		if from < lo || to > hi || from > to {
			return nil, false
		}
		for v := from; v <= to; v += step {
			set[v] = true
		}
	}

	return set, true
}

func cronValue(text string, names []string) (int, bool) {
	if n, err := strconv.Atoi(text); err == nil {
		return n, true
	}
	for i, name := range names {
		if name != "" && len(text) == 3 && strings.EqualFold(name[:3], text) {
			// Weekday names start at index 0; month names at 1.
			return i, true
		}
	}

	return 0, false
}
