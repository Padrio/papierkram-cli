package projectfile

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
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
// file (a new one gets 0666 minus the umask), renamed over the target. A
// symbolic link is followed, so its target is updated.
func Save(path string, ch Changes) (SaveResult, error) {
	var res SaveResult
	target := path
	if p, err := filepath.EvalSymlinks(path); err == nil {
		target = p
	}
	data, err := os.ReadFile(target)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return res, fmt.Errorf("%s lesen: %w", path, err)
	}
	m := map[string]any{}
	if exists {
		if m, err = decode(path, data); err != nil {
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
		return SaveResult{}, fmt.Errorf("%s kodieren: %w", path, err)
	}
	mode := fs.FileMode(0o666)
	if exists {
		if st, err := os.Stat(target); err == nil {
			mode = st.Mode().Perm()
		}
	}
	if err := writeAtomic(target, out, mode, exists); err != nil {
		return SaveResult{}, fmt.Errorf("%s schreiben: %w", path, err)
	}
	res.Created = !exists
	res.HadComments = exists && hasComments(data)
	return res, nil
}

// writeAtomic writes data to a new temporary file next to path with mode
// (the umask applies at creation; exact sets it on the open file, for the
// mode of a replaced file) and renames it to path.
func writeAtomic(path string, data []byte, mode fs.FileMode, exact bool) error {
	dir, base := filepath.Dir(path), filepath.Base(path)
	var f *os.File
	var err error
	for range 10 {
		name := filepath.Join(dir, "."+base+"."+rand.Text()[:12]+".tmp")
		f, err = os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, mode)
		if !errors.Is(err, fs.ErrExist) {
			break
		}
	}
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }() // no-op after the rename
	_, err = f.Write(data)
	if err == nil && exact {
		_ = f.Chmod(mode) // best effort (file systems without Unix modes)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
