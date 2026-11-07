package e2eguard

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Padrio/papierkram-cli/internal/spec"
)

func TestJudgeFollowUp(t *testing.T) {
	cases := []struct {
		name, resource string
		target         int64
		answer         string
		want           FollowUp
		found          bool
	}{
		{"same id", "voucher", 7, `{"id":7,"type":"expense_voucher"}`, FollowUp{}, false},
		{"empty body", "voucher", 7, ``, FollowUp{}, false},
		{"no object", "voucher", 7, `[{"id":8}]`, FollowUp{}, false},
		{"no id", "voucher", 7, `{"type":"expense_voucher"}`, FollowUp{}, false},
		{"id not a number", "voucher", 7, `{"id":"x8","type":"expense_voucher"}`, FollowUp{}, false},
		{"id zero", "voucher", 7, `{"id":0,"type":"expense_voucher"}`, FollowUp{}, false},
		{"target unknown", "voucher", 0, `{"id":8,"type":"expense_voucher"}`, FollowUp{}, false},
		{"voucher", "voucher", 7, `{"id":8,"type":"expense_voucher","name":"Storno"}`,
			FollowUp{ID: 8, Name: "Storno", Type: "expense_voucher", Track: true}, true},
		{"invoice without name", "invoice", 7, `{"id":12345678901,"type":"invoice"}`,
			FollowUp{ID: 12345678901, Type: "invoice", Track: true}, true},
		{"estimate", "estimate", 7, `{"id":9,"type":"estimate","name":5}`,
			FollowUp{ID: 9, Type: "estimate", Track: true}, true},
		{"other type", "voucher", 7, `{"id":8,"type":"invoice"}`,
			FollowUp{ID: 8, Type: "invoice"}, true},
		{"type missing", "invoice", 7, `{"id":8}`, FollowUp{ID: 8}, true},
		{"type not a string", "invoice", 7, `{"id":8,"type":1}`, FollowUp{ID: 8}, true},
		{"resource without type", "company", 7, `{"id":8,"type":""}`, FollowUp{ID: 8}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, found := JudgeFollowUp(c.resource, c.target, []byte(c.answer))
			if got != c.want || found != c.found {
				t.Errorf("JudgeFollowUp(%s, %d, %s) = %+v, %t; want %+v, %t", c.resource, c.target, c.answer, got, found, c.want, c.found)
			}
		})
	}
}

// The type values match the enum of type in the answers of describe,
// cancel and reverse of the embedded spec.
func TestFollowUpTypesMatchSpec(t *testing.T) {
	ops := map[string][]string{
		"voucher":  {"GetVoucher", "CancelVoucher", "CancelVoucherWithReverseEntry"},
		"invoice":  {"GetInvoice", "CancelInvoice"},
		"estimate": {"GetEstimate", "CancelEstimate"},
	}
	if len(followUpTypes) != len(ops) {
		t.Errorf("followUpTypes has %d resources, want %d", len(followUpTypes), len(ops))
	}
	for res, ids := range ops {
		for _, id := range ids {
			typ := typeProperty(spec.ResponseSchema(id))
			if typ == nil {
				t.Errorf("%s: no type in the response schema", id)
				continue
			}
			enum := typ.Enum
			if len(enum) != 1 || enum[0] != followUpTypes[res] {
				t.Errorf("%s: type enum %v, followUpTypes[%s] = %q", id, enum, res, followUpTypes[res])
			}
		}
	}
}

// typeProperty is the schema of the property type of s, also inside allOf;
// nil when there is none.
func typeProperty(s *openapi3.Schema) *openapi3.Schema {
	if s == nil {
		return nil
	}
	if p := s.Properties["type"]; p != nil && p.Value != nil {
		return p.Value
	}
	for _, a := range s.AllOf {
		if a != nil {
			if p := typeProperty(a.Value); p != nil {
				return p
			}
		}
	}
	return nil
}
