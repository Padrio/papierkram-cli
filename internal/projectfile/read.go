package projectfile

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/output"
)

// errSymlink reports a symbolic link where follow is false (Save).
var errSymlink = errors.New("symbolic link")

// readRegular reads the project file at path, which may come from a
// cloned repository (review I1): only a regular file of at most MaxSize
// bytes. A device (a checked-in link to /dev/zero), a FIFO (would block)
// or a directory is a usage error before it is opened; the opened handle
// is checked again, so a file swapped in between is never read beyond
// MaxSize+1 bytes. With follow a symbolic link to a regular file is read
// (a shared file in a monorepo); without (Save) a link is errSymlink and,
// where the platform allows, the open does not follow one either.
func readRegular(path string, follow bool) ([]byte, error) {
	stat := os.Stat
	if !follow {
		stat = os.Lstat
	}
	st, err := stat(path)
	if err != nil {
		return nil, fileErr(path, "lesen", err)
	}
	if st.Mode()&fs.ModeSymlink != 0 {
		return nil, errSymlink
	}
	if !st.Mode().IsRegular() {
		return nil, notRegular(path)
	}
	f, err := openRead(path, follow)
	if err != nil {
		return nil, fileErr(path, "lesen", err)
	}
	defer func() { _ = f.Close() }()
	fst, err := f.Stat()
	if err != nil {
		return nil, fileErr(path, "lesen", err)
	}
	if !fst.Mode().IsRegular() || !os.SameFile(st, fst) {
		return nil, notRegular(path)
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxSize+1))
	if err != nil {
		return nil, fileErr(path, "lesen", err)
	}
	if len(data) > MaxSize {
		return nil, clierr.New(clierr.CodeUsage,
			fmt.Sprintf("Projektdatei %s ist zu groß (mehr als %d KiB)", output.Printable(path), MaxSize>>10),
			"Die Projektdatei enthält nur wenige IDs und Aliase; Datei prüfen")
	}
	return data, nil
}

// notRegular is the usage error for a project file that is no regular
// file.
func notRegular(path string) error {
	return clierr.New(clierr.CodeUsage,
		fmt.Sprintf("Projektdatei %s ist keine reguläre Datei – nicht gelesen", output.Printable(path)),
		"Die Datei kann aus einem fremden Repository stammen (z. B. ein Link auf ein Gerät); prüfen und durch eine echte Datei ersetzen")
}
