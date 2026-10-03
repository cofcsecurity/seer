package job

import (
	"fmt"
	"strconv"
	"strings"
)

var macroDescriptions = map[string]string{
	"@reboot":   "At system startup",
	"@yearly":   "At 00:00 on January 1st",
	"@annually": "At 00:00 on January 1st",
	"@monthly":  "At 00:00 on the 1st of every month",
	"@weekly":   "At 00:00 on Sunday",
	"@daily":    "At 00:00 every day",
	"@midnight": "At 00:00 every day",
	"@hourly":   "At minute 0 of every hour",
}

var monthNames = []string{
	"", "January", "February", "March", "April", "May", "June",
	"July", "August", "September", "October", "November", "December",
}

var weekdayNames = []string{
	"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday",
}

// DescribeSchedule renders a cron schedule as plain English, for example
// "30 9 * * 1-5" becomes "At 09:30 on Monday through Friday". It returns an
// empty string when the schedule cannot be interpreted.
func DescribeSchedule(schedule string) string {
	fields := strings.Fields(schedule)
	if len(fields) == 1 {
		return macroDescriptions[fields[0]]
	}
	if len(fields) != 5 {
		return ""
	}

	minute, hour, dom, month, dow := fields[0], fields[1], fields[2], fields[3], fields[4]

	parts := []string{describeTime(minute, hour)}

	switch {
	case dom == "*" && dow == "*" && month == "*":
		if strings.HasPrefix(parts[0], "At") {
			parts = append(parts, "every day")
		}
	default:
		var when []string
		if dom != "*" {
			when = append(when, "day-of-month "+describeField(dom, "day", nil))
		}
		if dow != "*" {
			when = append(when, describeField(dow, "weekday", weekdayNames))
		}
		if len(when) > 0 {
			parts = append(parts, "on "+strings.Join(when, " or "))
		}
		if month != "*" {
			parts = append(parts, "in "+describeField(month, "month", monthNames))
		}
	}

	return strings.Join(parts, " ")
}

func describeTime(minute, hour string) string {
	m, mErr := strconv.Atoi(minute)
	h, hErr := strconv.Atoi(hour)

	switch {
	case mErr == nil && hErr == nil:
		return fmt.Sprintf("At %02d:%02d", h, m)
	case minute == "*" && hour == "*":
		return "Every minute"
	case hour == "*" && strings.HasPrefix(minute, "*/"):
		return "Every " + strings.TrimPrefix(minute, "*/") + " minutes"
	case hour == "*":
		return "At minute " + describeField(minute, "minute", nil) + " of every hour"
	case minute == "*":
		return "Every minute during hour " + describeField(hour, "hour", nil)
	case mErr == nil:
		return fmt.Sprintf("At minute %d of hour %s", m, describeField(hour, "hour", nil))
	default:
		return fmt.Sprintf("At minute %s of hour %s",
			describeField(minute, "minute", nil), describeField(hour, "hour", nil))
	}
}

// describeField renders one cron field (lists, ranges, steps) in words,
// substituting names (months, weekdays) when provided.
func describeField(field, unit string, names []string) string {
	items := strings.Split(field, ",")
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, describeItem(item, unit, names))
	}

	switch len(out) {
	case 1:
		return out[0]
	case 2:
		return out[0] + " and " + out[1]
	default:
		return strings.Join(out[:len(out)-1], ", ") + ", and " + out[len(out)-1]
	}
}

func describeItem(item, unit string, names []string) string {
	base, step, hasStep := strings.Cut(item, "/")

	var text string
	switch {
	case base == "*":
		text = "every " + unit
	case strings.Contains(base, "-"):
		lo, hi, _ := strings.Cut(base, "-")
		text = nameOf(lo, names) + " through " + nameOf(hi, names)
	default:
		text = nameOf(base, names)
	}

	if !hasStep {
		return text
	}
	if base == "*" {
		return fmt.Sprintf("every %s %ss", step, unit)
	}
	return fmt.Sprintf("every %s %ss from %s", step, unit, text)
}

func nameOf(value string, names []string) string {
	n, err := strconv.Atoi(value)
	if err != nil {
		// Three-letter names such as MON or JAN.
		for i, name := range names {
			if len(name) >= 3 && strings.EqualFold(name[:3], value) && name != "" {
				return names[i]
			}
		}
		return value
	}
	if err != nil || n < 0 || n >= len(names) {
		return value
	}
	return names[n]
}
