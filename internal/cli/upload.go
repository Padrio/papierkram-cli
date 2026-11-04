package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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

// voucher upload <beleg-id> <datei>, voucher document delete <beleg-id>
// <datei-id> and voucher create --file (spec 9.3, 8.1, 8.4, 10.4).

const uploadLong = `

Die Datei wird als multipart/form-data hochgeladen; der MIME-Typ folgt aus der Dateiendung (unbekannte Endung: application/octet-stream). Gesendet wird nur der Dateiname ohne Verzeichnis und ohne Steuer- oder unsichtbare Zeichen. Die Datei muss vorhanden, lesbar und nicht leer sein, sonst bricht der Befehl ohne Anfrage ab. --dry-run zeigt Dateiname, Größe und MIME-Typ. Die ganze Datei muss innerhalb von --timeout (Standard 30 s) übertragen sein; größere Dateien brauchen ggf. --timeout 120s.`

const documentDeleteLong = `

Löscht die Datei <datei-id> des Belegs <beleg-id>; die IDs stehen in voucher describe <beleg-id> unter documents. Im Terminal wird zur Bestätigung die Datei-ID abgefragt, ohne Terminal ist --yes nötig.`

const createFileLong = `

--file datei lädt die Datei nach dem Anlegen an den neuen Beleg hoch (%s; --dry-run zeigt beide Schritte und die Summe). Die Datei wird vor jeder Anfrage geprüft: fehlt sie, ist sie leer oder nicht lesbar, wird nichts angelegt. Scheitert nur das Hochladen, bleibt der Beleg bestehen: Exit 11 mit Beleg-ID und dem Befehl zum Nachholen (papierkram voucher upload mit Beleg-ID und absolutem Dateipfad) – dann NICHT den ganzen Befehl wiederholen, sonst entsteht ein zweiter Beleg.`

// newUploadCmd builds the upload command of a voucher (class create: no
// confirmation).
func (a *App) newUploadCmd(e registry.Entry) *cobra.Command {
	return &cobra.Command{
		Use:         e.Action + " <beleg-id> <datei>",
		Short:       e.Short,
		Long:        entryLong(e) + uploadLong,
		Example:     entryExample(e),
		Annotations: map[string]string{annotClass: string(e.Class)},
		Args:        uploadArgs,
		RunE:        func(c *cobra.Command, args []string) error { return a.runUpload(c, e, args) },
	}
}

// uploadArgs requires the voucher ID and the file.
func uploadArgs(c *cobra.Command, args []string) error {
	if err := exactArgs(2)(c, args); err != nil {
		return err
	}
	return idsArgs(1)(c, args[:1])
}

// idsArgs requires exactly n arguments that are IDs (integer ≥ 0).
func idsArgs(n int) cobra.PositionalArgs {
	return func(c *cobra.Command, args []string) error {
		if err := exactArgs(n)(c, args); err != nil {
			return err
		}
		for _, arg := range args {
			if _, ok := parseID(arg); !ok {
				return clierr.New(clierr.CodeUsage, fmt.Sprintf("ungültige ID %q: ganze Zahl ≥ 0 erwartet", arg), "Verwendung: "+useLine(c))
			}
		}
		return nil
	}
}

// runUpload uploads the file to the voucher. Order (plan): arguments and
// the file (exit 2, before anything is resolved or sent), configuration and
// read-only gate (mutationClient, exit 9), --dry-run with the file
// (beforeSend), then the request. Output is the API object (the document);
// an empty answer prints {"id":N,"ok":true} with the voucher ID.
func (a *App) runUpload(c *cobra.Command, e registry.Entry, args []string) error {
	params, err := pathValues(c, e, args)
	if err != nil {
		return err
	}
	file, err := openUpload(e, "", args[1])
	if err != nil {
		return err
	}
	defer file.close()
	client, r, err := a.mutationClient(e)
	if err != nil {
		return err
	}
	if proceed, err := a.beforeSend(c, e, client, params, nil, sendExtras{upload: file.info()}); !proceed || err != nil {
		return err
	}
	resp, err := file.send(c, client, params)
	if err != nil {
		return withTimeoutHint(err)
	}
	if len(bytes.TrimSpace(resp.Body)) > 0 {
		return a.render(c, resp.Body, e.Columns)
	}
	id := lastID(params)
	o := a.outputOptions(c.Context(), a.outputFormat(r))
	if ownText(o) {
		_, err := fmt.Fprintf(a.Out, "%s %d: Datei hochgeladen.\n", e.Resource, id)
		return afterRequest(err)
	}
	return afterRequest(output.RenderValue(o, okJSON{ID: id, OK: true}, output.Columns{}))
}

