package spec

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// vatField is the property name that gets VAT normalization wherever it
// appears (line items and propositions, spec 6.2).
const vatField = "vat_rate"

var vatRe = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

const vatHint = "erlaubt: Prozent (19, 19%) oder Dezimalbruch (0,19 bzw. 0.19) von 0 bis 100 %"

// NormalizeVAT turns a VAT rate in any accepted form into the decimal
// fraction the API expects (spec 6.2): 19, "19", "19%", "0,19", "0.19" and
// the number 19 all yield 0.19; 7 yields 0.07, 0 yields 0. Values above 1 and
// values with a % sign are percentages and are divided by 100. The result is
// rounded to four places. A bare 1 (1 % or 100 %?) is ambiguous; it, negative
// values, values above 100 and anything unparsable are a validation error.
func NormalizeVAT(v any) (float64, error) {
	var text string
	switch x := v.(type) {
	case string:
		text = x
	case json.Number:
		text = string(x)
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return 0, vatError(v)
		}
		return scaleVAT(x, false, v)
	case float32:
		return NormalizeVAT(float64(x))
	case int:
		return scaleVAT(float64(x), false, v)
	case int64:
		return scaleVAT(float64(x), false, v)
	case int32:
		return scaleVAT(float64(x), false, v)
	default:
		return 0, vatError(v)
	}
	s := strings.TrimSpace(text)
	s, percent := strings.CutSuffix(s, "%")
	s = strings.Replace(strings.TrimSpace(s), ",", ".", 1)
	if !vatRe.MatchString(s) {
		return 0, vatError(v)
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, vatError(v)
	}
	return scaleVAT(f, percent, v)
}

func scaleVAT(f float64, percent bool, orig any) (float64, error) {
	switch {
	case f < 0 || f > 100:
		return 0, vatError(orig)
	case f == 1 && !percent:
		return 0, clierr.New(clierr.CodeValidation, "vat_rate 1 ist mehrdeutig (1 % oder 100 %?)", "0.01 bzw. 1% oder 100% angeben")
	case percent || f > 1:
		f /= 100
	}
	return math.Round(f*10000) / 10000, nil
}

func vatError(v any) *clierr.Error {
	return clierr.New(clierr.CodeValidation, "ungültiger Steuersatz "+showValue(v), vatHint)
}

// FormatVAT renders a normalized VAT rate as the API's string form, e.g.
// 0.19 → "0.19", 0 → "0".
func FormatVAT(f float64) string {
	if f == 0 {
		return "0"
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}
