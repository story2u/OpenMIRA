package senddispatcher

import "testing"

// TestActiveDeviceSetFiltersAndReleasesDevices mirrors local coordinator behavior.
func TestActiveDeviceSetFiltersAndReleasesDevices(t *testing.T) {
	set := NewActiveDeviceSet()
	if !set.TryAcquire(" zimo ") {
		t.Fatal("initial acquire failed")
	}
	if set.TryAcquire("zimo") {
		t.Fatal("duplicate acquire succeeded")
	}
	idle := set.IdleDevices([]string{" zimo ", "ada", "", " bob "})
	if len(idle) != 2 || idle[0] != "ada" || idle[1] != "bob" {
		t.Fatalf("idle devices = %#v", idle)
	}
	set.Release("zimo")
	if !set.TryAcquire("zimo") {
		t.Fatal("acquire after release failed")
	}
	if set.TryAcquire(" ") {
		t.Fatal("empty device acquired")
	}
}

// TestNilActiveDeviceSetKeepsFilteringPure allows optional coordinator wiring.
func TestNilActiveDeviceSetKeepsFilteringPure(t *testing.T) {
	var set *ActiveDeviceSet
	idle := set.IdleDevices([]string{" zimo ", "", "ada"})
	if len(idle) != 2 || idle[0] != "zimo" || idle[1] != "ada" {
		t.Fatalf("idle devices = %#v", idle)
	}
	if set.TryAcquire("zimo") {
		t.Fatal("nil set acquired device")
	}
	set.Release("zimo")
}
