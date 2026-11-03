package spec

import (
	"slices"
	"testing"
)

func TestSuggest(t *testing.T) {
	fields := []string{"name", "notes", "description", "document_date"}
	tests := []struct {
		in         string
		candidates []string
		n          int
		want       []string
	}{
		{"note", fields, 1, []string{"notes"}},
		{"NOTE", fields, 1, []string{"notes"}},
		{"address", []string{"recipient", "subject", "body"}, 3, nil},
		{"non_eu", []string{"domestic", "eu", "foreign"}, 3, nil},
		{"xyz", []string{"domestic", "eu", "foreign"}, 3, nil},
		{"domestc", []string{"domestic", "eu", "foreign"}, 3, []string{"domestic"}},
		// Ties keep the candidate order.
		{"ab", []string{"ac", "xb", "ab", "abc"}, 3, []string{"ab", "ac", "xb"}},
		{"ab", []string{"ac", "xb"}, 0, nil},
		{"Bürobedraf", categories(t), 3, []string{"Bürobedarf"}},
		{"Burobedarf", categories(t), 1, []string{"Bürobedarf"}},
	}
	for _, tc := range tests {
		if got := Suggest(tc.in, tc.candidates, tc.n); !slices.Equal(got, tc.want) {
			t.Errorf("Suggest(%q, n=%d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
}

func categories(t *testing.T) []string {
	t.Helper()
	s := RequestSchema("CreateVoucher").Properties["line_items"].Value.Items.Value.Properties["category"].Value
	var out []string
	for _, e := range s.Enum {
		out = append(out, e.(string))
	}
	if len(out) != 94 {
		t.Fatalf("got %d categories, want 94", len(out))
	}
	return out
}

func TestNearest(t *testing.T) {
	tests := []struct {
		in         string
		candidates []string
		n          int
		want       []string
	}{
		// No distance limit: nonsense still gets the closest values.
		{"xyz", []string{"domestic", "eu", "foreign"}, 2, []string{"eu", "foreign"}},
		{"ab", []string{"ac", "xb", "ab", "ab"}, 5, []string{"ab", "ac", "xb"}},
		{"ab", []string{"ac"}, 0, nil},
		{"BÜRO", []string{"Büro", "Bar"}, 1, []string{"Büro"}},
	}
	for _, tc := range tests {
		if got := Nearest(tc.in, tc.candidates, tc.n); !slices.Equal(got, tc.want) {
			t.Errorf("Nearest(%q, n=%d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
}