// newDocumentCmd builds the group of the sub-object and its action from
// the entry "document delete": voucher document delete <beleg-id>
// <datei-id>. Both IDs are positional, so the generic path parameters
// (--voucher) do not apply.
func (a *App) newDocumentCmd(e registry.Entry) *cobra.Command {
	name, action, _ := strings.Cut(e.Action, " ")
	group := &cobra.Command{
		Use:   name,
		Short: "Dateien (Belegbilder) eines Belegs verwalten",
		Args: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				return unknownCommand(c, args[0])
			}
			return nil
		},
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	group.AddCommand(&cobra.Command{
		Use:         action + " <beleg-id> <datei-id>",
		Short:       e.Short,
		Long:        entryLong(e) + documentDeleteLong,
		Example:     entryExample(e),
		Annotations: map[string]string{annotClass: string(e.Class)},
		Args:        idsArgs(2),
		RunE:        func(c *cobra.Command, args []string) error { return a.runDocumentDelete(c, e, args) },
	})
	return group
}

// runDocumentDelete deletes a document of a voucher (class destructive:
// confirmation with the document ID typed in, or --yes). Output is
// {"id":N,"deleted":true} with the document ID.
func (a *App) runDocumentDelete(c *cobra.Command, e registry.Entry, args []string) error {
	voucher, _ := parseID(args[0]) // checked by idsArgs
	doc, _ := parseID(args[1])
	params := []int64{voucher, doc}
	client, r, err := a.mutationClient(e)
	if err != nil {
		return err
	}
	// the document has no GET of its own: the summary names both IDs
	sum := Summary{Resource: fmt.Sprintf("%s %d: Datei", resourceNoun(e.Resource), voucher), ID: doc}
	if proceed, err := a.beforeSend(c, e, client, params, nil, sendExtras{summary: &sum}); !proceed || err != nil {
		return err
	}
	if _, err := client.Call(c.Context(), e.OperationID, params, nil, nil); err != nil {
		return err
	}
	o := a.outputOptions(c.Context(), a.outputFormat(r))
	if ownText(o) {
		_, err := fmt.Fprintf(a.Out, "%s %d: Datei %d gelöscht.\n", e.Resource, voucher, doc)
		return afterRequest(err)
	}
	return afterRequest(output.RenderValue(o, deletedJSON{ID: doc, Deleted: true}, output.Columns{}))
}

// uploadFile is a checked file of an upload, open for reading, with the
// operation and form field it is sent with.
type uploadFile struct {
	f           *os.File
	abs         string // the absolute path (retry command)
	name        string // the name sent (uploadName)
	size        int64
	opID, field string
}

// openUpload checks and opens the file at path for the upload entry e
// before anything is resolved or sent: it must exist, be a regular file,
// readable and not empty; otherwise it is a usage error (exit 2). label
// prefixes the messages ("--file: "). The form field is the binary
// property of the multipart request schema.
func openUpload(e registry.Entry, label, path string) (*uploadFile, error) {
	field, err := uploadField(e.OperationID)
	if err != nil {
		return nil, errUnavailable(err)
	}
	if path == "" {
		return nil, clierr.New(clierr.CodeUsage, label+"Dateiname fehlt", "Datei angeben, z. B. beleg.pdf")
	}
	// Stat first: opening a FIFO would block
	fi, err := os.Stat(path)
	if err == nil {
		err = checkUploadInfo(label, path, fi)
	}
	if err != nil {
		return nil, uploadFileError(label, path, err)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, uploadFileError(label, path, err)
	}
	// the file opened is the one checked and sent
	if fi, err = f.Stat(); err == nil {
		err = checkUploadInfo(label, path, fi)
	}
	if err != nil {
		_ = f.Close()
		return nil, uploadFileError(label, path, err)
	}
	return &uploadFile{f: f, abs: absPath(path), name: uploadName(path), size: fi.Size(), opID: e.OperationID, field: field}, nil
}

// absPath is path made absolute against the working directory the file
// was opened in, so a command naming it works from any directory and never
// starts with "-"; when that fails, a path starting with "-" gets "./".
func absPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	if strings.HasPrefix(path, "-") {
		return "." + string(filepath.Separator) + path
	}
	return path
}

