package spec

import (
	_ "embed"
	"fmt"
	"maps"
	"mime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
)

//go:embed openapi.gen.json
var raw []byte

// apiPrefix is the base path of the API; MatchPath accepts paths with or
// without it.
const apiPrefix = "/api/v1"

// Raw returns the embedded patched spec. The slice is shared; do not modify it.
func Raw() []byte { return raw }

var (
	docOnce sync.Once
	doc     *openapi3.T
	ops     []Operation
	byID    map[string]Operation
	byShape map[string][]template
)

// Doc returns the parsed patched spec. A spec that does not load is a build
// defect (the embedded file is generated and tested), so it panics.
func Doc() *openapi3.T {
	load()
	return doc
}

// Operation is one API operation of the spec.
type Operation struct {
	ID     string
	Method string // upper case
	Path   string // template, e.g. /income/invoices/{id}/deliver
	Op     *openapi3.Operation
}

// template is an Operation split into path segments for MatchPath.
type template struct {
	op       Operation
	segments []string // "{name}" for parameters
	literals int
}

func load() {
	docOnce.Do(func() {
		d, err := openapi3.NewLoader().LoadFromData(raw)
		if err != nil {
			panic(fmt.Sprintf("spec: embedded openapi.gen.json does not load (run `go generate ./internal/spec/`): %v", err))
		}
		doc = d
		byID = map[string]Operation{}
		byShape = map[string][]template{}
		for path, item := range d.Paths.Map() {
			for method, op := range item.Operations() {
				o := Operation{ID: op.OperationID, Method: strings.ToUpper(method), Path: path, Op: op}
				ops = append(ops, o)
				byID[o.ID] = o
				t := template{op: o, segments: strings.Split(strings.Trim(path, "/"), "/")}
				for _, s := range t.segments {
					if !isParam(s) {
						t.literals++
					}
				}
				key := o.Method + " " + strconv.Itoa(len(t.segments))
				byShape[key] = append(byShape[key], t)
			}
		}
		// byShape was filled in map order. Sort the candidates by template path so
		// MatchPath breaks ties between equal literal counts deterministically
		// (the first, i.e. lexically smallest template, wins).
		for _, ts := range byShape {
			sort.Slice(ts, func(i, j int) bool { return ts[i].op.Path < ts[j].op.Path })
		}
		sort.Slice(ops, func(i, j int) bool {
			if ops[i].Path != ops[j].Path {
				return ops[i].Path < ops[j].Path
			}
			return ops[i].Method < ops[j].Method
		})
	})
}

func isParam(segment string) bool {
	return strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}")
}

// Operations returns all operations sorted by path, then method.
func Operations() []Operation {
	load()
	return append([]Operation(nil), ops...)
}

// OperationByID looks an operation up by its operationId.
func OperationByID(id string) (Operation, bool) {
	load()
	o, ok := byID[id]
	return o, ok
}

// MatchPath finds the operation for a concrete request path, e.g.
// "/income/invoices/12/deliver" yields DeliverInvoice and [12]. A leading
// /api/v1, a query string and a trailing slash are ignored. The returned IDs
// are the path parameters in template order. A parameter segment that is not a
// non-negative integer never matches. Where a literal and a parameter segment
// both fit, the template with more literal segments wins; on a tie the
// lexically smallest template path wins.
func MatchPath(method, concretePath string) (Operation, []int64, bool) {
	load()
	p, _, _ := strings.Cut(concretePath, "?")
	if rest, ok := strings.CutPrefix(p, apiPrefix); ok && (rest == "" || rest[0] == '/') {
		p = rest
	}
	p = strings.Trim(p, "/")
	if p == "" {
		return Operation{}, nil, false
	}
	segs := strings.Split(p, "/")

	var best Operation
	var bestIDs []int64
	bestLiterals, found := -1, false
	for _, t := range byShape[strings.ToUpper(method)+" "+strconv.Itoa(len(segs))] {
		ids, ok := t.match(segs)
		if ok && t.literals > bestLiterals {
			best, bestIDs, bestLiterals, found = t.op, ids, t.literals, true
		}
	}
	return best, bestIDs, found
}

// MatchPathLoose finds the operation a concrete path probably means when
// MatchPath finds none: literal segments compared without case, and any
// non-empty segment where the template has a parameter. canonical is the
// template path with its literals and, for each parameter, the leading
// digits of the given segment (12-x → 12) or the parameter itself ({id})
// when there are none. Prefix, query and trailing slash are handled like in
// MatchPath; the template with most literals wins, ties go to the
// lexically smallest. papierkram api rejects such a path (exit 2), so a
// form the server may route to that operation never escapes its rules.
func MatchPathLoose(method, concretePath string) (op Operation, canonical string, ok bool) {
	load()
	p, _, _ := strings.Cut(concretePath, "?")
	if rest, found := strings.CutPrefix(p, apiPrefix); found && (rest == "" || rest[0] == '/') {
		p = rest
	}
	p = strings.Trim(p, "/")
	if p == "" {
		return Operation{}, "", false
	}
	segs := strings.Split(p, "/")
	bestLiterals := -1
	for _, t := range byShape[strings.ToUpper(method)+" "+strconv.Itoa(len(segs))] {
		c, matched := t.looseMatch(segs)
		if matched && t.literals > bestLiterals {
			op, canonical, ok, bestLiterals = t.op, c, true, t.literals
		}
	}
	return op, canonical, ok
}

