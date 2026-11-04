package projectfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/safefile"
)

// Changes are the keys local init sets; a nil field leaves its key as it
// is. IDs must be positive (Load rejects others), the caller checks them.
type Changes struct {
	Context                      *string
	Company, Project, Task, User *int64
}

// SaveResult reports what Save did: Created a new file, Changed (wrote)
// the file, and whether the old file HadComments, which are lost.
type SaveResult struct {
	Created, Changed, HadComments bool
}

// Save writes ch into the project file at path (spec 9.8, local init). An
// existing file is read and only the given keys change; all other keys,
// unknown ones and [tasks] included, stay. Comments and formatting are
// not kept (the file is encoded anew). When no key changes, nothing is
// written, so an unchanged file keeps its comments. A file that is no
// valid TOML is never overwritten (usage error). The write is atomic: a
// temporary file in the same directory, with the mode of the existing
// file (a new one gets 0666 minus the umask), renamed over the target.
//
// Save never writes through a symbolic link: the file may come from a
// cloned repository that points it at ~/.bashrc. A link at path (also a
// dangling one) or anything else that is no regular file is a usage
// error, and nothing is read or written. The rename replaces the entry
// at path itself and the temporary file is created exclusively, so a link
// placed after the check is not followed either.
func Save(path string, ch Changes) (SaveResult, error) {
	var res SaveResult
	shown := output.Printable(path)
	st, err := os.Lstat(path)
	exists := err == nil
	switch {
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return res, fileErr(path, "prüfen", err)
	case exists && st.Mode()&fs.ModeSymlink != 0:
		return res, symlinkErr(shown)
	case exists && !st.Mode().IsRegular():
		return res, clierr.New(clierr.CodeUsage,
			fmt.Sprintf("Projektdatei %s ist keine reguläre Datei – nicht überschrieben", shown), "")
	}
	var data []byte
	if exists {
		// read through the checked handle, without following a link swapped
		// in after the Lstat (O_NOFOLLOW on Unix), and bounded (review M4, I1)
		if data, err = readRegular(path, false); err != nil {
			if errors.Is(err, errSymlink) {
				return res, symlinkErr(shown)
			}
			return res, err
		}
	}
	m := map[string]any{}
	if exists {
		if m, err = decode(shown, data); err != nil {
			return res, err
		}
		if m == nil {
			m = map[string]any{}
		}
	}
	set := func(key string, v any) {
		if old, ok := m[key]; !ok || old != v {
			m[key] = v
			res.Changed = true
		}
	}
	if ch.Context != nil {
		set("context", *ch.Context)
	}
	for _, kv := range []struct {
		key string
		v   *int64
	}{{"company", ch.Company}, {"project", ch.Project}, {"task", ch.Task}, {"user", ch.User}} {
		if kv.v != nil {
			set(kv.key, *kv.v)
		}
	}
	if !res.Changed {
		return res, nil
	}
	out, err := toml.Marshal(m)
	if err != nil {
		return SaveResult{}, fmt.Errorf("%s kodieren: %w", shown, err)
	}
	mode := fs.FileMode(0o666)
	if exists {
		mode = st.Mode().Perm()
	}
	if err := safefile.WriteAtomic(path, out, mode, safefile.WriteOptions{ExactMode: exists}); err != nil {
		return SaveResult{}, fileErr(path, "schreiben", err)
	}
	res.Created = !exists
	res.HadComments = exists && hasComments(data)
	return res, nil
}

// hasComments reports whether the TOML document data has a comment (a #
// outside of strings), found by the TOML parser.
func hasComments(data []byte) bool {
	p := unstable.Parser{KeepComments: true}
	p.Reset(data)
	for p.NextExpression() {
		// a comment after an expression is its next sibling
		for n := p.Expression(); n.Valid(); n = n.Next() {
			if commentIn(n) {
				return true
			}
		}
	}
	return false
}

// commentIn reports whether n or a node below it is a comment.
func commentIn(n *unstable.Node) bool {
	if n.Kind == unstable.Comment {
		return true
	}
	for it := n.Children(); it.Next(); {
		if commentIn(it.Node()) {
			return true
		}
	}
	return false
}

// symlinkErr is the usage error for a project file that is a symbolic
// link; shown is the printable path.
func symlinkErr(shown string) error {
	return clierr.New(clierr.CodeUsage,
		fmt.Sprintf("Projektdatei %s ist ein symbolischer Link – nicht überschrieben", shown),
		"Der Link kann aus einem fremden Repository stammen und auf eine andere Datei zeigen. "+
			"Link prüfen und durch eine echte Datei ersetzen oder die Werte von Hand eintragen")
}