// checkUploadInfo rejects what is no regular file with content.
func checkUploadInfo(label, path string, fi fs.FileInfo) error {
	hint := "eine vorhandene, nicht leere Datei angeben; es wurde nichts gesendet"
	switch {
	case fi.IsDir():
		return clierr.New(clierr.CodeUsage, fmt.Sprintf("%s%q ist ein Verzeichnis", label, path), hint)
	case !fi.Mode().IsRegular():
		return clierr.New(clierr.CodeUsage, fmt.Sprintf("%s%q ist keine reguläre Datei", label, path), hint)
	case fi.Size() == 0:
		return clierr.New(clierr.CodeUsage, fmt.Sprintf("%sDatei %q ist leer", label, path), hint)
	}
	return nil
}

// uploadFileError maps a failed check or open of the file to a usage error
// (exit 2): nothing has been sent.
func uploadFileError(label, path string, err error) error {
	var ce *clierr.Error
	if errors.As(err, &ce) {
		return ce
	}
	hint := "Pfad prüfen; es wurde nichts gesendet"
	switch {
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENOTDIR):
		return clierr.New(clierr.CodeUsage, fmt.Sprintf("%sDatei %q nicht gefunden", label, path), hint)
	case errors.Is(err, fs.ErrPermission):
		return clierr.New(clierr.CodeUsage, fmt.Sprintf("%skeine Leseberechtigung für %q", label, path), hint)
	}
	reason := err.Error()
	var pe *fs.PathError
	if errors.As(err, &pe) {
		reason = pe.Err.Error()
	}
	return clierr.New(clierr.CodeUsage, fmt.Sprintf("%sDatei %q nicht lesbar: %s", label, path, reason), hint)
}

// uploadField is the form field of the file: the one binary property of
// the multipart/form-data request schema of opID (spec: file).
func uploadField(opID string) (string, error) {
	op, ok := spec.OperationByID(opID)
	if !ok || op.Op.RequestBody == nil || op.Op.RequestBody.Value == nil {
		return "", fmt.Errorf("%s hat keinen Anfragekörper", opID)
	}
	mt := op.Op.RequestBody.Value.Content.Get("multipart/form-data")
	if mt == nil || mt.Schema == nil || mt.Schema.Value == nil {
		return "", fmt.Errorf("%s erwartet kein multipart/form-data", opID)
	}
	var fields []string
	for name, p := range mt.Schema.Value.Properties {
		if p != nil && p.Value != nil && p.Value.Format == "binary" {
			fields = append(fields, name)
		}
	}
	if len(fields) != 1 {
		return "", fmt.Errorf("%s: %d Dateifelder im Anfragekörper, erwartet 1", opID, len(fields))
	}
	return fields[0], nil
}

// uploadName is the file name sent: the last element of path (also after
// a backslash, so no Windows path goes out), without control and invisible
// format characters (see invisible) and surrounding spaces; "datei" when
// nothing is left.
func uploadName(path string) string {
	name := filepath.Base(path)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if invisible(r) {
			return -1
		}
		return r
	}, name))
	if name == "" || name == "." || name == ".." {
		return "datei"
	}
	return name
}

// info is the file as --dry-run shows it, with the media type that is sent.
func (u *uploadFile) info() *UploadInfo {
	return &UploadInfo{Name: u.name, Size: u.size, MIME: api.MediaType(u.name)}
}

// send uploads the file with the size checked by openUpload.
func (u *uploadFile) send(c *cobra.Command, client *api.Client, params []int64) (*api.Response, error) {
	return client.Upload(c.Context(), u.opID, params, u.field, u.name, u.f, u.size)
}

func (u *uploadFile) close() { _ = u.f.Close() }

// addCreateFile adds --file to voucher create (spec 9.3): create the
// voucher, then upload the file to it. Without --file the command runs
// runWrite unchanged.
func (a *App) addCreateFile(e registry.Entry, cmd *cobra.Command) {
	up, ok := registry.Lookup(e.Resource, "upload")
	if !ok {
		// fail closed: without the upload entry --file cannot work
		panic(fmt.Sprintf("registry: %s create --file ohne Eintrag %s upload", e.Resource, e.Resource))
	}
	a.addCreateFileWith(e, up, cmd)
}

// addCreateFileWith is addCreateFile with the upload entry up. It fails
// closed (panic, like entryKind) when up does not fit checkCreateFile.
func (a *App) addCreateFileWith(e, up registry.Entry, cmd *cobra.Command) {
	if err := checkCreateFile(e, up); err != nil {
		panic(err)
	}
	cmd.Long += fmt.Sprintf(createFileLong, moreCredits(up.Credits))
	cmd.Flags().String("file", "", "Datei (Belegbild), die nach dem Anlegen an den Beleg hochgeladen wird")
	write := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if !c.Flags().Changed("file") {
			return write(c, args)
		}
		return a.runWriteThen(c, e, args, writeSteps{up: &createUpload{entry: up, path: flagString(c, "file")}})
	}
}

