package projectfile

import (
	"errors"
	"fmt"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/safefile"
)

// errSymlink reports a symbolic link where follow is false (Save).
var errSymlink = safefile.ErrSymlink

// readRegular reads the project file at path, which may come from a
// cloned repository (review I1), through safefile.ReadRegular: only a
// regular file of at most MaxSize bytes. A device (a checked-in link to
// /dev/zero), a FIFO (would block) or a directory is a usage error before
// it is opened, and a file swapped in between is never read beyond
// MaxSize+1 bytes. With follow a symbolic link to a regular file is read
// (a shared file in a monorepo); without (Save) a link is errSymlink and,
// where the platform allows, the open does not follow one either.
func readRegular(path string, follow bool) ([]byte, error) {
	data, err := safefile.ReadRegular(path, follow, MaxSize)
	switch {
	case err == nil:
		return data, nil
	case errors.Is(err, safefile.ErrSymlink):
		return nil, errSymlink
	case errors.Is(err, safefile.ErrNotRegular):
		return nil, notRegular(path)
	case errors.Is(err, safefile.ErrTooLarge):
		return nil, clierr.New(clierr.CodeUsage,
			fmt.Sprintf("Projektdatei %s ist zu groß (mehr als %d KiB)", output.Printable(path), MaxSize>>10),
			"Die Projektdatei enthält nur wenige IDs und Aliase; Datei prüfen")
	}
	// found but not readable (permissions, I/O): a plain error, which the
	// caller maps (Load: readErr; Save: local init's "Projektdatei nicht
	// geschrieben: …", review T18-M1)
	return nil, fileErr(path, "lesen", err)
}

// readErr is the error of Load for a project file it found but could not
// read (readRegular): a usage error of readRegular as it is, any other an
// error of the CLI (exit 1) whose hint names the file (L81).
func readErr(path string, err error) error {
	var ce *clierr.Error
	if errors.As(err, &ce) {
		return err
	}
	e := clierr.New(clierr.CodeError, err.Error(), fmt.Sprintf("Projektdatei %s prüfen oder entfernen", output.Printable(path)))
	e.Cause = err
	return e
}

// notRegular is the usage error for a project file that is no regular
// file.
func notRegular(path string) error {
	return clierr.New(clierr.CodeUsage,
		fmt.Sprintf("Projektdatei %s ist keine reguläre Datei – nicht gelesen", output.Printable(path)),
		"Die Datei kann aus einem fremden Repository stammen (z. B. ein Link auf ein Gerät); prüfen und durch eine echte Datei ersetzen")
}
