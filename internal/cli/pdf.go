package cli

import (
	"context"
	"fmt"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"strings"

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

-O datei.pdf speichert das PDF in der Datei; eine vorhandene Datei wird erst ersetzt, wenn das PDF vollständig geladen ist. -O - schreibt das PDF nach stdout. Ohne -O geht es nur dann nach stdout, wenn stdout kein Terminal ist (Umleitung oder Pipe); im Terminal bricht der Befehl ohne Anfrage ab.

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
// whole PDF arrived: a failed download or write leaves no partial file and
// an existing file untouched.
func (a *App) runPDF(c *cobra.Command, e registry.Entry, args []string) error {
	params, err := pathValues(c, e, args)
	if err != nil {
		return err
	}
	target, err := a.pdfTarget(c)
	if err != nil {
		return err
	}
	client, r, err := a.Client(c)
	if err != nil {
		return err
	}
	var file *pdfFile
	if target != pdfStdout {
		if file, err = createPDFFile(target); err != nil {
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
	if file == nil {
		if _, err := a.Out.Write(resp.Body); err != nil {
			return clierr.New(clierr.CodeError, "Befehl ausgeführt, Ausgabe fehlgeschlagen: PDF nicht nach stdout geschrieben: "+err.Error(), "")
		}
	} else if err := file.commit(resp.Body, e); err != nil {
		return err
	}
	return a.pdfMessage(c.Context(), r, pdfSaved{Path: target, Bytes: len(resp.Body)})
}

// pdfTarget returns the target of the PDF: the file of -O or pdfStdout.
// Without -O the PDF goes to stdout only when it is no terminal (spec 9.2,
// R29). --fields and --jq apply to the message only, which is no output of
// its own when the PDF fills stdout. A file needs an existing directory
// and must not be a directory itself.
func (a *App) pdfTarget(c *cobra.Command) (string, error) {
	path := pdfStdout
	switch {
	case c.Flags().Changed("output-file"):
		if path = flagString(c, "output-file"); path == "" {
			return "", clierr.New(clierr.CodeUsage, "-O: Dateiname fehlt", "Datei angeben oder - für stdout")
		}
	case a.StdoutTTY:
		return "", clierr.New(clierr.CodeUsage, "kein Ziel für das PDF: stdout ist ein Terminal",
			"-O datei.pdf speichert das PDF in einer Datei, -O - schreibt es trotzdem nach stdout (z. B. -O - > datei.pdf). Verwendung: "+useLine(c)+" -O datei.pdf")
	}
	if path == pdfStdout {
		if len(output.ParseFields(a.Flags.Fields)) > 0 || a.Flags.JQ != "" {
			return "", clierr.New(clierr.CodeUsage, "--fields und --jq gelten nicht für ein PDF auf stdout",
				`mit -O datei.pdf gelten sie für die Meldung {"path":…,"bytes":…}`)
		}
		return path, nil
	}
	dir := filepath.Dir(path)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return "", clierr.New(clierr.CodeUsage, fmt.Sprintf("-O %s: Verzeichnis %s existiert nicht", path, dir),
			"Verzeichnis anlegen oder einen anderen Pfad angeben")
	}
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		return "", clierr.New(clierr.CodeUsage, fmt.Sprintf("-O %s ist ein Verzeichnis", path),
			"Dateinamen angeben, z. B. -O "+filepath.Join(path, "datei.pdf"))
	}
	return path, nil
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

// createPDFFile creates the temporary file next to path.
func createPDFFile(path string) (*pdfFile, error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return nil, clierr.New(clierr.CodeError, "PDF nicht gespeichert: "+err.Error(),
			"Schreibrechte im Verzeichnis prüfen; es wurde nichts angefragt")
	}
	return &pdfFile{path: path, tmp: tmp}, nil
}

// commit writes data to the temporary file and renames it to the target.
// A new file gets mode 0644, an existing one keeps its mode.
func (f *pdfFile) commit(data []byte, e registry.Entry) error {
	mode := fs.FileMode(0o644)
	if fi, err := os.Stat(f.path); err == nil {
		mode = fi.Mode().Perm()
	}
	err := writeAll(f.tmp, data, mode)
	if err == nil {
		err = os.Rename(f.tmp.Name(), f.path)
	}
	if err != nil {
		return clierr.New(clierr.CodeError, "PDF nicht gespeichert: "+err.Error(),
			fmt.Sprintf("Download erfolgreich, Speichern fehlgeschlagen: Schreibrechte und Speicherplatz prüfen; ein neuer Aufruf kostet %s", creditsText(e.Credits)))
	}
	f.done = true
	return nil
}

// writeAll writes data to f, sets mode, syncs and closes f.
func writeAll(f *os.File, data []byte, mode fs.FileMode) error {
	_, err := f.Write(data)
	if err == nil {
		err = f.Chmod(mode)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// discard removes the temporary file unless commit renamed it.
func (f *pdfFile) discard() {
	if f.done {
		return
	}
	_ = f.tmp.Close()
	_ = os.Remove(f.tmp.Name())
}

// creditsText is "1 Credit" or "N Credits".
func creditsText(n int) string {
	if n == 1 {
		return "1 Credit"
	}
	return fmt.Sprintf("%d Credits", n)
}