// createUpload is the upload after create: the upload entry and the path
// of --file, opened by open.
type createUpload struct {
	entry registry.Entry
	path  string
	file  *uploadFile
}

func (up *createUpload) open() (err error) {
	up.file, err = openUpload(up.entry, "--file: ", up.path)
	return err
}

func (up *createUpload) close() { up.file.close() }

// checkCreateFile requires the upload of create --file to have the class
// of create: the run asks no confirmation of its own and passes one
// read-only gate, so an upload that needed a confirmation (or any other
// class) must not be built silently.
func checkCreateFile(create, upload registry.Entry) error {
	if upload.Class != create.Class {
		return fmt.Errorf("registry: %s %s hat Klasse %q, %s %s aber %q – %s create --file müsste dafür Bestätigung und Schreibschutz eigens behandeln",
			upload.Resource, upload.Action, upload.Class, create.Resource, create.Action, create.Class, create.Resource)
	}
	return nil
}

// run is the entry of the whole run for the protection layer: the credits
// of create and upload together (--dry-run); the class is that of both
// (checkCreateFile).
func (up *createUpload) run(e registry.Entry) registry.Entry {
	run := e
	run.Credits += up.entry.Credits
	return run
}

// dryRunStep is the upload as the second request of the dry run: method,
// URL with the ID still a template ({voucher_id}; display only), credits
// and the file.
func (up *createUpload) dryRunStep(client *api.Client) *UploadInfo {
	info := up.file.info()
	info.Method, info.URL, info.Credits = up.entry.Method, client.TemplateURL(up.entry.Path), up.entry.Credits
	info.Then = fmt.Sprintf("Danach (zweite Anfrage, an den neuen %s)", resourceNoun(up.entry.Resource))
	return info
}

// partialJSON is the result of create --file when only the upload failed
// (spec 9.3, exit 11). id and retry_command are null when the API sent no
// ID: a command with a placeholder could be run by mistake.
type partialJSON struct {
	ID           *int64   `json:"id"`
	Completed    []string `json:"completed"`
	Failed       []string `json:"failed"`
	RetryCommand *string  `json:"retry_command"`
	Warnings     []string `json:"warnings"`
}

// uploadCreated uploads the file to the object resp created. Success
// prints the created object like create; a failed upload is exit 11
// (uploadPartial). Nothing is retried or deleted automatically.
func (a *App) uploadCreated(c *cobra.Command, e registry.Entry, client *api.Client, r *config.Resolved, resp *api.Response, up *createUpload) error {
	o := a.outputOptions(c.Context(), a.outputFormat(r))
	id, ok := createdID(resp.Body)
	if !ok {
		return a.uploadPartial(o, e, up, nil, clierr.New(clierr.CodeError, "API lieferte keine ID des angelegten Objekts", ""))
	}
	if _, err := up.file.send(c, client, []int64{id}); err != nil {
		return a.uploadPartial(o, e, up, &id, err)
	}
	return a.render(c, resp.Body, e.Columns)
}

// createdID is the id of the object in a create answer.
func createdID(body []byte) (int64, bool) {
	var obj struct {
		ID json.Number `json:"id"`
	}
	if json.Unmarshal(body, &obj) != nil {
		return 0, false
	}
	return parseID(obj.ID.String())
}

