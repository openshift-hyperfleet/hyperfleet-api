package registry

import "fmt"

var descriptors = make(map[string]EntityDescriptor)

// Register adds a descriptor to the global registry. Panics on empty Kind or duplicate Kind.
func Register(d EntityDescriptor) {
	if d.Kind == "" {
		panic("entity kind cannot be empty")
	}
	if d.Plural == "" {
		panic(fmt.Sprintf("entity kind %q has empty plural", d.Kind))
	}
	if _, exists := descriptors[d.Kind]; exists {
		panic(fmt.Sprintf("entity kind %q already registered", d.Kind))
	}
	descriptors[d.Kind] = d
}

// Get returns a descriptor by Kind, or (zero, false) if not found.
func Get(entityKind string) (EntityDescriptor, bool) {
	d, ok := descriptors[entityKind]
	return d, ok
}

// MustGet returns a descriptor by Kind. Panics if not found.
func MustGet(entityKind string) EntityDescriptor {
	d, ok := Get(entityKind)
	if !ok {
		panic(fmt.Sprintf("entity kind %q not registered", entityKind))
	}
	return d
}

// All returns a snapshot of all registered descriptors.
func All() []EntityDescriptor {
	result := make([]EntityDescriptor, 0, len(descriptors))
	for _, d := range descriptors {
		result = append(result, d)
	}
	return result
}

// ChildrenOf returns descriptors whose ParentKind matches the given kind.
func ChildrenOf(parentKind string) []EntityDescriptor {
	var children []EntityDescriptor
	for _, d := range descriptors {
		if d.ParentKind == parentKind {
			children = append(children, d)
		}
	}
	return children
}

// Validate checks registry integrity. Panics on:
//   - empty Kind or Plural on any descriptor
//   - any ParentKind that references an unregistered kind
//   - NameMinLen > NameMaxLen (when NameMaxLen is set)
//   - duplicate Plural values across descriptors
func Validate() {
	plurals := make(map[string]string, len(descriptors))

	for _, d := range descriptors {
		if d.Kind == "" {
			panic("entity kind cannot be empty")
		}
		if d.Plural == "" {
			panic(fmt.Sprintf("entity kind %q has empty plural", d.Kind))
		}

		if d.ParentKind != "" {
			if _, ok := descriptors[d.ParentKind]; !ok {
				panic(fmt.Sprintf(
					"entity kind %q references unregistered parent kind %q",
					d.Kind, d.ParentKind,
				))
			}
		}

		if d.NameMaxLen > 0 && d.NameMinLen > d.NameMaxLen {
			panic(fmt.Sprintf(
				"entity kind %q has NameMinLen (%d) > NameMaxLen (%d)",
				d.Kind, d.NameMinLen, d.NameMaxLen,
			))
		}

		if existing, ok := plurals[d.Plural]; ok {
			panic(fmt.Sprintf(
				"duplicate plural %q: registered by both %q and %q",
				d.Plural, existing, d.Kind,
			))
		}
		plurals[d.Plural] = d.Kind
	}
}

// Reset clears all registrations. Only for use in tests.
func Reset() {
	descriptors = make(map[string]EntityDescriptor)
}
