package nle

import "aago/snapshot"

// Pointers into the immutable tables are compared by identity (Python namedtuples / module objects), so
// snapshots must keep them as references to these tables, never as copies.
func init() {
	for _, o := range Objects {
		if o != nil {
			snapshot.RegisterStatic(o)
		}
	}
	for i := range Permonsts {
		snapshot.RegisterStatic(&Permonsts[i])
	}
	snapshot.RegisterStatic(DummyPermonst)
	for i := range ObjClasses {
		snapshot.RegisterStatic(&ObjClasses[i])
	}
	for _, s := range namedGSets {
		snapshot.RegisterStatic(s)
	}
	snapshot.RequireStatic((*Obj)(nil), (*Permonst)(nil), (*ObjClass)(nil), (*GSet)(nil))
}