// uploadPartial reports a create whose upload failed (spec 9.3, 10.4): the
// partial result on stdout (structured: partialJSON; table: German lines),
// the warning on stderr, and exit 11 with the command that uploads the file
// alone (absolute path, shell-quoted). When the upload may have arrived (no
// answer, network error, 5xx), the warning says to check the object with
// describe first. Without an ID there is no command at all.
func (a *App) uploadPartial(o output.Options, e registry.Entry, up *createUpload, id *int64, err error) error {
	ce := clierr.From(err)
	noun := resourceNoun(e.Resource)
	again := fmt.Sprintf("NICHT den ganzen Befehl wiederholen (sonst ein zweiter %s)", noun)
	out := &clierr.Error{Code: clierr.CodePartial, HTTPStatus: ce.HTTPStatus, CorrelationID: ce.CorrelationID, Cause: err}
	var warning, text string
	var retry *string
	if id == nil {
		warning = fmt.Sprintf("%s angelegt, %s-ID unbekannt (%s): Datei nicht hochgeladen – %s per papierkram %s list suchen und die Datei mit %s %s an ihn hochladen; %s",
			noun, noun, ce.Message, noun, e.Resource, up.entry.Resource, up.entry.Action, again)
		out.Message = fmt.Sprintf("%s angelegt, %s-ID unbekannt: %s; Datei nicht hochgeladen", noun, noun, ce.Message)
		out.Hint = fmt.Sprintf("%s per papierkram %s list suchen und die Datei mit %s %s an ihn hochladen – %s", noun, e.Resource, up.entry.Resource, up.entry.Action, again)
		text = fmt.Sprintf("%s angelegt, Datei nicht hochgeladen (erledigt: create; fehlgeschlagen: upload).\n%s-ID unbekannt: per papierkram %s list suchen.\n",
			e.Resource, noun, e.Resource)
	} else {
		cmd := fmt.Sprintf("papierkram %s %s %d %s", up.entry.Resource, up.entry.Action, *id, shellQuote(up.file.abs))
		retry = &cmd
		describe := fmt.Sprintf("papierkram %s describe %d", e.Resource, *id)
		if uploadUncertain(ce) {
			warning = fmt.Sprintf("%s %d angelegt; Ergebnis des Hochladens unbekannt: %s – die Datei ist möglicherweise schon angehängt; vor dem Nachholen per %s prüfen, dann nur das Hochladen nachholen: %s; %s",
				noun, *id, ce.Message, describe, cmd, again)
			out.Message = fmt.Sprintf("%s %d angelegt, Ergebnis des Hochladens unbekannt: %s", noun, *id, ce.Message)
			out.Hint = fmt.Sprintf("vor dem Nachholen per %s prüfen, ob die Datei schon angehängt ist; sonst nur das Hochladen nachholen: %s – %s", describe, cmd, again)
		} else {
			warning = fmt.Sprintf("%s %d angelegt, Datei nicht hochgeladen: %s – nur das Hochladen nachholen: %s; %s", noun, *id, ce.Message, cmd, again)
			out.Message = fmt.Sprintf("%s %d angelegt, Hochladen fehlgeschlagen: %s", noun, *id, ce.Message)
			out.Hint = fmt.Sprintf("nur das Hochladen nachholen: %s – %s", cmd, again)
		}
		text = fmt.Sprintf("%s %d angelegt, Datei nicht hochgeladen (erledigt: create; fehlgeschlagen: upload).\nNachholen: %s\n", e.Resource, *id, cmd)
	}
	if timedOut(err) {
		out.Hint += "; " + timeoutHint
	}
	output.Warn(a.Err, warning)
	var werr error
	if ownText(o) {
		_, werr = io.WriteString(a.Out, text)
	} else {
		werr = output.RenderValue(o, partialJSON{ID: id, Completed: []string{"create"}, Failed: []string{"upload"},
			RetryCommand: retry, Warnings: []string{warning}}, output.Columns{})
	}
	if werr != nil {
		out.Message += "; Ausgabe fehlgeschlagen: " + clierr.From(werr).Message
	}
	return out
}

// uploadUncertain reports an upload failure that does not prove the file
// was not attached: no complete answer (outcome_unknown, network) or a 5xx.
// Every 4xx, also one without its own code (413, 415), and a file that
// could not be read (no HTTP status) are clean failures.
func uploadUncertain(ce *clierr.Error) bool {
	switch ce.Code {
	case clierr.CodeOutcomeUnknown, clierr.CodeNetwork, clierr.CodeServer:
		return true
	}
	return ce.HTTPStatus >= 500
}

// timeoutHint names --timeout: the whole file has to go out within it.
const timeoutHint = "größere Dateien brauchen ggf. --timeout 120s"

// timedOut reports an upload that ran into the request timeout.
func timedOut(err error) bool { return errors.Is(err, context.DeadlineExceeded) }

// withTimeoutHint adds timeoutHint to the hint of an upload error caused
// by the timeout; the code (network, outcome_unknown) stays.
func withTimeoutHint(err error) error {
	if !timedOut(err) {
		return err
	}
	ce := *clierr.From(err)
	if ce.Hint != "" {
		ce.Hint += "; "
	}
	ce.Hint += timeoutHint
	return &ce
}

// shellSafe are arguments a POSIX shell takes as they are.
var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellQuote quotes s for a POSIX shell when needed (spaces, quotes,
// other special or non-ASCII characters): in single quotes, each single
// quote closed, escaped and reopened (see TestShellQuote).
func shellQuote(s string) string {
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
