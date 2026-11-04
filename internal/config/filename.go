package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// DirectFileName is the file name form of "no context" (direct mode over
// PAPIERKRAM_TOKEN): it has a dot, so it is no valid context name
// (ValidateName) and the escaping of FileName never produces it for one.
const DirectFileName = ".direct"

// maxFileName is the longest escaped name FileName returns as is; longer
// ones are replaced by a hash.
const maxFileName = 128

// FileName is the form of the context name ctx in the names of per-context
// files in the cache directory (timer state, completion cache): the one
// implementation, so every such file of a context carries the same name.
// A context name may hold any character but spaces and dots, so every
// character other than a–z, 0–9, - and _ is escaped as %XX per byte: the
// name is never a path, and contexts that differ only in case get names
// that differ in more than case (case-insensitive file systems). An empty
// ctx is DirectFileName; a name escaped to more than 128 bytes is replaced
// by a hash (~<hex>).
func FileName(ctx string) string {
	if ctx == "" {
		return DirectFileName
	}
	name := escapeFileName(ctx)
	if len(name) > maxFileName {
		h := sha256.Sum256([]byte(ctx))
		name = "~" + hex.EncodeToString(h[:16])
	}
	return name
}

// escapeFileName keeps a–z, 0–9, - and _ and writes every other byte as
// %XX.
func escapeFileName(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
