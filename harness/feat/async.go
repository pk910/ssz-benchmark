//go:build !noasync

// Package feat gives the harness the options of the library that not
// every version of it has. Each comes in two files: one that uses the
// option, and one, selected by a build tag, for a version without it. The
// benchmark daemon sets the tag when the checkout it builds against lacks
// the option, and the engines that need it are then left out.
package feat

import ssz "github.com/pk910/dynamic-ssz"

// Async returns the options of an engine that hashes with background
// workers, and whether the library has them.
func Async(workers int) ([]ssz.DynSszOption, bool) {
	return []ssz.DynSszOption{ssz.WithAsyncHashing(workers)}, true
}
