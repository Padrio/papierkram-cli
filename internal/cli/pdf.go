package cli

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

// invoice|estimate|voucher pdf <id> [-O datei|-] (spec 9.2, R29).

// pdfStdout is the -O value (and the path of the message) for stdout.
const pdfStdout = "-"

const pdfLong = `

-O datei.pdf speichert das PDF in der Datei; eine vorhandene Datei wird erst ersetzt, wenn das PDF vollständig geladen ist, und behält ihre Rechte, wenn sie dem Nutzer gehört (eine neue oder fremde Datei erhält die Rechte gemäß umask). Eine schreibgeschützte Datei wird nicht ersetzt; ein symbolischer Verweis bleibt erhalten, ersetzt wird die Datei, auf die er zeigt – nur wenn alle Verweise und die Datei dem Nutzer gehören; Gerätedateien wie /dev/null werden direkt beschrieben. -O - schreibt das PDF nach stdout. Ohne -O geht es nur dann nach stdout, wenn stdout kein Terminal ist (Umleitung oder Pipe); im Terminal bricht der Befehl ohne Anfrage ab.

--no-clobber ersetzt nichts: Gibt es unter dem Pfad von -O schon etwas (Datei, symbolischer Verweis, Gerät, Verzeichnis), bricht der Befehl vor der Anfrage mit Exit 2 ab; entsteht die Datei erst während des Downloads, bleibt sie unverändert (Exit 1), denn das PDF wird per hartem Verweis angelegt, der ein vorhandenes Ziel nie ersetzt. Auf Dateisystemen ohne harte Verweise prüft der Befehl stattdessen direkt vor dem Umbenennen; zwischen Prüfung und Umbenennen bleibt dort ein kurzes Rennfenster, in dem eine gerade entstandene Datei ersetzt werden kann. Gilt nur mit -O datei.

Danach meldet der Befehl Pfad und Größe ({"path":…,"bytes":…}, path "-" für stdout) auf stderr, bei -O datei mit -o json|yaml, --fields oder --jq auf stdout. Geht das PDF nach stdout, bleibt die Meldung auf stderr; --fields und --jq sind dann nicht erlaubt.`

// newPDFCmd builds the pdf command of an invoice, estimate or voucher
// entry. It only reads (class read_only): no mutation guard, and the root
// rejects --dry-run (checkDryRun).
func (a *App) newPDFCmd(e registry.Entry) *cobra.Command {
	cmd := &cobra.Command{
		Use:         e.Action + " <id>",
		Short:       e.Short,
		Long:        entryLong(e) + pdfLong,
		Example:     entryExample(e),
		Annotations: map[string]string{annotClass: string(e.Class)},
		Args:        idArgs,
		RunE:        func(c *cobra.Command, args []string) error { return a.runPDF(c, e, args) },
	}
	cmd.Flags().StringP("output-file", "O", "", "Zieldatei des PDFs oder - für stdout")
	markFile(cmd.Flags(), "output-file", fileFlagPath)
	cmd.Flags().Bool("no-clobber", false, "vorhandenes Ziel nicht ersetzen (Exit 2 vor der Anfrage)")
	return cmd
}

// pdfSaved is the message after the download (spec 9.2).
type pdfSaved struct {
	Path  string `json:"path"`
	Bytes int    `json:"bytes"`
}

