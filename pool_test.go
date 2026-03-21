//go:build js

package opfsvfs

import (
	"testing"
)

func TestPoolAcquireNewName(t *testing.T) {
	// Test that Acquire returns a handle for a new name (alloc)
	name := "test-pool-acquire-new"

	h, slot, err := globalPool.Acquire(name)
	if err != nil {
		t.Fatalf("Acquire(%q): %v", name, err)
	}
	if h == nil {
		t.Fatalf("Acquire(%q): got nil handle", name)
	}
	if slot < 1 {
		t.Fatalf("Acquire(%q): got slot %d, want >= 1 (slot 0 is reserved)", name, slot)
	}

	// Verify the pool knows about this name
	if !globalPool.Has(name) {
		t.Fatalf("Has(%q): returned false after Acquire", name)
	}

	// Cleanup
	if err := globalPool.Release(name); err != nil {
		t.Fatalf("Release(%q): %v", name, err)
	}
}

func TestPoolAcquireExistingName(t *testing.T) {
	// Test that Acquire returns the same handle for an existing name (hit)
	name := "test-pool-acquire-existing"

	// First acquire
	h1, slot1, err := globalPool.Acquire(name)
	if err != nil {
		t.Fatalf("Acquire(%q) first: %v", name, err)
	}

	// Second acquire - should return same slot
	h2, slot2, err := globalPool.Acquire(name)
	if err != nil {
		t.Fatalf("Acquire(%q) second: %v", name, err)
	}

	if slot1 != slot2 {
		t.Fatalf("Acquire(%q): got different slots: first=%d, second=%d", name, slot1, slot2)
	}
	if h1 != h2 {
		t.Fatalf("Acquire(%q): got different handles", name)
	}

	// Cleanup
	if err := globalPool.Release(name); err != nil {
		t.Fatalf("Release(%q): %v", name, err)
	}
}

func TestPoolReleaseAndReacquire(t *testing.T) {
	// Test that Release clears name, and Acquire for same name gets new slot
	name := "test-pool-release"

	// First acquire
	h1, slot1, err := globalPool.Acquire(name)
	if err != nil {
		t.Fatalf("Acquire(%q) first: %v", name, err)
	}

	// Write some data to verify it's cleared
	data := []byte("test data")
	if _, err := h1.Write(data, 0); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Release
	if err := globalPool.Release(name); err != nil {
		t.Fatalf("Release(%q): %v", name, err)
	}

	// Verify pool doesn't have the name anymore
	if globalPool.Has(name) {
		t.Fatalf("Has(%q): returned true after Release", name)
	}

	// Acquire again - might get same slot or different slot
	h2, slot2, err := globalPool.Acquire(name)
	if err != nil {
		t.Fatalf("Acquire(%q) second: %v", name, err)
	}

	// The slot might be the same or different, but the file should be empty (truncated to 0)
	size, err := h2.GetSize()
	if err != nil {
		t.Fatalf("GetSize: %v", err)
	}
	if size != 0 {
		t.Fatalf("GetSize after Release+Acquire: got %d, want 0 (file should be truncated)", size)
	}

	t.Logf("Release+Reacquire: first slot=%d, second slot=%d", slot1, slot2)

	// Cleanup
	if err := globalPool.Release(name); err != nil {
		t.Fatalf("Release(%q) cleanup: %v", name, err)
	}
}

func TestPoolFull(t *testing.T) {
	// Test that pool returns error after exhausting capacity
	// globalPool has 6 handles, slot 0 reserved = 5 usable slots
	const maxSlots = 5

	names := make([]string, 0, maxSlots+1)

	// Acquire all available slots
	for i := 0; i < maxSlots; i++ {
		name := "test-pool-full-" + string(rune('a'+i))
		names = append(names, name)

		_, _, err := globalPool.Acquire(name)
		if err != nil {
			t.Fatalf("Acquire slot %d (%q): %v", i, name, err)
		}
	}

	// Try to acquire one more - should fail
	overflowName := "test-pool-full-overflow"
	_, slot, err := globalPool.Acquire(overflowName)
	if err == nil {
		t.Fatalf("Acquire(%q): expected error when pool is full, got slot %d", overflowName, slot)
	}
	if slot != -1 {
		t.Fatalf("Acquire(%q) when full: got slot %d, want -1", overflowName, slot)
	}

	// Cleanup - release all acquired slots
	for _, name := range names {
		if err := globalPool.Release(name); err != nil {
			t.Errorf("Release(%q): %v", name, err)
		}
	}
}

