package window

import (
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func validInput() Input {
	return Input{Content: "public experiment", Inception: "2026-11-04T13:00:00Z", Expiration: "2026-11-04T14:00:00Z"}
}

func TestInputRejectsInvalidBoundaries(t *testing.T) {
	for _, change := range []struct {
		name   string
		change func(*Input)
	}{
		{"large content", func(in *Input) { in.Content = strings.Repeat("a", 513) }},
		{"large UTF-8 content", func(in *Input) { in.Content = strings.Repeat("é", 257) }},
		{"invalid UTF-8", func(in *Input) { in.Content = string([]byte{255}) }},
		{"local inception", func(in *Input) { in.Inception = "2026-11-04T13:00:00+00:00" }},
		{"fractional inception", func(in *Input) { in.Inception = "2026-11-04T13:00:00.1Z" }},
		{"invalid inception", func(in *Input) { in.Inception = "tomorrow" }},
		{"local expiration", func(in *Input) { in.Expiration = "2026-11-04T14:00:00-05:00" }},
		{"equal boundaries", func(in *Input) { in.Expiration = in.Inception }},
		{"reversed boundaries", func(in *Input) { in.Expiration = "2026-11-04T12:00:00Z" }},
		{"negative serial time", func(in *Input) { in.Inception = "1969-12-31T23:59:59Z" }},
		{"overflow serial time", func(in *Input) { in.Expiration = "2106-02-07T06:28:16Z" }},
		{"ambiguous duration", func(in *Input) { in.Expiration = "2094-11-23T16:14:08Z" }},
		{"ambiguous clock distance", func(in *Input) { in.Inception = "2095-01-01T00:00:00Z"; in.Expiration = "2095-01-01T01:00:00Z" }},
		{"ambiguous expiration distance", func(in *Input) { in.Inception = "2094-01-01T00:00:00Z"; in.Expiration = "2095-01-01T00:00:00Z" }},
	} {
		t.Run(change.name, func(t *testing.T) {
			in := validInput()
			change.change(&in)
			if err := in.Validate(testNow); err == nil {
				t.Fatal("invalid input passes validation")
			}
		})
	}
}

func TestValidationErrorNamesTheField(t *testing.T) {
	in := validInput()
	in.Content = strings.Repeat("x", 513)
	err := in.Validate(testNow)
	if err == nil || err.Error() != "content: content requires at most 512 UTF-8 bytes" {
		t.Fatal(err)
	}
}

func TestInputAcceptsSupportedBounds(t *testing.T) {
	for _, pair := range []struct {
		in  Input
		now time.Time
	}{
		{validInput(), testNow},
		{Input{"", "2026-11-04T13:00:00Z", "2026-11-04T14:00:00Z"}, testNow},
		{Input{strings.Repeat("é", 256), "2026-11-04T13:00:00Z", "2026-11-04T14:00:00Z"}, testNow},
		{Input{"public", "1970-01-01T00:00:00Z", "1970-01-01T00:00:01Z"}, time.Unix(0, 0)},
		{Input{"public", "2106-02-07T06:28:14Z", "2106-02-07T06:28:15Z"}, time.Unix(4294967295, 0)},
	} {
		if err := pair.in.Validate(pair.now); err != nil {
			t.Fatal(err)
		}
	}
}
