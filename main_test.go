package main

import "testing"

func TestParseArgs(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want options
	}{
		{nil, options{}},
		{[]string{"--cue"}, options{cue: true}},
		{[]string{"--background"}, options{background: true}},
		{[]string{"--background", "--cue"}, options{cue: true, background: true}},
		{[]string{"--cue", "extra"}, options{cue: true}},
	} {
		if got := parseArgs(tc.args); got != tc.want {
			t.Errorf("parseArgs(%q) = %+v, want %+v", tc.args, got, tc.want)
		}
	}
}
