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

// prompt writes question to stderr and reads one line of input (without
// the line ending). An interrupt ends it with errInterrupted (exit 10);
// end of input or a read error count as an empty answer.
func (a *App) prompt(ctx context.Context, question string) (string, error) {
	_, _ = fmt.Fprint(a.Err, question)
	line, err := a.readCtx(ctx, func() ([]byte, error) {
		s, _ := bufio.NewReader(a.In).ReadString('\n')
		return []byte(s), nil
	})
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(line), "\r\n"), nil
}