// looseMatch is match for MatchPathLoose; it returns the canonical path.
func (t template) looseMatch(segs []string) (string, bool) {
	out := make([]string, len(segs))
	for i, ts := range t.segments {
		switch {
		case !isParam(ts) && !strings.EqualFold(ts, segs[i]):
			return "", false
		case !isParam(ts):
			out[i] = ts
		case segs[i] == "":
			return "", false
		default:
			digits := segs[i]
			if n := strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }); n >= 0 {
				digits = digits[:n]
			}
			if digits == "" {
				digits = ts
			}
			out[i] = digits
		}
	}
	return "/" + strings.Join(out, "/"), true
}

func (t template) match(segs []string) ([]int64, bool) {
	var ids []int64
	for i, ts := range t.segments {
		if !isParam(ts) {
			if ts != segs[i] {
				return nil, false
			}
			continue
		}
		id, ok := parseID(segs[i])
		if !ok {
			return nil, false
		}
		ids = append(ids, id)
	}
	return ids, true
}

func parseID(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	id, err := strconv.ParseInt(s, 10, 64)
	return id, err == nil
}

// RequestSchema returns the schema of the request body: application/json, else
// multipart/form-data. It is nil for unknown operations and operations without
// a body.
func RequestSchema(opID string) *openapi3.Schema {
	o, ok := OperationByID(opID)
	if !ok || o.Op.RequestBody == nil || o.Op.RequestBody.Value == nil {
		return nil
	}
	for _, ct := range []string{"application/json", "multipart/form-data"} {
		if mt := o.Op.RequestBody.Value.Content[ct]; mt != nil && mt.Schema != nil {
			return mt.Schema.Value
		}
	}
	return nil
}

// ResponseSchema returns the application/json schema of the first 2xx
// response that has one (lowest status code first). allOf is left unresolved.
func ResponseSchema(opID string) *openapi3.Schema {
	for _, r := range successResponses(opID) {
		if mt := r.Content["application/json"]; mt != nil && mt.Schema != nil {
			return mt.Schema.Value
		}
	}
	return nil
}

// FileMediaType returns the media type of a file answer (application/pdf):
// the first non-JSON content type of the first 2xx response that has
// content, lowercased without parameters. JSON in any spelling
// (application/json with parameters, *+json) is no file type. It is "" for
// JSON operations, operations without content and unknown operations.
func FileMediaType(opID string) string { return fileMediaType(successResponses(opID)) }

// FileSchema returns the schema of the file answer FileMediaType names
// (the binary string of a PDF download); nil when there is none.
func FileSchema(opID string) *openapi3.Schema {
	_, mt := fileContent(successResponses(opID))
	if mt == nil || mt.Schema == nil {
		return nil
	}
	return mt.Schema.Value
}

func fileMediaType(resps []*openapi3.Response) string {
	t, _ := fileContent(resps)
	return t
}

// fileContent finds the file answer of resps: the first non-JSON content
// type of the first response that has content, with its media type
// object. FileMediaType and FileSchema share it.
func fileContent(resps []*openapi3.Response) (string, *openapi3.MediaType) {
	for _, r := range resps {
		if len(r.Content) == 0 {
			continue
		}
		for _, ct := range slices.Sorted(maps.Keys(r.Content)) {
			mt, _, err := mime.ParseMediaType(ct)
			if err == nil && mt != "application/json" && !strings.HasSuffix(mt, "+json") {
				return mt, r.Content[ct]
			}
		}
		return "", nil
	}
	return "", nil
}

// successResponses returns the 2xx responses of opID, lowest status code
// first.
func successResponses(opID string) []*openapi3.Response {
	o, ok := OperationByID(opID)
	if !ok || o.Op.Responses == nil {
		return nil
	}
	var codes []string
	for code := range o.Op.Responses.Map() {
		if strings.HasPrefix(code, "2") {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)
	var out []*openapi3.Response
	for _, code := range codes {
		if r := o.Op.Responses.Map()[code]; r.Value != nil {
			out = append(out, r.Value)
		}
	}
	return out
}

// ListEntryProperty returns the schema of the property name of the entries
// of a list operation (envelope with entries[], allOf resolved). It is nil
// for unknown operations, operations without a list response and unknown
// properties.
func ListEntryProperty(opID, name string) *openapi3.Schema {
	s := ResponseSchema(opID)
	if s == nil {
		return nil
	}
	if e := s.Properties["entries"]; e == nil || e.Value == nil {
		return nil
	}
	if found := children(items([]*openapi3.Schema{s}), name); len(found) > 0 {
		return found[0]
	}
	return nil
}

// QueryParams returns the query parameters of an operation by name, including
// those declared on the path item (operation level wins).
func QueryParams(opID string) map[string]*openapi3.Parameter {
	o, ok := OperationByID(opID)
	if !ok {
		return nil
	}
	params := map[string]*openapi3.Parameter{}
	add := func(list openapi3.Parameters) {
		for _, p := range list {
			if p.Value != nil && p.Value.In == "query" {
				params[p.Value.Name] = p.Value
			}
		}
	}
	add(Doc().Paths.Value(o.Path).Parameters)
	add(o.Op.Parameters)
	return params
}