// runPDF downloads the PDF and writes it unchanged to the file of -O or to
// stdout. Every check of the flags and the target runs before the request
// (exit 2, R29: no request without a target). A target file is written
// through a temporary file in its directory, created before the request
// (an unwritable directory costs no credit) and renamed only after the
// whole PDF arrived: a failed or cancelled download or write leaves no
// partial file and an existing file untouched. A device (/dev/null) is
// written directly.
func (a *App) runPDF(c *cobra.Command, e registry.Entry, args []string) error {
	params, err := a.pathValues(c, e, args)
	if err != nil {
		return err
	}
	dest, err := a.pdfTarget(c)
	if err != nil {
		return err
	}
	client, r, err := a.Client(c)
	if err != nil {
		return err
	}
	var file *pdfFile
	if !dest.stdout() && !dest.device() {
		if file, err = createPDFFile(dest.path, dest.modeFrom); err != nil {
			return err
		}
		defer file.discard()
	}
	resp, err := client.Download(c.Context(), e.OperationID, params, pdfMediaType(e))
	if err != nil {
		return err
	}
	if err := checkPDFAnswer(resp); err != nil {
		return err
	}
	if len(resp.Body) == 0 {
		output.Warn(a.Err, "Papierkram hat eine leere Antwort geliefert: PDF mit 0 Bytes geschrieben")
	}
	switch {
	case dest.stdout():
		if _, err := a.Out.Write(resp.Body); err != nil {
			return clierr.New(clierr.CodeError, outputFailed+"PDF nicht nach stdout geschrieben: "+err.Error(), "")
		}
	case file == nil:
		if err := writeDevice(c.Context(), dest.path, resp.Body); err != nil {
			if ce := (*clierr.Error)(nil); errors.As(err, &ce) {
				return ce // interrupted
			}
			return pdfSaveFailed(err, e)
		}
	default:
		warning, err := file.commit(resp.Body, dest.modeFrom, dest.noClobber)
		if err != nil {
			return pdfSaveFailed(err, e)
		}
		if warning != "" {
			output.Warn(a.Err, warning)
		}
	}
	return a.pdfMessage(c.Context(), r, pdfSaved{Path: dest.arg, Bytes: len(resp.Body)})
}

// pdfDest is the target of the PDF.
type pdfDest struct {
	arg  string      // the value of -O, pdfStdout for stdout; the path of the message
	path string      // the file written: arg with symbolic links resolved
	info fs.FileInfo // the existing file at path, nil when there is none
	// modeFrom is info when the user owns that file: only then is its mode
	// taken over. A file of another user is replaced like a new one (0666
	// minus umask), so its looser mode never applies to the PDF.
	modeFrom fs.FileInfo
	// noClobber: --no-clobber, nothing at arg is replaced (commit links
	// instead of renaming).
	noClobber bool
}

func (d pdfDest) stdout() bool { return d.arg == pdfStdout }

// device reports an existing target that is no regular file (/dev/null, a
// FIFO): it is written directly, a temporary file next to it makes no
// sense.
func (d pdfDest) device() bool { return d.info != nil && !d.info.Mode().IsRegular() }

// pdfTarget returns the target of the PDF: the file of -O or stdout.
// Without -O the PDF goes to stdout only when it is no terminal (spec 9.2,
// R29). --fields and --jq apply to the message only, which is no output of
// its own when the PDF fills stdout. A file is checked by pdfFileDest.
func (a *App) pdfTarget(c *cobra.Command) (pdfDest, error) {
	arg := pdfStdout
	switch {
	case c.Flags().Changed("output-file"):
		if arg = flagString(c, "output-file"); arg == "" {
			return pdfDest{}, clierr.New(clierr.CodeUsage, "-O: Dateiname fehlt", "Datei angeben oder - für stdout")
		}
	case a.StdoutTTY:
		return pdfDest{}, clierr.New(clierr.CodeUsage, "kein Ziel für das PDF: stdout ist ein Terminal",
			"-O datei.pdf speichert das PDF in einer Datei, -O - schreibt es trotzdem nach stdout (z. B. -O - > datei.pdf). Verwendung: "+useLine(c)+" -O datei.pdf")
	}
	noClobber := flagBool(c, "no-clobber")
	if arg != pdfStdout {
		return pdfFileDest(arg, noClobber)
	}
	if noClobber {
		return pdfDest{}, clierr.New(clierr.CodeUsage, "--no-clobber gilt nur für eine Datei",
			"Mit -O datei.pdf verwenden. Verwendung: "+useLine(c)+" -O datei.pdf --no-clobber")
	}
	if len(output.ParseFields(a.Flags.Fields)) > 0 || a.Flags.JQ != "" {
		return pdfDest{}, clierr.New(clierr.CodeUsage, "--fields und --jq gelten nicht für ein PDF auf stdout",
			`mit -O datei.pdf gelten sie für die Meldung {"path":…,"bytes":…}`)
	}
	return pdfDest{arg: arg, path: arg}, nil
}

