package services

// ListArguments are arguments relevant for listing objects.
// This struct is common to all service List funcs in this package
type ListArguments struct {
	Search      string
	RefType     string
	RefTargetID string
	Preloads    []string
	Order       []string
	Fields      []string
	Size        int64
	Page        int64
}

func NewListArguments() *ListArguments {
	return &ListArguments{
		Page: 1,
		Size: 20,
	}
}
