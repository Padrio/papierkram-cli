package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/registry"
)

// Name resolution in the flags of the builder commands (spec v0.2 §4):
// the ID flags of list filters, path parameters and create/update fields
// whose name the registry maps to a resource (registry.NameRefs) carry the
// annotation flagResolves (commands --json: resolves) and take a name instead of the ID. The runners
// check the values with the other flags (checkNameFlags, exit 2), read
// IDs with flagID (a name is 0 until resolved), resolve the names after
// the read-only gate (resolveNames, nameResolver) and before the body or
// query is built, and write the IDs back into the flags, so every later
// step sees IDs only. Commands with their own handler (time log, time
// start, local init, context create) take no names; positional <id>
// arguments and papierkram api neither.

// markResolves marks the ID flag name for name resolution when the
// registry maps it to a resource, and says so in its help: "ID" becomes
// "ID oder Name" ("Firmen-ID oder Name"), a help without "ID" gets
// "; ID oder Name".
func markResolves(fs *pflag.FlagSet, name string) {
	ref, ok := registry.NameRefOf(name)
	if !ok {
		return
	}
	annotate(fs, name, flagResolves, ref.Resource)
	f := fs.Lookup(name)
	if i := strings.Index(f.Usage, "ID"); i >= 0 {
		f.Usage = f.Usage[:i+len("ID")] + " oder Name" + f.Usage[i+len("ID"):]
	} else {
		f.Usage += "; ID oder Name"
	}
}

// resolves reports a flag with name resolution.
func resolves(f *pflag.Flag) bool { return len(f.Annotations[flagResolves]) == 1 }

// flagValues are the values given to f: one per occurrence of a text
// list (--team-member), else the one value.
func flagValues(f *pflag.Flag) []string {
	if sv, ok := f.Value.(pflag.SliceValue); ok {
		return sv.GetSlice()
	}
	return []string{f.Value.String()}
}

// flagID reads the ID given to the ID flag name (rawValue{typ:"id"}). A
// flag with name resolution takes an integer with optional sign and
// surrounding whitespace (isIDValue); a name there is 0 until
// resolveNames has replaced it. A flag without name resolution parses
// like the integer flag it was before (strconv.ParseInt base 0, as
// pflag's Int64), anything else is exit 2 with pflag's German text.
// Negative IDs are returned: the callers reject them with their own
// message, as before.
func flagID(c *cobra.Command, name string) (int64, error) {
	f := c.Flags().Lookup(name)
	raw := f.Value.String()
	s, base := raw, 0
	if resolves(f) {
		if !isIDValue(raw) {
			return 0, nil // a name
		}
		s, base = strings.TrimSpace(raw), 10
	}
	id, err := strconv.ParseInt(s, base, 64)
	if err != nil {
		return 0, clierr.New(clierr.CodeUsage,
			fmt.Sprintf("ungültiger Wert %q für --%s: %s", raw, name, germanCause(err.Error(), strconv.Quote(raw), "int64")), usageHint(c))
	}
	return id, nil
}

// checkNameFlags runs the checks of the name flags that need only the
// flags (exit 2, before anything is resolved or sent): a value that is
// empty after trimming (also an element of a list), and a name of a flag
// whose list hangs on a parent (--contact-person: the persons of
// --customer) without that parent as name or ID ≥ 1.
func checkNameFlags(c *cobra.Command) error {
	var err error
	c.Flags().Visit(func(f *pflag.Flag) {
		if err != nil || !resolves(f) {
			return
		}
		ref, _ := registry.NameRefOf(f.Name)
		for _, v := range flagValues(f) {
			if !isIDValue(v) && nameKey(v) == "" {
				err = clierr.New(clierr.CodeUsage, fmt.Sprintf("--%s braucht eine ID oder einen Namen", f.Name), usageHint(c))
				return
			}
			if ref.Parent != "" && !isIDValue(v) {
				if err = checkParent(c, f.Name, ref); err != nil {
					return
				}
			}
		}
	})
	return err
}

// checkParent checks the parent flag of a name of the flag name (ref):
// given, and a name or an ID ≥ 1 (the list of the parent object).
func checkParent(c *cobra.Command, name string, ref registry.NameRef) error {
	hint := "sonst die ID der " + ref.Label + " angeben"
	p := c.Flags().Lookup(ref.Parent)
	if p == nil || !p.Changed {
		return clierr.New(clierr.CodeUsage, fmt.Sprintf("Name für --%s braucht --%s (ID oder Name)", name, ref.Parent), hint)
	}
	raw := p.Value.String()
	if !isIDValue(raw) {
		return nil
	}
	if id, err := flagID(c, ref.Parent); err != nil || id < 1 {
		return clierr.New(clierr.CodeUsage,
			fmt.Sprintf("ungültiger Wert %q für --%s: Name für --%s braucht eine ID ≥ 1 oder einen Namen", raw, ref.Parent, name), hint)
	}
	return nil
}

// resolveNames resolves every name given to a flag with name resolution,
// first the flags without parent, then those with one (their parent is an
// ID by then), each in flag name order, and writes the IDs back into the
// flags (also the trimmed form of an ID with whitespace, " 12 " → "12",
// for the field flags whose coercion does not trim). It runs after the
// read-only gate with the client of the command and returns the
// resolutions for --dry-run; the first failing one ends it with its
// error (nothing is sent).
func (a *App) resolveNames(c *cobra.Command, client *api.Client, r *config.Resolved) ([]Resolution, error) {
	var flags []*pflag.Flag
	c.Flags().Visit(func(f *pflag.Flag) {
		if resolves(f) {
			flags = append(flags, f)
		}
	})
	var out []Resolution
	var n *nameResolver
	for _, withParent := range []bool{false, true} {
		for _, f := range flags {
			ref, _ := registry.NameRefOf(f.Name)
			if (ref.Parent != "") != withParent {
				continue
			}
			vals, changed := flagValues(f), false
			for i, v := range vals {
				if isIDValue(v) {
					if t := strings.TrimSpace(v); t != v {
						vals[i], changed = t, true
					}
					continue
				}
				var parent int64
				if ref.Parent != "" {
					var err error
					if parent, err = flagID(c, ref.Parent); err != nil {
						return out, err
					}
				}
				if n == nil {
					n = a.newNameResolver(c, client, r)
				}
				res, err := n.resolve(ref, v, parent)
				if err != nil {
					return out, err
				}
				out = append(out, res)
				vals[i], changed = strconv.FormatInt(res.ID, 10), true
			}
			if changed {
				if err := setFlagValues(c.Flags(), f, vals); err != nil {
					return out, err
				}
			}
		}
	}
	return out, nil
}

// setFlagValues replaces the values of f: the whole list of a text list,
// else the one value.
func setFlagValues(fs *pflag.FlagSet, f *pflag.Flag, vals []string) error {
	if sv, ok := f.Value.(pflag.SliceValue); ok {
		return sv.Replace(vals)
	}
	return fs.Set(f.Name, vals[0])
}

// patchPathParams sets the values of the path flags given (params of
// pathValues, where a name was 0) anew after resolveNames.
func patchPathParams(c *cobra.Command, e registry.Entry, params []int64) error {
	for i, p := range pathParamsOf(e) {
		if p.flag == "" || !c.Flags().Changed(p.flag) {
			continue
		}
		id, err := flagID(c, p.flag)
		if err != nil {
			return err
		}
		params[i] = id
	}
	return nil
}

// resolvedCredits is the sum of the credits of the resolutions.
func resolvedCredits(res []Resolution) int {
	n := 0
	for _, r := range res {
		n += r.Credits
	}
	return n
}
