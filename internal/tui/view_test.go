package tui

import "testing"

func TestLocalImage(t *testing.T) {
	for _, tc := range []struct {
		dest, want string
		ok         bool
	}{
		{"assets/a.png", "/n/topic/assets/a.png", true},
		{"assets/a%20b.png", "/n/topic/assets/a b.png", true},
		{"../img/x.jpg", "/n/img/x.jpg", true},
		{"/abs/x.png", "/abs/x.png", true},
		{"https://example.com/x.png", "", false},
		{"data:image/png;base64,AAAA", "", false},
		{"", "", false},
	} {
		got, ok := localImage("/n/topic/note.md", tc.dest)
		if got != tc.want || ok != tc.ok {
			t.Errorf("%q: got %q %v, want %q %v", tc.dest, got, ok, tc.want, tc.ok)
		}
	}
}
