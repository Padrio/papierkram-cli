package e2eguard

import (
	"bytes"
	"encoding/json"
	"strconv"
)

// followUpTypes is the value of "type" in the answer objects of a CLI
// resource (api/openapi.json: the one-value enum of type in describe,
// cancel and reverse; TestFollowUpTypesMatchSpec). Only these resources
// have a cancel or reverse.
var followUpTypes = map[string]string{
	"voucher":  "expense_voucher",
	"invoice":  "invoice",
	"estimate": "estimate",
}

// FollowUp is an object other than the target that the answer to a cancel
// or reverse names (a cancellation document, a reverse entry).
type FollowUp struct {
	ID   int64
	Name string // name of the answer; empty when missing or no string
	Type string // type of the answer; empty when missing or no string
	// Track reports whether Type is the type of the resource of the
	// target: only then is ID in the ID space of that resource, and only
	// then may the cleanup run "<resource> delete ID".
	Track bool
}

// JudgeFollowUp reads the answer to a cancel or reverse of resource
// target. It reports a follow-up when the answer is a JSON object whose id
// (a positive integer) differs from target; target 0 (unknown) never has
// one.
func JudgeFollowUp(resource string, target int64, answer []byte) (FollowUp, bool) {
	if target <= 0 {
		return FollowUp{}, false
	}
	var m map[string]any
	d := json.NewDecoder(bytes.NewReader(answer))
	d.UseNumber()
	if d.Decode(&m) != nil || m == nil {
		return FollowUp{}, false
	}
	n, ok := m["id"].(json.Number)
	if !ok {
		return FollowUp{}, false
	}
	id, err := strconv.ParseInt(n.String(), 10, 64)
	if err != nil || id <= 0 || id == target {
		return FollowUp{}, false
	}
	f := FollowUp{ID: id}
	f.Name, _ = m["name"].(string)
	f.Type, _ = m["type"].(string)
	f.Track = IsFollowUpType(resource, f.Type)
	return f, true
}

// IsFollowUpType reports whether typ is the type of the answer objects of
// resource, a resource with cancel or reverse: only an object of that
// type lies in the ID space of resource. JudgeFollowUp sets Track with it,
// and the cleanup of the live suite checks it again before it touches a
// follow-up.
func IsFollowUpType(resource, typ string) bool {
	want, known := followUpTypes[resource]
	return known && typ == want
}
