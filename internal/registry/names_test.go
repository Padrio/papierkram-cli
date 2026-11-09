package registry

import (
	"slices"
	"strings"
	"testing"
)

// Every flag with name resolution names a resource with a list request;
// a parent is itself a flag of the table; flags are unique.
func TestNameRefsMatchRegistry(t *testing.T) {
	refs := NameRefs()
	if len(refs) == 0 {
		t.Fatal("keine Zuordnungen")
	}
	flags := map[string]bool{}
	for _, ref := range refs {
		if flags[ref.Flag] {
			t.Errorf("Flag %q doppelt", ref.Flag)
		}
		flags[ref.Flag] = true
		if _, ok := Lookup(ref.Resource, "list"); !ok {
			t.Errorf("%s: Ressource %q ohne list", ref.Flag, ref.Resource)
		}
		if ref.Label == "" {
			t.Errorf("%s: ohne Label", ref.Flag)
		}
	}
	for _, ref := range refs {
		if ref.Parent != "" && !flags[ref.Parent] {
			t.Errorf("%s: Parent %q ist kein Flag der Tabelle", ref.Flag, ref.Parent)
		}
		list, _ := Lookup(ref.Resource, "list")
		hasPath := strings.Contains(list.Path, "{")
		switch {
		case ref.ParentFilter != "" && ref.Parent == "":
			t.Errorf("%s: ParentFilter ohne Parent", ref.Flag)
		case ref.ParentFilter != "" && hasPath:
			t.Errorf("%s: ParentFilter bei Liste mit Pfadparameter", ref.Flag)
		case ref.ParentFilter == "" && hasPath != (ref.Parent != ""):
			t.Errorf("%s: Parent %q passt nicht zum Pfad %s", ref.Flag, ref.Parent, list.Path)
		}
		if ref.ParentFilter != "" && !slices.ContainsFunc(list.Filters, func(f Filter) bool { return f.Flag == ref.ParentFilter && f.Kind == "id" }) {
			t.Errorf("%s: ParentFilter %q ist kein ID-Filter von %s list", ref.Flag, ref.ParentFilter, ref.Resource)
		}
	}
	if got, ok := NameRefOf("customer"); !ok || got.Resource != "company" || got.Label != "Kunde" || got.Parent != "" {
		t.Errorf("NameRefOf(customer) = %+v, %v", got, ok)
	}
	if got, ok := NameRefOf("contact-person"); !ok || got.Resource != "person" || got.Parent != "customer" {
		t.Errorf("NameRefOf(contact-person) = %+v, %v", got, ok)
	}
	if got, ok := NameRefOf("customer-project"); !ok || got.Resource != "project" || got.Parent != "customer" || got.ParentFilter != "company" {
		t.Errorf("NameRefOf(customer-project) = %+v, %v", got, ok)
	}
	if _, ok := NameRefOf("invoice"); ok {
		t.Error("NameRefOf(invoice) gefunden")
	}
	// NameRefs returns a copy
	refs[0].Resource = "x"
	if NameRefs()[0].Resource == "x" {
		t.Error("NameRefs gibt keine Kopie zurück")
	}
}
