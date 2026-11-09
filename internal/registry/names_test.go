package registry

import "testing"

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
	}
	if got, ok := NameRefOf("customer"); !ok || got.Resource != "company" || got.Label != "Kunde" || got.Parent != "" {
		t.Errorf("NameRefOf(customer) = %+v, %v", got, ok)
	}
	if got, ok := NameRefOf("contact-person"); !ok || got.Resource != "person" || got.Parent != "customer" {
		t.Errorf("NameRefOf(contact-person) = %+v, %v", got, ok)
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
