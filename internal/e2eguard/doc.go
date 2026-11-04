// Package e2eguard holds the safety logic of the opt-in live suite in e2e/
// (spec 11.1): the decision whether the suite may run at all, the name
// prefix of everything it creates, the credit meter, token redaction and an
// HTTP transport that only talks to the sandbox host, refuses deliver other
// than as PDF and stops at the credit budget. It lives outside e2e/ so that
// the normal `go test ./...` (without the e2e build tag) tests it.
package e2eguard