func TestPoolMetadataPersistence(t *testing.T) {
	// Test that metadata survives simulated reload
	// Write metadata → create new Pool from same handles → verify names restored

	name1 := "test-meta-persist-1"
	name2 := "test-meta-persist-2"

	// Acquire two slots in the global pool
	h1, slot1, err := globalPool.Acquire(name1)
	if err != nil {
		t.Fatalf("Acquire(%q): %v", name1, err)
	}
	_, _, err = globalPool.Acquire(name2)
	if err != nil {
		t.Fatalf("Acquire(%q): %v", name2, err)
	}

	// Write some data to verify handles are correct
	data1 := []byte("metadata test 1")
	if _, err := h1.Write(data1, 0); err != nil {
		t.Fatalf("Write to slot %d: %v", slot1, err)
	}

	// Create a new pool using the same handles (simulating reload)
	reloadedPool := NewPool(globalPool.handles, nil)

	// Verify names are restored
	if !reloadedPool.Has(name1) {
		t.Fatalf("reloaded pool: Has(%q) = false, want true", name1)
	}
	if !reloadedPool.Has(name2) {
		t.Fatalf("reloaded pool: Has(%q) = false, want true", name2)
	}

	// Verify slots match
	h1Reloaded, slot1Reloaded, err := reloadedPool.Acquire(name1)
	if err != nil {
		t.Fatalf("reloaded pool: Acquire(%q): %v", name1, err)
	}
	if slot1Reloaded != slot1 {
		t.Fatalf("reloaded pool: Acquire(%q) got slot %d, want %d", name1, slot1Reloaded, slot1)
	}

	// Verify data is still there
	buf := make([]byte, len(data1))
	n, err := h1Reloaded.Read(buf, 0)
	if err != nil {
		t.Fatalf("Read from reloaded handle: %v", err)
	}
	if n != len(data1) {
		t.Fatalf("Read from reloaded handle: got %d bytes, want %d", n, len(data1))
	}
	if string(buf) != string(data1) {
		t.Fatalf("Read from reloaded handle: got %q, want %q", buf, data1)
	}

	// Cleanup using original global pool (reloadedPool shares the same handles)
	if err := globalPool.Release(name1); err != nil {
		t.Fatalf("Release(%q): %v", name1, err)
	}
	if err := globalPool.Release(name2); err != nil {
		t.Fatalf("Release(%q): %v", name2, err)
	}
}

func TestPoolStatsCounters(t *testing.T) {
	// Test that stats counters increment correctly

	// Reset stats to start fresh
	stats := globalPool.Stats()
	stats.Reset()

	snap := stats.Snapshot()
	if snap.SlotHits != 0 || snap.SlotAllocs != 0 || snap.SlotFull != 0 {
		t.Fatalf("Stats after Reset: SlotHits=%d, SlotAllocs=%d, SlotFull=%d; want all 0",
			snap.SlotHits, snap.SlotAllocs, snap.SlotFull)
	}

	// Test SlotAllocs
	name1 := "test-stats-alloc"
	_, _, err := globalPool.Acquire(name1)
	if err != nil {
		t.Fatalf("Acquire(%q): %v", name1, err)
	}

	snap = stats.Snapshot()
	if snap.SlotAllocs != 1 {
		t.Fatalf("After first Acquire: SlotAllocs=%d, want 1", snap.SlotAllocs)
	}
	if snap.SlotHits != 0 {
		t.Fatalf("After first Acquire: SlotHits=%d, want 0", snap.SlotHits)
	}

	// Test SlotHits
	_, _, err = globalPool.Acquire(name1) // same name
	if err != nil {
		t.Fatalf("Acquire(%q) second: %v", name1, err)
	}

	snap = stats.Snapshot()
	if snap.SlotAllocs != 1 {
		t.Fatalf("After second Acquire: SlotAllocs=%d, want 1", snap.SlotAllocs)
	}
	if snap.SlotHits != 1 {
		t.Fatalf("After second Acquire: SlotHits=%d, want 1", snap.SlotHits)
	}

	// Test SlotFull - first exhaust the pool
	const maxSlots = 5
	names := []string{name1}
	for i := 1; i < maxSlots; i++ {
		name := "test-stats-full-" + string(rune('a'+i))
		names = append(names, name)
		_, _, err := globalPool.Acquire(name)
		if err != nil {
			t.Fatalf("Acquire slot %d (%q): %v", i, name, err)
		}
	}

	// Now try to acquire one more
	overflowName := "test-stats-overflow"
	_, _, err = globalPool.Acquire(overflowName)
	if err == nil {
		t.Fatalf("Acquire(%q): expected error when pool is full", overflowName)
	}

	snap = stats.Snapshot()
	if snap.SlotFull != 1 {
		t.Fatalf("After overflow Acquire: SlotFull=%d, want 1", snap.SlotFull)
	}

	// Cleanup
	for _, name := range names {
		if err := globalPool.Release(name); err != nil {
			t.Errorf("Release(%q): %v", name, err)
		}
	}
}

