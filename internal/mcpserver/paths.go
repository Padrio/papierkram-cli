package mcpserver

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
	if why := unsafePath(root, dir, v); why != "" {
		return "", &ArgError{
			Msg:  fmt.Sprintf("Parameter %s: Pfad %s %s", quoted(name), quoted(v), why),
			Hint: "Datei direkt im Dateiordner des Servers angeben (vorhandenes Verzeichnis, keine Verweise hinaus)",
		}
	}
	return full, nil
}

// lstat is os.Lstat; tests replace it (Windows junctions, which Lstat
// reports as irregular files, cannot be made elsewhere).
var lstat = os.Lstat

// unsafePath checks the local path v below dir (root: dir with its links
// resolved) fail closed and returns why it is refused, "" when it is
// safe. Every component is read with Lstat: an irregular one (a Windows
// junction) or one that cannot be read is refused, and so is a missing
// directory component. Only the last component may be missing, and only
// when it is no symbolic link (a link that leads nowhere is refused). The
// path, or for a missing last component its directory, must lie in root
// with every link resolved; any error resolving it (a loop, no
// permission, a NUL byte) is a refusal.
func unsafePath(root, dir, v string) string {
	cur, leafMissing := dir, false
	parts := strings.Split(filepath.Clean(v), string(filepath.Separator))
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		fi, err := lstat(cur)
		switch {
		case errors.Is(err, fs.ErrNotExist) && i == len(parts)-1:
			leafMissing = true
		case err != nil:
			return "ist nicht prüfbar (Verzeichnis fehlt oder ist nicht lesbar)"
		case fi.Mode()&fs.ModeIrregular != 0:
			return "führt über einen Verbindungspunkt (Junction) oder eine unbekannte Dateiart"
		}
	}
	check := filepath.Join(dir, v)
	if leafMissing {
		check = filepath.Dir(check)
	}
	real, err := filepath.EvalSymlinks(check)
	if err != nil {
		return "ist nicht prüfbar (symbolischer Verweis ins Leere, Schleife oder keine Rechte)"
	}
	if rel, err := filepath.Rel(root, real); err != nil || (rel != "." && !filepath.IsLocal(rel)) {
		return "führt über einen symbolischen Verweis aus dem Dateiordner hinaus"
	}
	return ""
}
