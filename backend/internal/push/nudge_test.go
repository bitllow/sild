package push_test

import (
	"testing"

	"github.com/bitllow/sild/backend/internal/push"
)

// The two settings are independent, so all four combinations must produce
// distinct, sensible notification text — "show who, hide what" is a real
// position and one flag could not express it.
func TestComposeCoversEverySettingCombination(t *testing.T) {
	cases := []struct {
		name      string
		s         push.Settings
		wantTitle string
		wantBody  string
	}{
		{"nothing revealed", push.Settings{}, "New message", ""},
		{"body only", push.Settings{IncludeBody: true}, "New message", "running late"},
		{"sender only", push.Settings{IncludeSender: true}, "Alice", "New message"},
		{"both", push.Settings{IncludeSender: true, IncludeBody: true}, "Alice", "running late"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			title, body := push.Compose(tc.s, "Alice", "running late")
			if title != tc.wantTitle || body != tc.wantBody {
				t.Fatalf("Compose = (%q, %q), want (%q, %q)", title, body, tc.wantTitle, tc.wantBody)
			}
		})
	}
}

// A sender the tenant wants named but whom we cannot name must not produce an
// empty title.
func TestComposeFallsBackWhenTheSenderIsUnknown(t *testing.T) {
	title, _ := push.Compose(push.Settings{IncludeSender: true}, "", "hello")
	if title != "New message" {
		t.Fatalf("title with no sender = %q", title)
	}
}

// An empty message body (an attachment-only message) must not leave the
// notification repeating itself.
func TestComposeDoesNotRepeatItself(t *testing.T) {
	title, body := push.Compose(push.Settings{IncludeBody: true}, "", "")
	if title == body {
		t.Fatalf("title and body are both %q", title)
	}
}

func TestComposeTrimsALongBody(t *testing.T) {
	long := make([]rune, 500)
	for i := range long {
		long[i] = 'a'
	}
	_, body := push.Compose(push.Settings{IncludeBody: true}, "Alice", string(long))
	if len([]rune(body)) >= 500 {
		t.Fatalf("body was not trimmed: %d runes", len([]rune(body)))
	}
}
