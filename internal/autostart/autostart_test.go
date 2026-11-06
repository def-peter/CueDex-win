package autostart

import (
	"testing"
)

// scratch returns an entry under a test-only key, so the suite never touches
// the Run entry a real session reads.
func scratch(t *testing.T) Entry {
	t.Helper()
	return Entry{
		Key:   `Software\CueDex\Tests\Autostart`,
		Value: "CueDexTest",
	}
}

func TestCommandQuotesThePathAndStartsInTheBackground(t *testing.T) {
	got := Command(`C:\Program Files\CueDex\CueDex.exe`)
	want := `"C:\Program Files\CueDex\CueDex.exe" --background`
	if got != want {
		t.Fatalf("command %q, want %q", got, want)
	}
}

func TestEnableWritesTheEntry(t *testing.T) {
	entry := scratch(t)
	t.Cleanup(func() { _ = entry.Disable() })
	if err := entry.Disable(); err != nil {
		t.Fatalf("clearing the scratch entry: %v", err)
	}
	exe := `C:\Apps\CueDex.exe`
	if on, err := entry.Enabled(exe); err != nil || on {
		t.Fatalf("before enabling: on = %v, err = %v", on, err)
	}
	if err := entry.Enable(exe); err != nil {
		t.Fatal(err)
	}
	if on, err := entry.Enabled(exe); err != nil || !on {
		t.Fatalf("after enabling: on = %v, err = %v", on, err)
	}
	// A different executable means the entry is stale: it would start the old
	// copy, so the panel shows it as off.
	if on, err := entry.Enabled(`D:\Elsewhere\CueDex.exe`); err != nil || on {
		t.Fatalf("for another executable: on = %v, err = %v", on, err)
	}
	if err := entry.Disable(); err != nil {
		t.Fatal(err)
	}
	if on, err := entry.Enabled(exe); err != nil || on {
		t.Fatalf("after disabling: on = %v, err = %v", on, err)
	}
}

func TestDisableIgnoresAMissingEntry(t *testing.T) {
	entry := scratch(t)
	if err := entry.Disable(); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := entry.Disable(); err != nil {
		t.Fatalf("second: %v", err)
	}
}

func TestSameCommandIgnoresCaseAndTheFlag(t *testing.T) {
	cases := []struct {
		command string
		exe     string
		want    bool
	}{
		{`"C:\Apps\CueDex.exe" --background`, `C:\Apps\CueDex.exe`, true},
		{`"c:\apps\cuedex.exe" --background`, `C:\Apps\CueDex.exe`, true},
		{`C:\Apps\CueDex.exe --background`, `C:\Apps\CueDex.exe`, true},
		{`"C:\Apps\CueDex.exe"`, `C:\Apps\CueDex.exe`, true},
		{`"C:\Apps\Other.exe" --background`, `C:\Apps\CueDex.exe`, false},
		{``, `C:\Apps\CueDex.exe`, false},
	}
	for _, c := range cases {
		if got := sameCommand(c.command, c.exe); got != c.want {
			t.Errorf("sameCommand(%q, %q) = %v, want %v", c.command, c.exe, got, c.want)
		}
	}
}
