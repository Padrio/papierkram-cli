package output

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/itchyny/gojq"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// renderJQ applies the --jq expression (after --fields) and prints one
// result per line: strings raw (like gh --jq), everything else as compact
// JSON. Output is written only when the whole expression succeeded.
//
// Numbers: the input keeps json.Number literals and gojq prints them
// unchanged (".total_gross" → 1190.00, ".id" → 9007199254740993). Once an
// expression computes with a number, gojq uses int, *big.Int or float64:
// integers stay exact, decimals are normalized (1190.00+0 → 1190).
// Objects are printed with sorted keys (gojq does not keep key order).
//
// Runtime and output errors are exit 1 (see outputErr); the expression
// runs with o.Context, so it can be cancelled, and the buffered output is
// capped at jqOutputLimit.
//
// The expression sees no environment ($ENV and env are empty): gojq only
// reads the environment through an explicit loader, so tokens in
// PAPIERKRAM_TOKEN cannot leak through --jq.
func renderJQ(o Options, raw []byte) error {
	code, err := compileJQ(o.JQ)
	if err != nil {
		return err
	}
	v, err := decode(raw)
	if err != nil {
		return err
	}
	if len(o.Fields) > 0 {
		v = project(v, o.Fields)
	}
	ctx := o.Context
	if ctx == nil {
		ctx = context.Background()
	}
	var b bytes.Buffer
	iter := code.RunWithContext(ctx, toPlain(v))
	for {
		r, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := r.(error); ok {
			var halt *gojq.HaltError
			if errors.As(err, &halt) && halt.Value() == nil {
				break
			}
			return outputErr(err.Error(), err)
		}
		if s, ok := r.(string); ok {
			b.WriteString(s)
		} else {
			j, err := gojq.Marshal(r)
			if err != nil {
				return outputErr(err.Error(), err)
			}
			b.Write(j)
		}
		b.WriteByte('\n')
		if b.Len() > jqOutputLimit {
			return outputErr(fmt.Sprintf("jq-Ausgabe größer als %d MiB", jqOutputLimit>>20), nil)
		}
	}
	_, err = o.Out.Write(b.Bytes())
	return err
}

// jqOutputLimit caps the buffered --jq output (e.g. range(1e9)).
var jqOutputLimit = 64 << 20

// CheckJQ parses and compiles a --jq expression. The CLI calls it with the
// global flag checks, before any request is sent; an invalid expression is
// a usage error (exit 2).
func CheckJQ(expr string) error {
	_, err := compileJQ(expr)
	return err
}

func compileJQ(expr string) (*gojq.Code, error) {
	query, err := gojq.Parse(expr)
	if err == nil {
		var code *gojq.Code
		if code, err = gojq.Compile(query); err == nil {
			return code, nil
		}
	}
	// The gojq detail stays English; it quotes the expression.
	e := clierr.New(clierr.CodeUsage, "ungültiger jq-Ausdruck: "+err.Error(), "Syntax siehe https://jqlang.org/manual/")
	e.Cause = err
	return nil, e
}

// outputErr reports a --jq failure that depends on the data: the command
// itself has run (and may have changed data), only its output failed, so
// it is exit 1 and not a usage error that invites a retry.
func outputErr(detail string, cause error) error {
	e := clierr.New(clierr.CodeError, "Befehl ausgeführt, Ausgabe fehlgeschlagen: "+detail, "")
	e.Cause = cause
	return e
}
