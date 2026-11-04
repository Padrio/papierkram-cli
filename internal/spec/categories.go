package spec

import (
	"slices"
	"sync"
)

// VoucherCategories returns the expense categories of a voucher line item
// (spec 4.4, 6.2): the enum of line_items[].category of the CreateVoucher
// request in the patched spec, in spec order, without null and duplicates
// (the overlay already removes them). Shell completion offers these values.
// The slice is a copy.
func VoucherCategories() []string {
	return slices.Clone(voucherCategories())
}

// voucherCategories reads the enum once. A spec without it yields nil,
// which TestVoucherCategories rules out.
var voucherCategories = sync.OnceValue(func() []string {
	req := RequestSchema("CreateVoucher")
	if req == nil || req.Properties["line_items"] == nil {
		return nil
	}
	items := itemSchema(req.Properties["line_items"].Value)
	if items == nil || items.Properties["category"] == nil {
		return nil
	}
	var out []string
	for _, e := range items.Properties["category"].Value.Enum {
		if v, ok := e.(string); ok && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
})
