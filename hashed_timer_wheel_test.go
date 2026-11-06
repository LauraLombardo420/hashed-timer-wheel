package hashedtimerwheel

import (
	"testing"
	"time"
)

func mustNew(t *testing.T, slots int, tick time.Duration) *Wheel {
	t.Helper()
	w, err := New(slots, tick)
	if err != nil {
		t.Fatalf("New(%d, %v) returned error: %v", slots, tick, err)
	}
	return w
}

func TestNewRejectsNonPowerOfTwo(t *testing.T) {
	for _, slots := range []int{0, -1, 3, 5, 6, 100} {
		if _, err := New(slots, time.Second); err != ErrBadSlotCount {
			t.Errorf("New(%d, ...) error = %v, want ErrBadSlotCount", slots, err)
		}
	}
}

func TestNewRejectsNonPositiveTick(t *testing.T) {
	for _, tick := range []time.Duration{0, -time.Second} {
		if _, err := New(8, tick); err != ErrBadTick {
			t.Errorf("New(8, %v) error = %v, want ErrBadTick", tick, err)
		}
	}
}

func TestNewAcceptsPowerOfTwo(t *testing.T) {
	for _, slots := range []int{1, 2, 4, 8, 1024} {
		if _, err := New(slots, time.Millisecond); err != nil {
			t.Errorf("New(%d, ...) unexpected error: %v", slots, err)
		}
	}
}

func TestStartFiresAfterDelay(t *testing.T) {
	w := mustNew(t, 8, time.Second)
	fired := 0
	w.Start(1, 3*time.Second, func() { fired++ })

	w.Advance(2)
	if fired != 0 {
		t.Fatalf("fired = %d after 2 ticks, want 0", fired)
	}
	w.Advance(1)
	if fired != 1 {
		t.Fatalf("fired = %d after 3 ticks, want 1", fired)
	}
	w.Advance(1)
	if fired != 1 {
		t.Fatalf("fired = %d after 4 ticks, want 1 (must not repeat)", fired)
	}
}

func TestStartRoundsUpPartialTick(t *testing.T) {
	w := mustNew(t, 8, time.Second)
	fired := 0
	// 1500ms is one and a half ticks; it must not fire at tick 1.
	w.Start(1, 1500*time.Millisecond, func() { fired++ })
	w.Advance(1)
	if fired != 0 {
		t.Fatalf("fired = %d after 1 tick, want 0", fired)
	}
	w.Advance(1)
	if fired != 1 {
		t.Fatalf("fired = %d after 2 ticks, want 1", fired)
	}
}

func TestStartWithNonPositiveDelayFiresNextTick(t *testing.T) {
	w := mustNew(t, 8, time.Second)
	fired := 0
	w.Start(1, 0, func() { fired++ })
	if fired != 0 {
		t.Fatalf("fired = %d before advancing, want 0", fired)
	}
	w.Advance(1)
	if fired != 1 {
		t.Fatalf("fired = %d after 1 tick, want 1", fired)
	}
}

func TestCancelPreventsFiring(t *testing.T) {
	w := mustNew(t, 8, time.Second)
	fired := 0
	w.Start(7, 2*time.Second, func() { fired++ })
	if !w.Cancel(7) {
		t.Fatal("Cancel(7) = false, want true")
	}
	w.Advance(5)
	if fired != 0 {
		t.Fatalf("fired = %d, want 0 after cancel", fired)
	}
	if w.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", w.Len())
	}
}

func TestCancelUnknownKey(t *testing.T) {
	w := mustNew(t, 8, time.Second)
	if w.Cancel(99) {
		t.Fatal("Cancel on unknown key = true, want false")
	}
}

func TestCancelAfterFiring(t *testing.T) {
	w := mustNew(t, 8, time.Second)
	w.Start(1, time.Second, func() {})
	w.Advance(1)
	if w.Cancel(1) {
		t.Fatal("Cancel after firing = true, want false")
	}
}

