//go:build noasync

package feat

import ssz "github.com/pk910/dynamic-ssz"

// Async: this version of the library has no background hashing.
func Async(workers int) ([]ssz.DynSszOption, bool) { return nil, false }