// pdfFileDest checks the file target arg before the request. A symbolic
// link is followed (the link stays, its target is replaced); one that
// leads nowhere, a missing directory and a directory as target are usage
// errors (exit 2). A directory or file that cannot be checked, and a
// write-protected file, are errors (exit 1): Windows refuses to replace a
// read-only file, so it is refused everywhere. With noClobber anything at
// arg (file, link, also one that leads nowhere, device, directory) is a
// usage error.
func pdfFileDest(arg string, noClobber bool) (pdfDest, error) {
	d := pdfDest{arg: arg, path: arg, noClobber: noClobber}
	if noClobber {
		_, err := os.Lstat(arg)
		switch {
		case err == nil:
			return d, clierr.New(clierr.CodeUsage, fmt.Sprintf("-O %s existiert bereits und wird mit --no-clobber nicht ersetzt", arg),
				"anderen Dateinamen angeben; es wurde nichts angefragt")
		case !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR):
			return d, pdfTargetUnusable(arg, err)
		}
	}
	if fi, err := os.Lstat(arg); err == nil && fi.Mode()&fs.ModeSymlink != 0 {
		real, err := resolvePDFLink(arg)
		if err != nil {
			return d, err
		}
		d.path = real
	}
	dir := filepath.Dir(d.path)
	fi, err := os.Stat(dir)
	switch {
	case err == nil && !fi.IsDir(), errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENOTDIR):
		return d, clierr.New(clierr.CodeUsage, fmt.Sprintf("-O %s: Verzeichnis %s existiert nicht", arg, dir),
			"Verzeichnis anlegen oder einen anderen Pfad angeben")
	case err != nil:
		return d, pdfTargetUnusable(arg, err)
	}
	fi, err = os.Stat(d.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return d, nil
	case err != nil:
		return d, pdfTargetUnusable(arg, err)
	case fi.IsDir():
		return d, clierr.New(clierr.CodeUsage, fmt.Sprintf("-O %s ist ein Verzeichnis", arg),
			"Dateinamen angeben, z. B. -O "+filepath.Join(arg, "datei.pdf"))
	case fi.Mode().IsRegular() && fi.Mode().Perm()&0o200 == 0:
		return d, clierr.New(clierr.CodeError, fmt.Sprintf("-O %s: Datei ist schreibgeschützt und wird nicht ersetzt", arg),
			"Schreibschutz aufheben oder einen anderen Dateinamen wählen; es wurde nichts angefragt")
	}
	d.info = fi
	if ownedByMe(fi) {
		d.modeFrom = fi
	}
	return d, nil
}

// maxLinkHops limits the symbolic links resolvePDFLink follows (like
// the 40 of Linux).
const maxLinkHops = 40

// resolvePDFLink follows the symbolic link arg hop by hop with Lstat and
// returns the final file. Every link and the final file must belong to the
// user (ownedByMe), like the kernel's protected_symlinks; otherwise, and
// for a link that leads nowhere, it is a usage error before the request.
// Links in the directory part of a hop are resolved by the system, as for
// any path. The final file's mode and ownership are taken at check time
// (pdfFileDest), so swapping a file later never loosens the mode of the
// PDF; a directory swapped between check and write only moves the
// temporary file, which is created exclusively with that checked mode.
func resolvePDFLink(arg string) (string, error) {
	p := arg
	for range maxLinkHops {
		fi, err := os.Lstat(p)
		switch {
		case errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR):
			return "", clierr.New(clierr.CodeUsage, fmt.Sprintf("-O %s: symbolischer Verweis zeigt auf kein vorhandenes Ziel", arg),
				"Ziel des Verweises anlegen oder einen anderen Pfad angeben")
		case err != nil:
			return "", pdfTargetUnusable(arg, err)
		}
		link := fi.Mode()&fs.ModeSymlink != 0
		if !ownedByMe(fi) {
			what := "Ziel ist ein Symlink, der einem anderen Nutzer gehört"
			if !link {
				what = "Symlink zeigt auf eine Datei, die einem anderen Nutzer gehört"
			}
			return "", clierr.New(clierr.CodeUsage, fmt.Sprintf("-O %s: %s (%s)", arg, what, p), "anderen Pfad angeben")
		}
		if !link {
			return p, nil
		}
		dest, err := os.Readlink(p)
		if err != nil {
			return "", pdfTargetUnusable(arg, err)
		}
		p = linkTarget(p, dest)
	}
	return "", clierr.New(clierr.CodeUsage, fmt.Sprintf("-O %s: zu viele symbolische Verweise hintereinander", arg),
		"anderen Pfad angeben")
}

