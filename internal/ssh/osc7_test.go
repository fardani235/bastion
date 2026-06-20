package ssh

import "testing"

func TestParseOSC7(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantDir string
		wantOK  bool
	}{
		{
			name:    "BEL terminator",
			in:      "prompt\x1b]7;file://host/home/alice\arest",
			wantDir: "/home/alice",
			wantOK:  true,
		},
		{
			name:    "ST terminator (ESC backslash)",
			in:      "\x1b]7;file://host/var/www\x1b\\",
			wantDir: "/var/www",
			wantOK:  true,
		},
		{
			name:    "percent-escaped path is decoded",
			in:      "\x1b]7;file://host/home/alice/my%20dir\a",
			wantDir: "/home/alice/my dir",
			wantOK:  true,
		},
		{
			name:    "last occurrence wins",
			in:      "\x1b]7;file://h/a\a junk \x1b]7;file://h/b\a",
			wantDir: "/b",
			wantOK:  true,
		},
		{
			name:    "no sequence",
			in:      "just some normal output\n",
			wantDir: "",
			wantOK:  false,
		},
		{
			name:    "partial sequence (no terminator yet) is ignored",
			in:      "\x1b]7;file://host/home/alice",
			wantDir: "",
			wantOK:  false,
		},
		{
			name:    "empty hostless path is ignored",
			in:      "\x1b]7;file://\a",
			wantDir: "",
			wantOK:  false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, ok := parseOSC7([]byte(tc.in))
			if ok != tc.wantOK || dir != tc.wantDir {
				t.Fatalf("parseOSC7(%q) = (%q, %v), want (%q, %v)", tc.in, dir, ok, tc.wantDir, tc.wantOK)
			}
		})
	}
}
