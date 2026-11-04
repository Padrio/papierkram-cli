package spec

import (
	"slices"
	"testing"
)

// The expense categories for completion (spec 6.2): 94 unique values from
// the patched spec, the same for create and update, no null.
func TestVoucherCategories(t *testing.T) {
	got := VoucherCategories()
	if len(got) != 94 {
		t.Fatalf("%d categories, want 94", len(got))
	}
	seen := map[string]bool{}
	for _, c := range got {
		if c == "" || seen[c] {
			t.Errorf("empty or duplicate category %q", c)
		}
		seen[c] = true
	}
	for _, c := range []string{"Bürobedarf", "Bewirtungskosten", "Betriebsbedarf"} {
		if !seen[c] {
			t.Errorf("%q missing", c)
		}
	}
	update := RequestSchema("UpdateVoucher").Properties["line_items"].Value.Items.Value.Properties["category"].Value
	var want []string
	for _, e := range update.Enum {
		want = append(want, e.(string))
	}
	if !slices.Equal(got, want) {
		t.Errorf("categories differ from the UpdateVoucher enum")
	}
	// callers get a copy
	got[0] = "geändert"
	if VoucherCategories()[0] == "geändert" {
		t.Error("VoucherCategories shares memory with the spec")
	}
}
