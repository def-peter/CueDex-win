package i18n

import "testing"

func TestResolvePrefersTheStoredChoice(t *testing.T) {
	for _, tc := range []struct {
		setting string
		system  Language
		want    Language
	}{
		{"zh", English, Chinese},
		{"en", Chinese, English},
		{"", Chinese, Chinese},
		{"", English, English},
		{"de", Chinese, Chinese},
	} {
		if got := ResolveWith(tc.setting, tc.system); got != tc.want {
			t.Errorf("ResolveWith(%q, %q) = %q, want %q", tc.setting, tc.system, got, tc.want)
		}
	}
}

func TestResolveAlwaysNamesALanguage(t *testing.T) {
	if got := Resolve(string(Chinese)); got != Chinese {
		t.Fatalf("resolved %q", got)
	}
	if got := Resolve(string(English)); got != English {
		t.Fatalf("resolved %q", got)
	}
	if got := Resolve(""); got != Chinese && got != English {
		t.Fatalf("the system language resolved to %q", got)
	}
}
