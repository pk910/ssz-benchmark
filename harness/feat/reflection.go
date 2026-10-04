//go:build !nodelegation

package feat

import ssz "github.com/pk910/dynamic-ssz"

// Reflection returns the options of an engine that runs on reflection
// alone, bypassing the generated methods of the types, and whether the
// library can do that.
func Reflection() ([]ssz.DynSszOption, bool) {
	return []ssz.DynSszOption{ssz.WithNoFastSsz(), ssz.WithNoDelegation()}, true
}
