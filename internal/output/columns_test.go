package output

import (
	"encoding/json"
	"testing"
)

// The API sends TimeEntry.duration in seconds: the spec examples of
// /tracker/time_entries have 13:44–16:44 = 10800 and 10:20–12:00 = 6000.
func TestFmtDurationHM(t *testing.T) {
	for in, want := range map[any]string{
		json.Number("10800"): "3:00",
		json.Number("6000"):  "1:40",
		json.Number("0"):     "0:00",
		json.Number("59"):    "0:00", // whole minutes, truncated
		json.Number("3599"):  "0:59",
		json.Number("36000"): "10:00",
		json.Number("-5400"): "-1:30",
		json.Number("-59"):   "0:00", // no "-0:00"
		"5400":               "1:30",
		int64(90061):         "25:01",
		json.Number("1.5"):   "1.5", // not an integer: as given
		"abc":                "abc",
		nil:                  "",
	} {
		if got := FmtDurationHM(in); got != want {
			t.Errorf("FmtDurationHM(%#v) = %q, want %q", in, got, want)
		}
	}
}

// Amounts keep the literal of the API: no float formatting, no rounding.
func TestFmtMoney(t *testing.T) {
	for in, want := range map[any]string{
		json.Number("1190.00"): "1190.00",
		json.Number("19.99"):   "19.99",
		json.Number("1e3"):     "1e3",
		"0.10":                 "0.10",
		nil:                    "",
	} {
		if got := FmtMoney(in); got != want {
			t.Errorf("FmtMoney(%#v) = %q, want %q", in, got, want)
		}
	}
}

// Date-times keep the wall clock of their API offset (no conversion to the
// local time zone); unparsable values are shown as given.
func TestFmtDate(t *testing.T) {
	for in, want := range map[any]string{
		"2026-08-28":                    "2026-08-28",
		"2026-08-28T13:44:00.000+02:00": "2026-08-28 13:44",
		"2026-10-25T23:59:59+01:00":     "2026-10-25 23:59",
		"2026-01-02T03:04:05Z":          "2026-01-02 03:04",
		"28.08.2026":                    "28.08.2026",
		"":                              "",
		nil:                             "",
		json.Number("20260828"):         "20260828",
	} {
		if got := FmtDate(in); got != want {
			t.Errorf("FmtDate(%#v) = %q, want %q", in, got, want)
		}
	}
}
