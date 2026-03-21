//go:build js

package opfsvfs

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Pool manages a fixed set of pre-allocated OPFS handles.
// Slot 0 is reserved for metadata persistence.
type Pool struct {
	handles  []Handle
	names    map[int]string // slot index -> virtual name
	slots    map[string]int // virtual name -> slot index
	stats    *Stats
	observer Observer
}

// metadata is the JSON structure persisted in slot 0.
type metadata struct {
	Slots map[string]string `json:"slots"` // slot index (as string) -> virtual name
}

// NewPool creates a Pool from pre-allocated handles.
// Slot 0 is reserved for metadata; at least 2 handles are required.
// The observer may be nil (no-op observer will be used).
func NewPool(handles []Handle, observer Observer) *Pool {
	if handles == nil {
		panic("opfsvfs: NewPool handles must not be nil")
	}
	if len(handles) < 2 {
		panic("opfsvfs: NewPool requires at least 2 handles (slot 0 is reserved for metadata)")
	}

	p := &Pool{
		handles:  handles,
		names:    make(map[int]string),
		slots:    make(map[string]int),
		stats:    &Stats{},
		observer: resolveObserver(observer),
	}

	p.loadMetadata()
	return p
}

// Stats returns the pool's statistics counters.
func (p *Pool) Stats() *Stats {
	return p.stats
}

// Acquire returns the handle and slot index for the given virtual name.
// If the name is already mapped, it returns the existing slot (hit).
// Otherwise it allocates the first free slot (alloc).
// Returns an error if the pool is full.
func (p *Pool) Acquire(name string) (Handle, int, error) {
	if name == "" {
		panic("opfsvfs: Acquire name must not be empty")
	}

	// Hit: name already has a slot.
	if slot, ok := p.slots[name]; ok {
		p.assertSlotInBounds(slot)
		p.stats.SlotHits.Add(1)
		return p.handles[slot], slot, nil
	}

	// Alloc: find first free slot (skip slot 0, reserved for metadata).
	for i := 1; i < len(p.handles); i++ {
		if _, used := p.names[i]; !used {
			p.assertSlotInBounds(i)
			p.names[i] = name
			p.slots[name] = i
			p.stats.SlotAllocs.Add(1)
			p.saveMetadata()
			p.observer.OnPoolEvent(PoolEvent{Type: PoolEventAlloc, Name: name, Slot: i})
			return p.handles[i], i, nil
		}
	}

	// Full: no free slots.
	p.stats.SlotFull.Add(1)
	p.observer.OnPoolEvent(PoolEvent{Type: PoolEventFull, Name: name, Slot: -1})
	return nil, -1, fmt.Errorf("opfsvfs: pool full, cannot acquire slot for %q", name)
}

// Release removes the name-to-slot mapping, truncates the slot file to zero,
// and updates persisted metadata.
func (p *Pool) Release(name string) error {
	if name == "" {
		panic("opfsvfs: Release name must not be empty")
	}

	slot, ok := p.slots[name]
	if !ok {
		return fmt.Errorf("opfsvfs: Release: name %q not found in pool", name)
	}
	p.assertSlotInBounds(slot)

	delete(p.slots, name)
	delete(p.names, slot)

	if err := p.handles[slot].Truncate(0); err != nil {
		return fmt.Errorf("opfsvfs: Release: truncate slot %d: %w", slot, err)
	}

	p.saveMetadata()
	p.observer.OnPoolEvent(PoolEvent{Type: PoolEventRelease, Name: name, Slot: slot})
	return nil
}

// Has reports whether the given virtual name is currently mapped to a slot.
func (p *Pool) Has(name string) bool {
	if name == "" {
		panic("opfsvfs: Has name must not be empty")
	}
	_, ok := p.slots[name]
	return ok
}

// loadMetadata reads the JSON metadata from slot 0 and rebuilds the name maps.
func (p *Pool) loadMetadata() {
	p.assertSlotInBounds(0)

	size, err := p.handles[0].GetSize()
	if err != nil || size == 0 {
		return
	}

	buf := make([]byte, size)
	n, err := p.handles[0].Read(buf, 0)
	if err != nil {
		return
	}
	buf = buf[:n]

	var meta metadata
	if err := json.Unmarshal(buf, &meta); err != nil {
		return
	}

	for key, name := range meta.Slots {
		slot, err := strconv.Atoi(key)
		if err != nil {
			continue
		}
		if slot < 1 || slot >= len(p.handles) {
			continue
		}
		p.names[slot] = name
		p.slots[name] = slot
	}
}

// saveMetadata writes the current name mappings as JSON to slot 0.
func (p *Pool) saveMetadata() {
	p.assertSlotInBounds(0)

	meta := metadata{
		Slots: make(map[string]string, len(p.names)),
	}
	for slot, name := range p.names {
		meta.Slots[strconv.Itoa(slot)] = name
	}

	data, err := json.Marshal(&meta)
	if err != nil {
		panic(fmt.Sprintf("opfsvfs: saveMetadata: marshal failed: %v", err))
	}

	// Assert round-trip.
	var check metadata
	if err := json.Unmarshal(data, &check); err != nil {
		panic(fmt.Sprintf("opfsvfs: saveMetadata: round-trip unmarshal failed: %v", err))
	}

	if err := p.handles[0].Truncate(0); err != nil {
		return
	}
	if _, err := p.handles[0].Write(data, 0); err != nil {
		return
	}
	if err := p.handles[0].Flush(); err != nil {
		return
	}

	p.observer.OnPoolEvent(PoolEvent{Type: PoolEventMeta, Name: "", Slot: 0})
}

// assertSlotInBounds panics if the slot index is out of range.
func (p *Pool) assertSlotInBounds(slot int) {
	if slot < 0 || slot >= len(p.handles) {
		panic(fmt.Sprintf("opfsvfs: slot index %d out of bounds [0, %d)", slot, len(p.handles)))
	}
}
