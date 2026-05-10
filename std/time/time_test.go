package time

import (
	"testing"
	"time"
)

func TestTimeTimezoneHelpers(t *testing.T) {
	utc := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	lisbon := inTimezone(utc, "Europe/Lisbon")
	if lisbon.Location().String() != "Europe/Lisbon" {
		t.Fatalf("location mismatch: %s", lisbon.Location())
	}
	parsed := parseInTimezone("2006-01-02 15:04", "2026-05-10 12:00", "Europe/Lisbon")
	if parsed.Location().String() != "Europe/Lisbon" || parsed.Hour() != 12 {
		t.Fatalf("parseInTimezone mismatch: %s", parsed)
	}
}
