// Package hashedtimerwheel implements a hashed timer wheel: a data structure
// for scheduling a large number of callbacks, where starting and cancelling a
// timer are both O(1).
//
// The wheel does not own a clock. Callers advance time explicitly by calling
// Advance. This keeps the structure deterministic and testable, and lets an
// application drive it from whatever source of time it already has (a real
// ticker, a simulated clock, a game loop).
package hashedtimerwheel

import (
	"errors"
	"time"
)

// ErrBadSlotCount is returned by New when the number of slots is not a power of
// two. The wheel relies on a power-of-two slot count so that the slot index can
// be computed with a cheap bitmask instead of a division, which is what keeps
// insertion O(1) in practice as well as in theory.
var ErrBadSlotCount = errors.New("hashedtimerwheel: slots must be a power of two")

// ErrBadTick is returned by New when the tick duration is not positive.
var ErrBadTick = errors.New("hashedtimerwheel: tick must be positive")

// timer is a single scheduled callback. It is stored in a doubly linked list so
// that cancellation can unlink it in O(1) without scanning the wheel.
type timer struct {
	expires  int64 // absolute tick at which the callback fires
	slot     int   // index of the slot currently holding this timer
	callback func()
	prev     *timer
	next     *timer
	set      bool // false once cancelled or fired; guards against double-cancel
}

// Wheel is a hashed timer wheel. The zero value is not usable; construct one
// with New.
//
// Timers are keyed by a caller-supplied uint64. Starting a timer with a key that
// is already scheduled replaces the existing timer: the old callback will not
// fire, and the new expiry takes its place. This is the behaviour most callers
// want for "reset this deadline" and it is what makes Start idempotent per key.
type Wheel struct {
	slots    []*timer // each slot is the head of a doubly linked list
	mask     int      // len(slots)-1, valid because len is a power of two
	tick     time.Duration
	now      int64 // current tick
	pending  int   // number of live timers, for Len
	byKey    map[uint64]*timer
}

// New returns a Wheel with the given number of slots and tick duration. slots
// must be a power of two and tick must be positive.
func New(slots int, tick time.Duration) (*Wheel, error) {
	if slots <= 0 || slots&(slots-1) != 0 {
		return nil, ErrBadSlotCount
	}
	if tick <= 0 {
		return nil, ErrBadTick
	}
	return &Wheel{
		slots: make([]*timer, slots),
		mask:  slots - 1,
		tick:  tick,
		byKey: make(map[uint64]*timer),
	}, nil
}

// Tick returns the wheel's tick duration.
func (w *Wheel) Tick() time.Duration { return w.tick }

// Now returns the current tick count. It starts at zero and increases by one
// per tick advanced.
func (w *Wheel) Now() int64 { return w.now }

// Len returns the number of timers currently scheduled.
func (w *Wheel) Len() int { return w.pending }

// Start schedules callback to fire after d. If d is not positive the callback
// is scheduled for the next tick boundary, so it will fire on the next Advance.
//
// Starting with a key that is already scheduled replaces the previous timer.
func (w *Wheel) Start(key uint64, d time.Duration, callback func()) {
	if callback == nil {
		return
	}
	w.Cancel(key)

	ticks := int64(d / w.tick)
	if d%w.tick != 0 {
		// Round up: a timer must never fire before its requested delay has
		// actually elapsed.
		ticks++
	}
	if ticks < 1 {
		ticks = 1
	}

	t := &timer{
		expires:  w.now + ticks,
		callback: callback,
		set:      true,
	}
	t.slot = int(t.expires & int64(w.mask))
	w.link(t)
	w.byKey[key] = t
}

// Cancel removes the timer with the given key. It reports whether a timer was
// actually removed.
func (w *Wheel) Cancel(key uint64) bool {
	t, ok := w.byKey[key]
	if !ok {
		return false
	}
	delete(w.byKey, key)
	w.unlink(t)
	t.set = false
	return true
}

// Advance moves the wheel forward by n ticks, firing every callback whose
// deadline falls on or before the new current tick. Callbacks are invoked in
// the order their slots are visited, which is not necessarily the order they
// were scheduled.
//
// A callback that calls Start or Cancel during Advance is safe: the wheel is in
// a consistent state at the moment the callback runs.
func (w *Wheel) Advance(n int64) {
	for i := int64(0); i < n; i++ {
		w.now++
		slot := int(w.now & int64(w.mask))
		w.fireSlot(slot)
	}
}

// fireSlot drains every timer in the given slot whose deadline has arrived.
func (w *Wheel) fireSlot(slot int) {
	head := w.slots[slot]
	for head != nil {
		next := head.next
		if head.expires <= w.now {
			w.unlink(head)
			head.set = false
			w.forget(head)
			if head.callback != nil {
				head.callback()
			}
		}
		head = next
	}
}

// forget removes a fired timer from the key index. We do not know the key here,
// so we scan the map only when it is small enough to matter; to avoid that scan
// entirely we instead rely on Start overwriting stale entries. This helper
// exists to keep fireSlot readable.
func (w *Wheel) forget(t *timer) {
	// The map entry for a fired timer is removed lazily: Start and Cancel both
	// consult the map, and a stale entry would make Cancel report true for a
	// timer that already fired. We therefore remove it eagerly by scanning.
	// The scan is O(len(byKey)) but only runs for timers that actually fire.
	for k, v := range w.byKey {
		if v == t {
			delete(w.byKey, k)
			return
		}
	}
}

// link prepends t to its slot's list.
func (w *Wheel) link(t *timer) {
	head := w.slots[t.slot]
	t.prev = nil
	t.next = head
	if head != nil {
		head.prev = t
	}
	w.slots[t.slot] = t
	w.pending++
}

// unlink removes t from its slot's list.
func (w *Wheel) unlink(t *timer) {
	if t.prev != nil {
		t.prev.next = t.next
	} else {
		w.slots[t.slot] = t.next
	}
	if t.next != nil {
		t.next.prev = t.prev
	}
	t.prev = nil
	t.next = nil
	if t.set {
		w.pending--
	}
}
