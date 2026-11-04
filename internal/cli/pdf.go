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

-O datei.pdf speichert das PDF in der Datei; eine vorhandene Datei wird erst ersetzt, wenn das PDF vollständig geladen ist, und behält ihre Rechte (eine neue Datei erhält die Rechte gemäß umask). Eine schreibgeschützte Datei wird nicht ersetzt; ein symbolischer Verweis bleibt erhalten, ersetzt wird die Datei, auf die er zeigt; Gerätedateien wie /dev/null werden direkt beschrieben. -O - schreibt das PDF nach stdout. Ohne -O geht es nur dann nach stdout, wenn stdout kein Terminal ist (Umleitung oder Pipe); im Terminal bricht der Befehl ohne Anfrage ab.

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
	params, err := pathValues(c, e, args)
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
		if file, err = createPDFFile(dest.path); err != nil {
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
			return clierr.New(clierr.CodeError, "Befehl ausgeführt, Ausgabe fehlgeschlagen: PDF nicht nach stdout geschrieben: "+err.Error(), "")
		}
	case file == nil:
		if err := writeDevice(dest.path, resp.Body); err != nil {
			return pdfSaveFailed(err, e)
		}
	default:
		warning, err := file.commit(resp.Body, dest.info)
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
	if arg != pdfStdout {
		return pdfFileDest(arg)
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
// read-only file, so it is refused everywhere.
func pdfFileDest(arg string) (pdfDest, error) {
	d := pdfDest{arg: arg, path: arg}
	if fi, err := os.Lstat(arg); err == nil && fi.Mode()&fs.ModeSymlink != 0 {
		real, err := filepath.EvalSymlinks(arg)
		if err != nil {
			return d, clierr.New(clierr.CodeUsage, fmt.Sprintf("-O %s: symbolischer Verweis zeigt auf kein vorhandenes Ziel", arg),
				"Ziel des Verweises anlegen oder einen anderen Pfad angeben")
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
	return d, nil
}

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

// createPDFFile creates the temporary file next to path with mode 0666, so
// the umask applies like for any new file (curl, wget, os.WriteFile):
// os.CreateTemp would create 0600 and need a chmod that bypasses it.
func createPDFFile(path string) (*pdfFile, error) {
	dir, base := filepath.Dir(path), filepath.Base(path)
	var err error
	for range 10 {
		name := filepath.Join(dir, "."+base+"."+rand.Text()[:12]+".tmp")
		var f *os.File
		f, err = os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666)
		if err == nil {
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
// When it replaces existing, it takes over its mode best effort: a failed
// chmod (file systems without Unix modes) never discards the paid
// download, it only returns a warning.
func (f *pdfFile) commit(data []byte, existing fs.FileInfo) (warning string, err error) {
	if err := writeAll(f.tmp, data); err != nil {
		return "", err
	}
	if existing != nil {
		if err := chmodFile(f.tmp.Name(), existing.Mode().Perm()); err != nil {
			warning = "Rechte der ersetzten Datei nicht übernommen: " + err.Error()
		}
	}
	if err := os.Rename(f.tmp.Name(), f.path); err != nil {
		return "", err
	}
	f.done = true
	return warning, nil
}

// chmodFile sets the mode of a file by name; tests replace it.
var chmodFile = os.Chmod

// writeAll writes data to f, syncs and closes it.
func writeAll(f *os.File, data []byte) error {
	_, err := f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// writeDevice writes data to an existing file that is no regular file
// (/dev/null, a FIFO).
func writeDevice(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
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
