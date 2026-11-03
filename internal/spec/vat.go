package spec

import (
	"encoding/json"
	"fmt"
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
// values with a % sign are percentages and are divided by 100 (rounded to four
// places). Negative values, values above 100 and anything unparsable are a
// validation error.
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
	if f < 0 || f > 100 {
		return 0, vatError(orig)
	}
	if percent || f > 1 {
		return math.Round(f/100*10000) / 10000, nil
	}
	return f, nil
}

func vatError(v any) *clierr.Error {
	var shown string
	switch x := v.(type) {
	case string:
		shown = strconv.Quote(x)
	case nil:
		shown = "null"
	default:
		shown = fmt.Sprintf("%v", x)
	}
	return clierr.New(clierr.CodeValidation, "ungültiger Steuersatz "+shown, vatHint)
}

// FormatVAT renders a normalized VAT rate as the API's string form, e.g.
// 0.19 → "0.19", 0 → "0".
func FormatVAT(f float64) string {
	if f == 0 {
		return "0"
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}
