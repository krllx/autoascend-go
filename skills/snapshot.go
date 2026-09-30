package skills

import "aago/snapshot"

// untrapContainerF returns the message and whether a trap was found.
func init() { snapshot.Register(UntrapResult{}) }
