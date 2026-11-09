package registry

// NameRef is a flag that takes the ID of a resource with names: instead
// of the ID it takes a name, resolved through the list request of
// Resource (spec v0.2 §4). Label is the German noun of the flag in hints
// ("Kunde"); Parent is the flag whose ID the list needs (the contact
// person lists of a company), empty for lists without parent.
type NameRef struct {
	Flag, Resource, Label, Parent string
}

// nameRefs are the flags with name resolution, by flag name; a flag of
// the same name means the same resource in every command.
var nameRefs = []NameRef{
	{Flag: "customer", Resource: "company", Label: "Kunde"},
	{Flag: "creditor", Resource: "company", Label: "Lieferant"},
	{Flag: "company", Resource: "company", Label: "Firma"},
	{Flag: "project", Resource: "project", Label: "Projekt"},
	{Flag: "customer-project", Resource: "project", Label: "Projekt"},
	{Flag: "contact-person", Resource: "person", Label: "Kontaktperson", Parent: "customer"},
	{Flag: "task", Resource: "task", Label: "Aufgabe"},
	{Flag: "proposition", Resource: "proposition", Label: "Artikel"},
	{Flag: "default-proposition", Resource: "proposition", Label: "Artikel"},
	{Flag: "payment-term", Resource: "payment-term", Label: "Zahlungsbedingung"},
	{Flag: "user", Resource: "user", Label: "Benutzer"},
	{Flag: "team-member", Resource: "user", Label: "Benutzer"},
}

// NameRefs returns the flags with name resolution in table order.
func NameRefs() []NameRef {
	out := make([]NameRef, len(nameRefs))
	copy(out, nameRefs)
	return out
}

// NameRefOf returns the name resolution of the flag (without dashes).
func NameRefOf(flag string) (NameRef, bool) {
	for _, ref := range nameRefs {
		if ref.Flag == flag {
			return ref, true
		}
	}
	return NameRef{}, false
}
