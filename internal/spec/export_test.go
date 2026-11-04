package spec

import "slices"

// Raw returns the embedded patched spec (what the binary embeds). The
// slice is shared; do not modify it.
func Raw() []byte { return raw }

// ReservedFlags returns the flag names no schema-derived field may use.
// --item belongs to the KindItems field and --attr to the KindAttrs field.
func ReservedFlags() []string { return slices.Clone(reservedFlags) }
