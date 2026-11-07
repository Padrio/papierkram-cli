//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/e2eguard"
)

// This file is the only place of the suite that writes test output: every
// line goes through the redaction of the token first, and is checked to be
// free of it afterwards. internal/e2eguard (TestE2ESuiteSource) fails when
// another file of e2e/ calls t.Log, t.Error, t.Fatal, t.Skip and their f
// forms, or prints to stdout or stderr itself.

// live returns the suite, or skips the test when TestMain decided not to
// run against a sandbox. Every test calls it first (checked by
// TestE2ESuiteSource), so a skipped run sends no request at all.
func live(t *testing.T) *liveSuite {
	t.Helper()
	if suite == nil {
		t.Skip(skipReason)
	}
	return suite
}

// safe is msg with the token redacted; a token left after that is a bug
// of the redaction and stops the run without printing msg.
func (s *liveSuite) safe(msg string) string {
	out := s.red.String(msg)
	if s.red.Contains(out) {
		panic("e2e: Token nach der Schwärzung noch in einer Logzeile")
	}
	return out
}

func (s *liveSuite) logf(t testing.TB, format string, args ...any) {
	t.Helper()
	t.Log(s.safe(fmt.Sprintf(format, args...)))
}

// measuref logs the result of a measurement (spec 17, task 44 carries)
// with the marker MESSUNG, so a run can be searched for them.
func (s *liveSuite) measuref(t testing.TB, topic, format string, args ...any) {
	t.Helper()
	t.Log(s.safe("MESSUNG " + topic + ": " + fmt.Sprintf(format, args...)))
}

// deviatef logs a measurement that differs from what the CLI assumes,
// with the marker ABWEICHUNG, and collects it for the summary at the end
// of the run. It does not fail the test (controller ruling: such checks
// are measurements).
func (s *liveSuite) deviatef(t testing.TB, format string, args ...any) {
	t.Helper()
	msg := s.safe(fmt.Sprintf(format, args...))
	s.deviations = append(s.deviations, t.Name()+": "+msg)
	t.Log("ABWEICHUNG: " + msg)
}

func (s *liveSuite) errorf(t testing.TB, format string, args ...any) {
	t.Helper()
	t.Error(s.safe(fmt.Sprintf(format, args...)))
}

func (s *liveSuite) fatalf(t testing.TB, format string, args ...any) {
	t.Helper()
	t.Fatal(s.safe(fmt.Sprintf(format, args...)))
}

func (s *liveSuite) skipf(t testing.TB, format string, args ...any) {
	t.Helper()
	t.Skip(s.safe(fmt.Sprintf(format, args...)))
}

// stderrf writes a line of TestMain (no testing.T there: cleanup, credit
// total) to stderr.
func (s *liveSuite) stderrf(format string, args ...any) {
	_, _ = fmt.Fprintln(os.Stderr, s.safe("e2e: "+fmt.Sprintf(format, args...)))
}

// deviationSummary writes the collected deviations to stderr (TestMain).
func (s *liveSuite) deviationSummary() {
	if len(s.deviations) == 0 {
		s.stderrf("keine Abweichungen")
		return
	}
	s.stderrf("%d Abweichung(en):", len(s.deviations))
	for _, d := range s.deviations {
		s.stderrf("ABWEICHUNG: %s", d)
	}
}

// coverageSummary writes the coverage report of the run (one ABDECKUNG
// line each) to stderr and returns it (TestMain).
func (s *liveSuite) coverageSummary() e2eguard.CoverageReport {
	rep := s.coverage.Report(e2eguard.CoveragePlan)
	for _, l := range rep.Lines {
		s.stderrf("%s", l)
	}
	return rep
}

// stderrPlain writes a line of TestMain before the suite exists; it never
// contains a value of the environment.
func stderrPlain(msg string) {
	_, _ = fmt.Fprintln(os.Stderr, "e2e: "+msg)
}
