package job

import "testing"

func TestDescribeSchedule(t *testing.T) {
	for schedule, want := range map[string]string{
		"30 9 * * *":        "At 09:30 every day",
		"30 9 * * 1-5":      "At 09:30 on Monday through Friday",
		"*/15 * * * *":      "Every 15 minutes",
		"* * * * *":         "Every minute",
		"0 0 1 * *":         "At 00:00 on day-of-month 1",
		"0 12 * 1,6 *":      "At 12:00 in January and June",
		"5 4 * * 0":         "At 04:05 on Sunday",
		"0 8-17 * * 1,3,5":  "At minute 0 of hour 8 through 17 on Monday, Wednesday, and Friday",
		"@daily":            "At 00:00 every day",
		"@reboot":           "At system startup",
		"0 9 * jan mon-fri": "At 09:00 on Monday through Friday in January",
		"bogus":             "",
	} {
		if got := DescribeSchedule(schedule); got != want {
			t.Errorf("DescribeSchedule(%q) = %q, want %q", schedule, got, want)
		}
	}
}
