package types

import (
	"fmt"
	"time"
)

var taggedInstFormats = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.000-07:00",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

func ParseInstString(s string) (Time, error) {
	for _, f := range taggedInstFormats {
		if t, err := time.Parse(f, s); err == nil {
			return Time{T: t}, nil
		}
	}
	return Time{}, fmt.Errorf("Cannot parse #inst %q", s)
}

func ValidateUUIDString(s string) error {
	if len(s) != 36 {
		return fmt.Errorf("Invalid UUID format: %q", s)
	}
	return nil
}
