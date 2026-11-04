// Package registry is the command registry (spec 5.3): one Go table that
// maps every API command to its operation, action class, credit cost, list
// filters, table columns, short help and examples. The cobra commands,
// `commands --json`, the coverage test and the protection rules are built
// from it. It must not import internal/cli.
package registry

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/Padrio/papierkram-cli/internal/output"
)

// Class is the action class of a command (spec 8.1). It decides about
// confirmation, the read-only lock and --dry-run.
type Class string

// The action classes of spec 8.1.
const (
	ClassReadOnly    Class = "read_only"
	ClassCreate      Class = "create"
	ClassUpdate      Class = "update"
	ClassArchive     Class = "archive"
	ClassDestructive Class = "destructive"
	ClassSend        Class = "send"
)

// Filter maps a list flag (Flag, without dashes) to a query parameter
// (Param, spec 4.2). Kind tells the builder how to parse the value: "id",
// "date", "datetime_from", "datetime_to" or "enum". Kind "local_enum" is a
// filter the API ignores (company list --type, spec 9.7): Param is then a
// property of the list entries, never sent; the CLI loads all pages and
// keeps the entries whose Param has the given value of its enum. Help is
// the German flag description.
type Filter struct {
	Flag, Param, Kind, Help string
}

// Entry is one API command. Method and Path are those of the spec
// operation OperationID; Credits is the cost per request (spec 4.3). Action
// is empty for a resource that is a command itself (info). Custom marks
// commands with their own handler instead of the generic builder.
//
// Preset names a project file key (spec 9.8: "project") whose value the
// command uses when it is not given: the positional ID of describe, which
// becomes optional, or the list filter flag or create field flag of that
// name. A flag (or the ID) always wins, then PAPIERKRAM_<KEY>, then the
// project file.
type Entry struct {
	Resource, Action, OperationID, Method, Path string
	Class                                       Class
	Credits                                     int
	Filters                                     []Filter
	Columns                                     output.Columns
	Short                                       string
	Examples                                    []string
	Custom                                      bool
	Preset                                      string
}

// Resource is a top-level command noun with its German aliases (spec 7.1).
// Noun is the German singular shown in confirmation prompts ("Rechnung").
type Resource struct {
	Name    string
	Aliases []string
	Short   string
	Noun    string
}

// Resources returns all resources in the order of spec 7.1.
func Resources() []Resource {
	out := make([]Resource, len(resources))
	for i, r := range resources {
		r.Aliases = slices.Clone(r.Aliases)
		out[i] = r
	}
	return out
}

// Entries returns all registry entries in table order.
func Entries() []Entry {
	out := make([]Entry, len(table))
	for i, e := range table {
		out[i] = e.clone()
	}
	return out
}

// Lookup finds the entry of a command by resource name (not an alias) and
// action ("" for info).
func Lookup(resource, action string) (Entry, bool) {
	i, ok := theIndex().byCommand[commandKey(resource, action)]
	if !ok {
		return Entry{}, false
	}
	return table[i].clone(), true
}

// ByOperationID finds the entry of a spec operation.
func ByOperationID(id string) (Entry, bool) {
	i, ok := theIndex().byOperation[id]
	if !ok {
		return Entry{}, false
	}
	return table[i].clone(), true
}

// creditsByCollection is the cost of a POST that creates an object in the
// collection named by the last path segment (spec 4.3); documents are the
// documents of a voucher.
var creditsByCollection = map[string]int{
	"invoices":          50,
	"estimates":         50,
	"vouchers":          50,
	"companies":         10,
	"persons":           10,
	"documents":         10,
	"propositions":      10,
	"projects":          10,
	"custom_attributes": 10,
	"tasks":             10,
	"time_entries":      10,
}

// ExpectedCredits is the credit cost of a request (spec 4.3): 50 for
// creating an invoice, estimate or voucher, 10 for creating other objects,
// 1 for everything else, including actions such as archive. path is a path
// template or a concrete path (query string and trailing slash ignored); a
// create is a POST whose last segment is a collection.
func ExpectedCredits(method, path string) int {
	if !strings.EqualFold(method, "POST") {
		return 1
	}
	path, _, _ = strings.Cut(path, "?")
	path = strings.TrimRight(path, "/")
	last := path[strings.LastIndexByte(path, '/')+1:]
	if c, ok := creditsByCollection[last]; ok {
		return c
	}
	return 1
}

func (e Entry) clone() Entry {
	e.Filters = slices.Clone(e.Filters)
	e.Examples = slices.Clone(e.Examples)
	e.Columns.Table = slices.Clone(e.Columns.Table)
	e.Columns.Wide = slices.Clone(e.Columns.Wide)
	return e
}

func commandKey(resource, action string) string {
	return resource + "\x00" + action
}

// index holds the positions of the table entries by command and by
// operation.
type index struct {
	byCommand, byOperation map[string]int
}

// theIndex is built once. newIndex only fails for a defective table, which
// TestNewIndex rules out.
var theIndex = sync.OnceValue(func() *index {
	idx, err := newIndex(resources, table)
	if err != nil {
		panic(err)
	}
	return idx
})

// newIndex checks the resources and entries and indexes the entries: every
// entry needs an operation and a known resource; commands and operations
// must be unique.
func newIndex(rs []Resource, es []Entry) (*index, error) {
	if err := checkResources(rs); err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, r := range rs {
		names[r.Name] = true
	}
	idx := &index{byCommand: map[string]int{}, byOperation: map[string]int{}}
	for i, e := range es {
		cmd := strings.TrimSpace(e.Resource + " " + e.Action)
		if !names[e.Resource] {
			return nil, fmt.Errorf("registry: %s: unknown resource %q", cmd, e.Resource)
		}
		if e.OperationID == "" {
			return nil, fmt.Errorf("registry: %s: no operation", cmd)
		}
		key := commandKey(e.Resource, e.Action)
		if _, dup := idx.byCommand[key]; dup {
			return nil, fmt.Errorf("registry: duplicate command %q", cmd)
		}
		if _, dup := idx.byOperation[e.OperationID]; dup {
			return nil, fmt.Errorf("registry: duplicate operation %s", e.OperationID)
		}
		idx.byCommand[key] = i
		idx.byOperation[e.OperationID] = i
	}
	return idx, nil
}

// reservedNames are top-level commands of their own (meta commands, cobra
// built-ins); no resource name or alias may shadow them.
var reservedNames = []string{"help", "completion", "context", "auth", "version", "api", "commands", "schema", "local", "gen-docs"}

// checkResources requires unique names and aliases: no alias may repeat or
// equal any resource name, and no name or alias may be reserved.
func checkResources(rs []Resource) error {
	names := map[string]bool{}
	for _, r := range rs {
		for _, n := range append([]string{r.Name}, r.Aliases...) {
			if slices.Contains(reservedNames, n) {
				return fmt.Errorf("registry: %q (resource %s) is a reserved command name", n, r.Name)
			}
		}
		if names[r.Name] {
			return fmt.Errorf("registry: duplicate resource %q", r.Name)
		}
		names[r.Name] = true
	}
	aliases := map[string]string{}
	for _, r := range rs {
		for _, a := range r.Aliases {
			if names[a] {
				return fmt.Errorf("registry: alias %q of %s is a resource name", a, r.Name)
			}
			if other, dup := aliases[a]; dup {
				return fmt.Errorf("registry: alias %q of %s already used by %s", a, r.Name, other)
			}
			aliases[a] = r.Name
		}
	}
	return nil
}
