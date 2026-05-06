package host

// externTable holds an ordered, free-list-reusing collection of any
// values keyed by ExternHandle. One table per host component instance,
// held on (*ComponentInstance).extTable.
//
// Not safe for concurrent use; the component-model single-threaded
// per-instance invariant is enough.
type externTable struct {
	entries      []externEntry
	freeListHead uint32 // 1-indexed; 0 means empty
}

type externEntry struct {
	obj      any
	nextFree uint32 // 1-indexed; 0 = end of list. Valid when occupied == false.
	occupied bool
}

// put registers obj and returns a fresh ExternHandle.
func (t *externTable) put(obj any) ExternHandle {
	if t.freeListHead != 0 {
		idx := t.freeListHead - 1
		t.freeListHead = t.entries[idx].nextFree
		t.entries[idx] = externEntry{obj: obj, occupied: true}
		return ExternHandle(idx + 1)
	}
	idx := uint32(len(t.entries))
	t.entries = append(t.entries, externEntry{obj: obj, occupied: true})
	return ExternHandle(idx + 1)
}

// Lookup satisfies core.ExternTable. It returns the obj stored at rep
// (treated as an ExternHandle) and true on success, or (nil, false) for
// rep == 0, out-of-range, or freed slots.
func (t *externTable) Lookup(rep uint32) (any, bool) {
	return t.lookup(ExternHandle(rep))
}

// lookup returns the obj for an ExternHandle and true on success.
func (t *externTable) lookup(eh ExternHandle) (any, bool) {
	if eh == 0 {
		return nil, false
	}
	idx := uint32(eh) - 1
	if idx >= uint32(len(t.entries)) || !t.entries[idx].occupied {
		return nil, false
	}
	return t.entries[idx].obj, true
}

// release removes the entry and returns its obj plus true on success.
// Returns (nil, false) if eh is 0, out-of-range, or already free.
func (t *externTable) release(eh ExternHandle) (any, bool) {
	if eh == 0 {
		return nil, false
	}
	idx := uint32(eh) - 1
	if idx >= uint32(len(t.entries)) || !t.entries[idx].occupied {
		return nil, false
	}
	obj := t.entries[idx].obj
	t.entries[idx] = externEntry{nextFree: t.freeListHead, occupied: false}
	t.freeListHead = uint32(eh)
	return obj, true
}

// liveCount returns the number of currently-occupied entries.
func (t *externTable) liveCount() int {
	n := 0
	for i := range t.entries {
		if t.entries[i].occupied {
			n++
		}
	}
	return n
}
