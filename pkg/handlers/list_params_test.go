package handlers

import (
	"net/url"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

func Test_parseListParams(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name     string
		query    string
		expected *services.ListArguments
		errors   []expectedDetail
	}{
		// Defaults
		{
			name:  "defaults",
			query: "",
			expected: &services.ListArguments{
				Page:  1,
				Size:  20,
				Order: []string{"created_time desc"},
			},
		},
		// Custom values
		{
			name:  "custom page and size",
			query: "?page=2&size=50",
			expected: &services.ListArguments{
				Page:  2,
				Size:  50,
				Order: []string{"created_time desc"},
			},
		},
		{
			name:  "search",
			query: "?search=status%20%3D%20active",
			expected: &services.ListArguments{
				Page:   1,
				Size:   20,
				Search: "status = active",
				Order:  []string{"created_time desc"},
			},
		},
		// Order
		{
			name:  "custom order",
			query: "?order=name%20asc,created_time%20desc",
			expected: &services.ListArguments{
				Page:  1,
				Size:  20,
				Order: []string{"name asc", "created_time desc"},
			},
		},
		{
			name:  "order preserves raw value for downstream validation",
			query: "?order=name%20%20%20asc",
			expected: &services.ListArguments{
				Page:  1,
				Size:  20,
				Order: []string{"name   asc"},
			},
		},
		// Fields
		{
			name:  "fields auto-includes id",
			query: "?fields=name,kind",
			expected: &services.ListArguments{
				Page:   1,
				Size:   20,
				Order:  []string{"created_time desc"},
				Fields: []string{"name", "kind", "id"},
			},
		},
		{
			name:  "fields preserves existing id",
			query: "?fields=id,name",
			expected: &services.ListArguments{
				Page:   1,
				Size:   20,
				Order:  []string{"created_time desc"},
				Fields: []string{"id", "name"},
			},
		},
		// Ref pairing
		{
			name:  "paired ref parameters",
			query: "?ref_type=dep&ref_target_id=target-1",
			expected: &services.ListArguments{
				Page:        1,
				Size:        20,
				Order:       []string{"created_time desc"},
				RefType:     "dep",
				RefTargetID: "target-1",
			},
		},
		{
			name:  "ref_type without target_id",
			query: "?ref_type=dep",
			errors: []expectedDetail{
				{message: "ref_type and ref_target_id must be provided together"},
			},
		},
		{
			name:  "ref_target_id without ref_type",
			query: "?ref_target_id=some-id",
			errors: []expectedDetail{
				{message: "ref_type and ref_target_id must be provided together"},
			},
		},
		// Format errors
		{
			name:  "non-numeric page",
			query: "?page=abc",
			errors: []expectedDetail{
				{field: "page", message: "must be a valid integer"},
			},
		},
		{
			name:  "non-numeric size",
			query: "?size=xyz",
			errors: []expectedDetail{
				{field: "size", message: "must be a valid integer"},
			},
		},
		{
			name:  "both page and size non-numeric",
			query: "?page=abc&size=xyz",
			errors: []expectedDetail{
				{field: "page", message: "must be a valid integer"},
				{field: "size", message: "must be a valid integer"},
			},
		},
		// Range errors
		{
			name:  "negative page",
			query: "?page=-1",
			errors: []expectedDetail{
				{field: "page", message: "page must be at least 1"},
			},
		},
		{
			name:  "zero page",
			query: "?page=0",
			errors: []expectedDetail{
				{field: "page", message: "page must be at least 1"},
			},
		},
		{
			name:  "negative size",
			query: "?size=-1",
			errors: []expectedDetail{
				{field: "size", message: "size must be at least 1"},
			},
		},
		{
			name:  "zero size",
			query: "?size=0",
			errors: []expectedDetail{
				{field: "size", message: "size must be at least 1"},
			},
		},
		{
			name:  "size above maximum",
			query: "?size=101",
			errors: []expectedDetail{
				{field: "size", message: "size must be at most 100"},
			},
		},
		{
			name:  "very large size",
			query: "?size=999999",
			errors: []expectedDetail{
				{field: "size", message: "size must be at most 100"},
			},
		},
		{
			name:  "multiple range errors at once",
			query: "?page=0&size=999",
			errors: []expectedDetail{
				{field: "page", message: "page must be at least 1"},
				{field: "size", message: "size must be at most 100"},
			},
		},
		// Edge cases
		{
			name:  "search with leading and trailing spaces",
			query: "?search=%20%20status%20%3D%20active%20%20",
			expected: &services.ListArguments{
				Page:   1,
				Size:   20,
				Search: "status = active",
				Order:  []string{"created_time desc"},
			},
		},
		{
			name:  "empty order falls back to default",
			query: "?order=",
			expected: &services.ListArguments{
				Page:  1,
				Size:  20,
				Order: []string{"created_time desc"},
			},
		},
		{
			name:  "whitespace-only order falls back to default",
			query: "?order=%20%20%20",
			expected: &services.ListArguments{
				Page:  1,
				Size:  20,
				Order: []string{"created_time desc"},
			},
		},
		{
			name:  "pageSize is ignored",
			query: "?pageSize=50",
			expected: &services.ListArguments{
				Page:  1,
				Size:  20,
				Order: []string{"created_time desc"},
			},
		},
		{
			name:  "valid boundary page=1 size=1",
			query: "?page=1&size=1",
			expected: &services.ListArguments{
				Page:  1,
				Size:  1,
				Order: []string{"created_time desc"},
			},
		},
		{
			name:  "valid boundary page=1 size=100",
			query: "?page=1&size=100",
			expected: &services.ListArguments{
				Page:  1,
				Size:  100,
				Order: []string{"created_time desc"},
			},
		},
		{
			name:  "valid large page",
			query: "?page=999&size=50",
			expected: &services.ListArguments{
				Page:  999,
				Size:  50,
				Order: []string{"created_time desc"},
			},
		},
		{
			name:  "page above maximum",
			query: "?page=100001",
			errors: []expectedDetail{
				{field: "page", message: "page must be at most 100000"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			u, _ := url.Parse("/resources" + tt.query)
			result, err := parseListParams(u.Query())

			if len(tt.errors) > 0 {
				Expect(err).ToNot(BeNil())
				Expect(err.HTTPCode).To(Equal(400))
				Expect(result).To(BeNil())
				for _, exp := range tt.errors {
					matcher := HaveField("Message", exp.message)
					if exp.field != "" {
						matcher = And(HaveField("Field", exp.field), matcher)
					}
					Expect(err.Details).To(ContainElement(matcher))
				}
			} else {
				Expect(err).To(BeNil())
				Expect(result).To(Equal(tt.expected))
			}
		})
	}
}

type expectedDetail struct {
	field   string
	message string
}

func TestNormalizeList(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{"nil input", nil, nil},
		{"empty input", []string{}, nil},
		{"single value", []string{"name"}, []string{"name"}},
		{"comma-separated", []string{"name,kind,id"}, []string{"name", "kind", "id"}},
		{"repeated params", []string{"name", "kind", "id"}, []string{"name", "kind", "id"}},
		{"mixed", []string{"name,kind", "id"}, []string{"name", "kind", "id"}},
		{"with whitespace", []string{" name , kind , id "}, []string{"name", "kind", "id"}},
		{"trailing comma", []string{"name,"}, []string{"name"}},
		{"only commas", []string{",,,"}, nil},
		{"empty strings", []string{"", ""}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(normalizeList(tt.input)).To(Equal(tt.expected))
		})
	}
}
