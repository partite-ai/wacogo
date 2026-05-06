package core

// ExternTable is a per-instance read-only lookup from rep (uint32) to a
// host-supplied Go object. Set on a *ComponentInstance via InstanceSpec
// at construction time; queried via (*ComponentInstance).LookupExtern.
type ExternTable interface {
	Lookup(rep uint32) (any, bool)
}