func TestPoolObserver(t *testing.T) {
	// Test that observer receives correct PoolEvents

	obs := &RecordingObserver{}

	// Create a test pool with the recording observer
	// We'll reuse the same handles from globalPool
	testPool := NewPool(globalPool.handles, obs)

	name1 := "test-observer-1"

	// Acquire - should trigger PoolEventAlloc
	h1, slot1, err := testPool.Acquire(name1)
	if err != nil {
		t.Fatalf("Acquire(%q): %v", name1, err)
	}

	// Write data to trigger metadata save
	if _, err := h1.Write([]byte("test"), 0); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Check events: should have 1 Alloc and at least 1 Meta (from saveMetadata)
	if len(obs.PoolEvents) < 2 {
		t.Fatalf("After first Acquire: got %d events, want at least 2 (Alloc + Meta)", len(obs.PoolEvents))
	}

	allocEvent := obs.PoolEvents[0]
	if allocEvent.Type != PoolEventAlloc {
		t.Fatalf("First event: got type %d, want PoolEventAlloc (%d)", allocEvent.Type, PoolEventAlloc)
	}
	if allocEvent.Name != name1 {
		t.Fatalf("Alloc event: got name %q, want %q", allocEvent.Name, name1)
	}
	if allocEvent.Slot != slot1 {
		t.Fatalf("Alloc event: got slot %d, want %d", allocEvent.Slot, slot1)
	}

	metaEvent := obs.PoolEvents[1]
	if metaEvent.Type != PoolEventMeta {
		t.Fatalf("Second event: got type %d, want PoolEventMeta (%d)", metaEvent.Type, PoolEventMeta)
	}
	if metaEvent.Slot != 0 {
		t.Fatalf("Meta event: got slot %d, want 0", metaEvent.Slot)
	}

	// Acquire same name - should not trigger Alloc (hit, no new event)
	prevEventCount := len(obs.PoolEvents)
	_, _, err = testPool.Acquire(name1)
	if err != nil {
		t.Fatalf("Acquire(%q) second: %v", name1, err)
	}
	if len(obs.PoolEvents) != prevEventCount {
		t.Fatalf("After hit Acquire: got %d events, want %d (no new events on hit)",
			len(obs.PoolEvents), prevEventCount)
	}

	// Release - should trigger PoolEventRelease and PoolEventMeta
	prevEventCount = len(obs.PoolEvents)
	if err := testPool.Release(name1); err != nil {
		t.Fatalf("Release(%q): %v", name1, err)
	}

	if len(obs.PoolEvents) < prevEventCount+2 {
		t.Fatalf("After Release: got %d events, want at least %d (Release + Meta)",
			len(obs.PoolEvents), prevEventCount+2)
	}

	releaseEvent := obs.PoolEvents[prevEventCount]
	if releaseEvent.Type != PoolEventRelease {
		t.Fatalf("Release event: got type %d, want PoolEventRelease (%d)",
			releaseEvent.Type, PoolEventRelease)
	}
	if releaseEvent.Name != name1 {
		t.Fatalf("Release event: got name %q, want %q", releaseEvent.Name, name1)
	}
	if releaseEvent.Slot != slot1 {
		t.Fatalf("Release event: got slot %d, want %d", releaseEvent.Slot, slot1)
	}

	// Exhaust pool to trigger PoolEventFull
	const maxSlots = 5
	for i := 0; i < maxSlots; i++ {
		name := "test-observer-full-" + string(rune('a'+i))
		_, _, err := testPool.Acquire(name)
		if err != nil {
			t.Fatalf("Acquire slot %d (%q): %v", i, name, err)
		}
	}

	prevEventCount = len(obs.PoolEvents)
	overflowName := "test-observer-overflow"
	_, _, err = testPool.Acquire(overflowName)
	if err == nil {
		t.Fatalf("Acquire(%q): expected error when pool is full", overflowName)
	}

	if len(obs.PoolEvents) <= prevEventCount {
		t.Fatalf("After overflow Acquire: got %d events, want > %d (should have Full event)",
			len(obs.PoolEvents), prevEventCount)
	}

	fullEvent := obs.PoolEvents[len(obs.PoolEvents)-1]
	if fullEvent.Type != PoolEventFull {
		t.Fatalf("Last event: got type %d, want PoolEventFull (%d)",
			fullEvent.Type, PoolEventFull)
	}
	if fullEvent.Name != overflowName {
		t.Fatalf("Full event: got name %q, want %q", fullEvent.Name, overflowName)
	}
	if fullEvent.Slot != -1 {
		t.Fatalf("Full event: got slot %d, want -1", fullEvent.Slot)
	}

	// Cleanup
	for i := 0; i < maxSlots; i++ {
		name := "test-observer-full-" + string(rune('a'+i))
		if err := testPool.Release(name); err != nil {
			t.Errorf("Release(%q): %v", name, err)
		}
	}
}
