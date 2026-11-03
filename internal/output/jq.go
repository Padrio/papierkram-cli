package output

import (
	"bytes"
	"context"
	"errors"

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
// The expression sees no environment ($ENV and env are empty): gojq only
// reads the environment through an explicit loader, so tokens in
// PAPIERKRAM_TOKEN cannot leak through --jq.
func renderJQ(o Options, raw []byte) error {
	query, err := gojq.Parse(o.JQ)
	if err != nil {
		return jqErr("ungültiger jq-Ausdruck", err)
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return jqErr("ungültiger jq-Ausdruck", err)
	}
	v, err := decode(raw)
	if err != nil {
		return err
	}
	if len(o.Fields) > 0 {
		v = project(v, o.Fields)
	}
	var b bytes.Buffer
	iter := code.RunWithContext(context.Background(), toPlain(v))
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
			return jqErr("jq-Ausdruck fehlgeschlagen", err)
		}
		if s, ok := r.(string); ok {
			b.WriteString(s)
		} else {
			j, err := gojq.Marshal(r)
			if err != nil {
				return jqErr("jq-Ergebnis nicht darstellbar", err)
			}
			b.Write(j)
		}
		b.WriteByte('\n')
	}
	_, err = o.Out.Write(b.Bytes())
	return err
}

// jqErr is a usage error (exit 2): the expression is part of the call. The
// detail of gojq stays English, it quotes the expression.
func jqErr(what string, err error) error {
	e := clierr.New(clierr.CodeUsage, what+": "+err.Error(),
		"Syntax siehe https://jqlang.org/manual/; Zeichenketten werden ohne Anführungszeichen ausgegeben")
	e.Cause = err
	return e
}
