package cli

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// errInterrupted is returned when an interrupt (Ctrl+C) ends a running
// keyboard input; exit 10 like an interrupted request.
func errInterrupted() *clierr.Error {
	return clierr.New(clierr.CodeNetwork, "Abgebrochen: Eingabe durch Unterbrechung beendet", "")
}

// readCtx runs the blocking read in a goroutine and returns its result, or
// errInterrupted as soon as ctx is cancelled: the interrupt signal is
// caught for the command context, so a blocking read would otherwise
// swallow Ctrl+C. On cancel the terminal state of the start is restored
// (a hidden token read switches echo off) and the line is ended on stderr.
// The abandoned read ends with the process.
func (a *App) readCtx(ctx context.Context, read func() ([]byte, error)) ([]byte, error) {
	type result struct {
		b   []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		b, err := read()
		done <- result{b, err}
	}()
	select {
	case r := <-done:
		return r.b, r.err
	case <-ctx.Done():
		if a.restoreTerminal != nil {
			a.restoreTerminal()
		}
		_, _ = fmt.Fprintln(a.Err)
		return nil, errInterrupted()
	}
}

// stdin returns the one buffered reader over a.In that every stdin
// consumer of this invocation shares (prompts, --input -, --items -,
// --token-stdin): a reader per call would buffer ahead and lose the input
// of the next prompt on piped stdin.
func (a *App) stdin() *bufio.Reader {
	if a.stdinBuf == nil {
		in := a.In
		if in == nil {
			in = strings.NewReader("")
		}
		a.stdinBuf = bufio.NewReader(in)
	}
	return a.stdinBuf
}

// prompt writes question to stderr and reads one line of input (without
// the line ending). An interrupt ends it with errInterrupted (exit 10);
// end of input or a read error count as an empty answer. A confirmation
// whose empty answer means yes uses ask, so that end of input is no.
func (a *App) prompt(ctx context.Context, question string) (string, error) {
	line, _, err := a.ask(ctx, question)
	return line, err
}

// ask is prompt that also reports the end of the input (Ctrl-D, closed
// stdin, a read error) before anything was typed: eof is true and line
// empty. A last line without line ending is an answer (eof false).
func (a *App) ask(ctx context.Context, question string) (line string, eof bool, err error) {
	_, _ = fmt.Fprint(a.Err, question)
	r := a.stdin()
	var readErr error
	b, err := a.readCtx(ctx, func() ([]byte, error) {
		s, rerr := r.ReadString('\n')
		readErr = rerr
		return []byte(s), nil
	})
	if err != nil {
		return "", false, err
	}
	line = strings.TrimRight(string(b), "\r\n")
	return line, readErr != nil && len(b) == 0, nil
}
