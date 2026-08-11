package gt

import "context"

// maxConcurrentSpawns caps how many `gt` processes slis may run at once, across
// every caller in the binary. gt is expensive — a Graphite CLI invocation is a
// process that runs its own git children — and an unbounded fan-out over a
// multi-repo workspace once left ~30 gt processes contending for the machine.
// Every spawn in this package passes through the gate, so no caller (present or
// future) can burst past the cap; the worst it can do is queue.
const maxConcurrentSpawns = 4

var spawnSlots = make(chan struct{}, maxConcurrentSpawns)

// acquireSpawnSlot blocks until a spawn slot is free and returns the release
// func. Callers hold the slot for the lifetime of the gt process:
//
//	defer acquireSpawnSlot()()
//
// Acquire BEFORE building a timeout context so queue time is not charged against
// the process's own deadline.
func acquireSpawnSlot() (release func()) {
	spawnSlots <- struct{}{}
	return func() { <-spawnSlots }
}

// acquireSpawnSlotCtx is acquireSpawnSlot that gives up when ctx is done, so a
// cancelled read waiting in the queue never starts a gt process at all. ok is
// false when the context ended first (no slot held, nothing to release).
func acquireSpawnSlotCtx(ctx context.Context) (release func(), ok bool) {
	// Check first: with a free slot AND a cancelled context, select would pick
	// either case at random and could start work for a caller that is already gone.
	if ctx.Err() != nil {
		return func() {}, false
	}
	select {
	case spawnSlots <- struct{}{}:
		return func() { <-spawnSlots }, true
	case <-ctx.Done():
		return func() {}, false
	}
}
