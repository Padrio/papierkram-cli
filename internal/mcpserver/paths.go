package mcpserver

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
)

// File paths of tool calls (ruling T5-A): every KindPath value names a
// file inside the files directory of the server (Options.FilesDir, mcp
// serve --files-dir). It must be local in the sense of filepath.IsLocal:
// relative, without "..", not empty; "-" (stdin/stdout, the transport)
// is not allowed either. The CLI gets the absolute path below FilesDir,
// so a call can neither write nor read outside of it by path; a symbolic
// link inside that leads out is refused too. The policy applies to every
// KindPath parameter, the files a command writes (pdf -O) and those it
// reads (voucher upload, voucher create --file: task 6).

// localPath is the absolute path of the tool path value v of param name
// below dir, or the ArgError why it is not allowed.
func localPath(name, dir, v string) (string, error) {
	switch {
	case v == "-":
		return "", &ArgError{
			Msg:  fmt.Sprintf("Parameter %s: „-“ (stdin/stdout) ist im MCP-Server nicht erlaubt", quoted(name)),
			Hint: "Einen Dateinamen im Dateiordner des Servers angeben; stdin und stdout sind der Transport des MCP-Servers",
		}
	case !filepath.IsLocal(v):
		return "", &ArgError{
			Msg:  fmt.Sprintf("Parameter %s: Pfad %s liegt nicht im Dateiordner des Servers", quoted(name), quoted(v)),
			Hint: "Relativen Pfad im Dateiordner angeben (mcp serve --files-dir): nicht leer, nicht absolut, ohne „..“",
		}
	case dir == "":
		return "", errors.New("mcp: kein Dateiordner gesetzt")
	}
	full := filepath.Join(dir, v)
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("mcp: Dateiordner %s: %w", dir, err)
	}
	if escapes(root, full) {
		return "", &ArgError{
			Msg:  fmt.Sprintf("Parameter %s: Pfad %s führt über einen symbolischen Verweis aus dem Dateiordner hinaus", quoted(name), quoted(v)),
			Hint: "Datei direkt im Dateiordner des Servers angeben",
		}
	}
	return full, nil
}

// escapes reports whether full, with its symbolic links resolved, lies
// outside root (resolved). Of a path that does not exist (yet) its
// directory is checked; when that does not exist either, nothing there can
// lead out (the CLI fails on the missing directory).
func escapes(root, full string) bool {
	real, err := filepath.EvalSymlinks(full)
	if errors.Is(err, fs.ErrNotExist) {
		real, err = filepath.EvalSymlinks(filepath.Dir(full))
	}
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, real)
	return err != nil || (rel != "." && !filepath.IsLocal(rel))
}
