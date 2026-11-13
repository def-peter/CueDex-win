package instance

import "testing"

func TestAcquireAllowsOneProcessPerName(t *testing.T) {
	name := `Local\CueDex-test-single-instance`

	release, first := Acquire(name)
	if !first {
		t.Fatal("the first acquire was refused")
	}
	if _, again := Acquire(name); again {
		t.Fatal("a second acquire of the same name was allowed")
	}
	release()

	release, first = Acquire(name)
	if !first {
		t.Fatal("the name was not free after the first release")
	}
	release()
}