// linkTarget is the path dest of the link at p: absolute, or relative to
// the directory of p. It is not cleaned lexically, so ".." is resolved by
// the system like when the link is followed.
func linkTarget(p, dest string) string {
	if filepath.IsAbs(dest) {
		return dest
	}
	i := strings.LastIndexAny(p, `/`+string(filepath.Separator))
	if i < 0 {
		return dest
	}
	return p[:i+1] + dest
}

// ownedByMe reports whether a file belongs to the user; tests replace it.
var ownedByMe = ownedByEUID

// pdfTargetUnusable is the error of a target that cannot be checked
// (permission, I/O): exit 1, nothing was requested.
func pdfTargetUnusable(arg string, err error) error {
	return clierr.New(clierr.CodeError, fmt.Sprintf("-O %s: Ziel nicht nutzbar: %v", arg, err),
		"Rechte prüfen; es wurde nichts angefragt")
}

// pdfMediaType is the file type of the download from the spec
// (application/pdf, overlay); the Accept header names it.
func pdfMediaType(e registry.Entry) string {
	if mt := spec.FileMediaType(e.OperationID); mt != "" {
		return mt
	}
	return "application/pdf"
}

// checkPDFAnswer rejects a 2xx answer that declares JSON or text (an API
// object, a proxy page): written as .pdf it would be a broken file with
// exit 0. Binary and missing types are written as they are.
func checkPDFAnswer(resp *api.Response) error {
	ct := resp.Header.Get("Content-Type")
	mt, _, _ := mime.ParseMediaType(ct)
	if mt == "application/json" || strings.HasSuffix(mt, "+json") || strings.HasPrefix(mt, "text/") {
		return clierr.New(clierr.CodeError, fmt.Sprintf("Antwort ist kein PDF (Content-Type %s), nichts gespeichert", ct),
			"Antwort mit --debug ansehen")
	}
	return nil
}

// pdfMessage prints the message {"path","bytes"}: as a German line on
// stderr, structured (-o json|yaml, --fields, --jq) on stdout. When the
// PDF went to stdout, it always goes to stderr (pdfTarget rejected --fields
// and --jq then).
func (a *App) pdfMessage(ctx context.Context, r *config.Resolved, v pdfSaved) error {
	o := a.outputOptions(ctx, a.outputFormat(r))
	if v.Path == pdfStdout {
		o.Out, o.Color = a.Err, false
	}
	if ownText(o) {
		line := fmt.Sprintf("PDF gespeichert: %s (%d Bytes)\n", v.Path, v.Bytes)
		if v.Path == pdfStdout {
			line = fmt.Sprintf("PDF nach stdout geschrieben (%d Bytes)\n", v.Bytes)
		}
		_, err := fmt.Fprint(a.Err, line)
		return afterRequest(err)
	}
	return afterRequest(output.RenderValue(o, v, output.Columns{}))
}

// pdfFile is the temporary file a PDF is written to before it replaces the
// target (same directory, so the rename is atomic).
type pdfFile struct {
	path string
	tmp  *os.File
	done bool
}

// createPDFFile creates the temporary file next to path. For a new file
// the mode is 0666, so the umask applies like for any new file (curl, wget,
// os.WriteFile; os.CreateTemp would create 0600 and need a chmod that
// bypasses it). When it replaces existing, it is created with that file's
// mode right away (the umask can only make it stricter), so the PDF is
// never more readable than the file it replaces, not even before commit
// sets the exact mode.
func createPDFFile(path string, existing fs.FileInfo) (*pdfFile, error) {
	dir, base := filepath.Dir(path), filepath.Base(path)
	mode := fs.FileMode(0o666)
	if existing != nil {
		mode = existing.Mode().Perm()
	}
	var err error
	for range 10 {
		name := filepath.Join(dir, "."+base+"."+rand.Text()[:12]+".tmp")
		var f *os.File
		f, err = os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, mode)
		if err == nil {
			pdfTempCreated(name)
			return &pdfFile{path: path, tmp: f}, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			break
		}
	}
	return nil, clierr.New(clierr.CodeError, "PDF nicht gespeichert: "+err.Error(),
		"Schreibrechte im Verzeichnis prüfen; es wurde nichts angefragt")
}

