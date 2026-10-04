//go:build nodelegation

package feat

import ssz "github.com/pk910/dynamic-ssz"

// Reflection: this version of the library cannot bypass the generated
// methods of a type, so reflection alone cannot be measured on the
// harness types, which carry generated code.
func Reflection() ([]ssz.DynSszOption, bool) { return nil, false }