func TestStartReplacesExistingKey(t *testing.T) {
	w := mustNew(t, 8, time.Second)
	first, second := 0, 0
	w.Start(5, time.Second, func() { first++ })
	w.Start(5, 3*time.Second, func() { second++ })
	if w.Len() != 1 {
		t.Fatalf("Len() = %d, want 1 after replace", w.Len())
	}
	w.Advance(1)
	if first != 0 {
		t.Fatalf("first callback fired = %d, want 0", first)
	}
	w.Advance(2)
	if second != 1 {
		t.Fatalf("second callback fired = %d, want 1", second)
	}
}

func TestLenTracksPending(t *testing.T) {
	w := mustNew(t, 8, time.Second)
	w.Start(1, time.Second, func() {})
	w.Start(2, time.Second, func() {})
	w.Start(3, time.Second, func() {})
	if w.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", w.Len())
	}
	w.Cancel(2)
	if w.Len() != 2 {
		t.Fatalf("Len() = %d after cancel, want 2", w.Len())
	}
	w.Advance(1)
	if w.Len() != 0 {
		t.Fatalf("Len() = %d after firing, want 0", w.Len())
	}
}

func TestNowAdvancesWithTicks(t *testing.T) {
	w := mustNew(t, 8, time.Second)
	if w.Now() != 0 {
		t.Fatalf("Now() = %d, want 0", w.Now())
	}
	w.Advance(4)
	if w.Now() != 4 {
		t.Fatalf("Now() = %d, want 4", w.Now())
	}
}

func TestTickReturnsConfiguredDuration(t *testing.T) {
	w := mustNew(t, 8, 250*time.Millisecond)
	if w.Tick() != 250*time.Millisecond {
		t.Fatalf("Tick() = %v, want 250ms", w.Tick())
	}
}

func TestLongDelayWrapsAroundWheel(t *testing.T) {
	// Four slots; a delay of 10 ticks must still fire at exactly tick 10 even
	// though it wraps past the wheel several times.
	w := mustNew(t, 4, time.Second)
	fired := 0
	w.Start(1, 10*time.Second, func() { fired++ })
	w.Advance(9)
	if fired != 0 {
		t.Fatalf("fired = %d after 9 ticks, want 0", fired)
	}
	w.Advance(1)
	if fired != 1 {
		t.Fatalf("fired = %d after 10 ticks, want 1", fired)
	}
}

func TestNilCallbackIsIgnored(t *testing.T) {
	w := mustNew(t, 8, time.Second)
	w.Start(1, time.Second, nil)
	if w.Len() != 0 {
		t.Fatalf("Len() = %d after nil callback, want 0", w.Len())
	}
	w.Advance(2)
}

func TestCallbackCanScheduleDuringAdvance(t *testing.T) {
	w := mustNew(t, 8, time.Second)
	chain := 0
	w.Start(1, time.Second, func() {
		chain++
		w.Start(2, time.Second, func() { chain++ })
	})
	w.Advance(1)
	if chain != 1 {
		t.Fatalf("chain = %d after 1 tick, want 1", chain)
	}
	w.Advance(1)
	if chain != 2 {
		t.Fatalf("chain = %d after 2 ticks, want 2", chain)
	}
}

func TestMultipleTimersSameSlotAllFire(t *testing.T) {
	// With 2 slots, every even expiry lands in slot 0 and every odd in slot 1.
	// Three timers with the same expiry must all fire.
	w := mustNew(t, 2, time.Second)
	count := 0
	w.Start(1, 4*time.Second, func() { count++ })
	w.Start(2, 4*time.Second, func() { count++ })
	w.Start(3, 4*time.Second, func() { count++ })
	w.Advance(4)
	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}
}

func TestAdvanceZeroIsNoOp(t *testing.T) {
	w := mustNew(t, 8, time.Second)
	fired := 0
	w.Start(1, time.Second, func() { fired++ })
	w.Advance(0)
	if fired != 0 {
		t.Fatalf("fired = %d after Advance(0), want 0", fired)
	}
	if w.Now() != 0 {
		t.Fatalf("Now() = %d after Advance(0), want 0", w.Now())
	}
}