// commit writes data to the temporary file and renames it to the target.
// When it replaces existing, writeAll sets its exact mode (the umask may
// have cut bits at creation); a failed chmod (file systems without Unix
// modes) never discards the paid download, it only returns a warning.
// With noClobber the temporary file is linked to the target instead, which
// fails when something appeared there meanwhile; a file system without
// hard links falls back to a check and the rename.
func (f *pdfFile) commit(data []byte, existing fs.FileInfo, noClobber bool) (warning string, err error) {
	warning, err = writeAll(f.tmp, data, existing)
	if err != nil {
		return "", err
	}
	if noClobber {
		return warning, f.commitNew()
	}
	if err := os.Rename(f.tmp.Name(), f.path); err != nil {
		return "", err
	}
	f.done = true
	return warning, nil
}

// errClobber is the commit error of --no-clobber when the target appeared
// during the download.
var errClobber = errors.New("das Ziel ist während des Downloads entstanden und wird mit --no-clobber nicht ersetzt")

// linkFile creates a hard link; tests replace it (file systems without
// hard links).
var linkFile = os.Link

// commitNew moves the temporary file to the target without replacing
// anything there.
func (f *pdfFile) commitNew() error {
	err := linkFile(f.tmp.Name(), f.path)
	switch {
	case errors.Is(err, fs.ErrExist):
		return errClobber
	case err != nil:
		if _, serr := os.Lstat(f.path); !errors.Is(serr, fs.ErrNotExist) {
			return errClobber
		}
		if err := os.Rename(f.tmp.Name(), f.path); err != nil {
			return err
		}
	default:
		if removeFile(f.tmp.Name()) != nil {
			// the PDF is saved under its name; discard tries to remove
			// the temporary name again
			return nil
		}
	}
	f.done = true
	return nil
}

// removeFile removes a file; tests replace it.
var removeFile = os.Remove

// chmodFile sets the mode through the open handle of a file; tests replace
// it.
var chmodFile = (*os.File).Chmod

// pdfTempCreated runs right after the temporary file was created, before
// anything is written to it; tests inspect its mode there.
var pdfTempCreated = func(string) {}

// writeAll writes data to f, sets the mode of existing (when not nil)
// through the open handle, syncs and closes f. The chmod is best effort:
// its failure is the warning, never an error.
func writeAll(f *os.File, data []byte, existing fs.FileInfo) (warning string, err error) {
	_, err = f.Write(data)
	if err == nil && existing != nil {
		if cerr := chmodFile(f, existing.Mode().Perm()); cerr != nil {
			warning = "Rechte der ersetzten Datei nicht übernommen: " + cerr.Error()
		}
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return warning, err
}

// pdfDeviceOpening runs before a device target is opened; tests use it to
// interrupt the wait for a FIFO reader.
var pdfDeviceOpening = func() {}

// writeDevice writes data to an existing file that is no regular file
// (/dev/null, a FIFO). Opening a FIFO waits for a reader and writing may
// wait for it to read, so both run in a goroutine and an interrupt (ctx)
// ends the wait: exit 10 like any interrupted command. The goroutine stays
// blocked until a reader comes or the process ends, which follows right
// after.
func writeDevice(ctx context.Context, path string, data []byte) error {
	pdfDeviceOpening()
	done := make(chan error, 1)
	go func() {
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			done <- err
			return
		}
		_, err = f.Write(data)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		select {
		case err := <-done: // finished at the same moment: the write counts
			return err
		default:
		}
		return &clierr.Error{Code: clierr.CodeNetwork, Message: "Abgebrochen: PDF nicht geschrieben", Cause: ctx.Err()}
	}
}

// pdfSaveFailed is the error of a write after the download (exit 1).
func pdfSaveFailed(err error, e registry.Entry) error {
	return clierr.New(clierr.CodeError, "PDF nicht gespeichert: "+err.Error(),
		fmt.Sprintf("Download erfolgreich, Speichern fehlgeschlagen: Schreibrechte und Speicherplatz prüfen; ein neuer Aufruf kostet %s", creditsText(e.Credits)))
}

// discard removes the temporary file unless commit renamed it.
func (f *pdfFile) discard() {
	if f.done {
		return
	}
	_ = f.tmp.Close()
	_ = os.Remove(f.tmp.Name())
}
