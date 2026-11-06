# hashed-timer-wheel

A timer wheel that schedules callbacks with O(1) start and cancel, advanced by an external clock you drive yourself.

```go
package main

import (
	"fmt"
	"time"

	hashedtimerwheel "example.com/hashed-timer-wheel"
)

func main() {
	w, err := hashedtimerwheel.New(64, time.Second)
	if err != nil {
		panic(err)
	}

	w.Start(1, 3*time.Second, func() { fmt.Println("timer 1 fired") })
	w.Start(2, 10*time.Second, func() { fmt.Println("timer 2 fired") })

	// Somewhere else, driven by a real ticker, a game loop, or a fake clock:
	for i := 0; i < 10; i++ {
		w.Advance(1)
	}

	// Cancel is O(1) and reports whether anything was removed.
	w.Start(3, time.Minute, func() { fmt.Println("never runs") })
	fmt.Println(w.Cancel(3)) // true
}
```

## Why this exists

A heap gives you O(log n) insert and O(log n) cancel, and cancel is the awkward one: you need an index from key to heap position, which the standard library's `container/heap` does not give you. A timer wheel trades that for O(1) on both, at the cost of resolution. The wheel only knows about whole ticks, so a timer requested for 1500ms on a 1s wheel fires at tick 2, not at 1500ms. If you need exact sub-tick deadlines this is the wrong structure.

The slot index is `expires & (slots-1)`, which is why the slot count must be a power of two: the mask replaces a division. A timer that expires further away than one lap of the wheel simply wraps, and the expiry tick is stored on the timer itself so it still fires at the right tick.

## The awkward edge

The wheel has no goroutine and no clock. `Advance(n)` is the only thing that moves time, and it fires callbacks synchronously, one slot at a time. Two consequences are worth stating plainly:

- Callbacks fire in slot order, not in the order they were scheduled. If you need strict ordering among timers that expire on the same tick, impose it yourself.
- `Advance` is not safe to call concurrently with itself or with `Start`/`Cancel`. Wrap the wheel in a mutex if more than one goroutine touches it.

Starting a timer with a key that is already live replaces the existing timer: the old callback will not fire. This makes `Start` idempotent per key and is what you want for "reset this deadline", but it means two timers cannot share a key.

## API

- `New(slots int, tick time.Duration) (*Wheel, error)` — `slots` must be a power of two, `tick` must be positive.
- `(*Wheel) Start(key uint64, d time.Duration, callback func())` — schedules `callback` to fire after `d`, rounded up to whole ticks. A non-positive `d` fires on the next tick. A nil callback is ignored.
- `(*Wheel) Cancel(key uint64) bool` — removes the timer, reporting whether one was present.
- `(*Wheel) Advance(n int64)` — moves time forward by `n` ticks, firing everything due.
- `(*Wheel) Now() int64` — current tick, starting at zero.
- `(*Wheel) Len() int` — number of live timers.
- `(*Wheel) Tick() time.Duration` — the configured tick.
- `ErrBadSlotCount`, `ErrBadTick` — returned by `New`.
