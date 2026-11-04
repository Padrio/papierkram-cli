package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/registry"
)

// UploadInfo describes the file of an upload in a dry run (spec 8.4:
// file name, size, MIME type). When the upload is a request of its own
// after the one shown (voucher create --file), Method, URL (a path
// template: the ID is not known yet; display only) and Credits describe
// it, and Then is the German heading of that second request.
type UploadInfo struct {
	Method  string `json:"method,omitempty"`
	URL     string `json:"url,omitempty"`
	Credits int    `json:"credits,omitempty"`
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	MIME    string `json:"mime_type"`
	Then    string `json:"-"`
}

// step reports an upload shown as a request of its own.
func (u *UploadInfo) step() bool { return u != nil && u.URL != "" }

// dryRunJSON is the -o json contract of --dry-run (spec 8.4).
type dryRunJSON struct {
	DryRun  bool        `json:"dry_run"`
	Method  string      `json:"method"`
	URL     string      `json:"url"`
	Body    any         `json:"body"`
	Upload  *UploadInfo `json:"upload,omitempty"`
	Credits int         `json:"credits"`
}

// dryRun prints the request a mutation would send, without sending it
// (spec 8.4): method, full URL including the query, body (null without
// one), the upload file if any, and the credit cost of the entry. -o
// json/yaml, --fields and --jq render the dryRunJSON object; table and
// wide print German lines. Nothing has been sent, so output errors keep
// their code (a usage error stays exit 2).
func (a *App) dryRun(ctx context.Context, e registry.Entry, method, url string, body any, upload *UploadInfo) error {
	v := dryRunJSON{DryRun: true, Method: method, URL: url, Body: body, Upload: upload, Credits: e.Credits}
	o := a.outputOptions(ctx, a.outputFormat(a.resolved))
	if !ownText(o) {
		return output.RenderValue(o, v, output.Columns{})
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Probelauf – nichts gesendet\nMethode: %s\nURL:     %s\nCredits: %d", method, url, e.Credits)
	if upload.step() {
		b.WriteString(" (beide Anfragen)")
	}
	b.WriteByte('\n')
	if upload != nil && !upload.step() {
		fmt.Fprintf(&b, "Datei:   %s (%d Bytes, %s)\n", upload.Name, upload.Size, upload.MIME)
	}
	if body == nil {
		b.WriteString("Body:    –\n")
	} else {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(body); err != nil {
			return err
		}
		b.WriteString("Body:\n")
		b.Write(buf.Bytes())
	}
	if upload.step() {
		fmt.Fprintf(&b, "\n%s:\nMethode: %s\nURL:     %s\nCredits: %d\nDatei:   %s (%d Bytes, %s)\n",
			upload.Then, upload.Method, upload.URL, upload.Credits, upload.Name, upload.Size, upload.MIME)
	}
	_, err := fmt.Fprint(a.Out, b.String())
	return err
}
