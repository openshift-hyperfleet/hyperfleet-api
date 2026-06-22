package services

import (
	"net/url"
	"reflect"
	"testing"

	. "github.com/onsi/gomega"
)

func TestNewListArguments_OrderBy(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name            string
		queryParams     url.Values
		expectedOrderBy []string
	}{
		{
			name:            "no orderBy - should use default created_time desc",
			queryParams:     url.Values{},
			expectedOrderBy: []string{"created_time desc"},
		},
		{
			name:            "orderBy with asc direction",
			queryParams:     url.Values{"orderBy": []string{"name asc"}},
			expectedOrderBy: []string{"name asc"},
		},
		{
			name:            "orderBy with desc direction",
			queryParams:     url.Values{"orderBy": []string{"name desc"}},
			expectedOrderBy: []string{"name desc"},
		},
		{
			name:            "orderBy without direction",
			queryParams:     url.Values{"orderBy": []string{"name"}},
			expectedOrderBy: []string{"name"},
		},
		{
			name:            "multiple orderBy fields",
			queryParams:     url.Values{"orderBy": []string{"name asc,created_time desc"}},
			expectedOrderBy: []string{"name asc", "created_time desc"},
		},
		{
			name:            "orderBy with spaces should be trimmed",
			queryParams:     url.Values{"orderBy": []string{" name asc "}},
			expectedOrderBy: []string{"name asc"},
		},
		{
			name:            "empty orderBy string - should use default",
			queryParams:     url.Values{"orderBy": []string{""}},
			expectedOrderBy: []string{"created_time desc"},
		},
		{
			name:            "orderBy without direction + order=asc applies direction",
			queryParams:     url.Values{"orderBy": []string{"name"}, "order": []string{"asc"}},
			expectedOrderBy: []string{"name asc"},
		},
		{
			name:            "orderBy without direction + order=desc applies direction",
			queryParams:     url.Values{"orderBy": []string{"name"}, "order": []string{"desc"}},
			expectedOrderBy: []string{"name desc"},
		},
		{
			name:            "orderBy with direction + order parameter - orderBy takes precedence",
			queryParams:     url.Values{"orderBy": []string{"name desc"}, "order": []string{"asc"}},
			expectedOrderBy: []string{"name desc"},
		},
		{
			name:            "multiple orderBy fields + order parameter",
			queryParams:     url.Values{"orderBy": []string{"name,created_time"}, "order": []string{"desc"}},
			expectedOrderBy: []string{"name desc", "created_time desc"},
		},
		{
			name:            "mixed orderBy (with and without direction) + order parameter",
			queryParams:     url.Values{"orderBy": []string{"name,created_time asc"}, "order": []string{"desc"}},
			expectedOrderBy: []string{"name desc", "created_time asc"},
		},
		{
			name:            "orderBy with leading/trailing spaces + order parameter - should trim",
			queryParams:     url.Values{"orderBy": []string{"name, status , created_time"}, "order": []string{"asc"}},
			expectedOrderBy: []string{"name asc", "status asc", "created_time asc"},
		},
		{
			name:            "orderBy with whitespace only - should use default",
			queryParams:     url.Values{"orderBy": []string{"   "}},
			expectedOrderBy: []string{"created_time desc"},
		},
		{
			name:            "orderBy with empty tokens - should filter out",
			queryParams:     url.Values{"orderBy": []string{"name,,created_time"}},
			expectedOrderBy: []string{"name", "created_time"},
		},
		{
			name:            "orderBy with empty tokens + order parameter - should filter and apply direction",
			queryParams:     url.Values{"orderBy": []string{"name, , status"}, "order": []string{"desc"}},
			expectedOrderBy: []string{"name desc", "status desc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			listArgs, err := NewListArguments(tt.queryParams)
			Expect(err).To(BeNil(), "Should not return error for valid orderBy parameters")
			Expect(listArgs.OrderBy).To(Equal(tt.expectedOrderBy),
				"OrderBy mismatch for test case: %s", tt.name)
		})
	}
}

func TestNewListArguments_DefaultValues(t *testing.T) {
	RegisterTestingT(t)

	listArgs, err := NewListArguments(url.Values{})

	Expect(err).To(BeNil(), "Should not return error for default values")
	Expect(listArgs.Page).To(Equal(1), "Default page should be 1")
	Expect(listArgs.Size).To(Equal(int64(20)), "Default size should be 20")
	Expect(listArgs.Search).To(Equal(""), "Default search should be empty")
	Expect(listArgs.OrderBy).To(Equal([]string{"created_time desc"}), "Default orderBy should be created_time desc")
}

func TestNewListArguments_PageSize(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		queryParams  url.Values
		name         string
		expectedPage int
		expectedSize int64
	}{
		{
			name:         "custom page and pageSize",
			queryParams:  url.Values{"page": []string{"2"}, "pageSize": []string{"50"}},
			expectedPage: 2,
			expectedSize: 50,
		},
		{
			name:         "custom page and size (legacy)",
			queryParams:  url.Values{"page": []string{"3"}, "size": []string{"25"}},
			expectedPage: 3,
			expectedSize: 25,
		},
		{
			name:         "pageSize takes precedence over size",
			queryParams:  url.Values{"pageSize": []string{"30"}, "size": []string{"60"}},
			expectedPage: 1,
			expectedSize: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			listArgs, err := NewListArguments(tt.queryParams)
			Expect(err).To(BeNil(), "Should not return error for valid parameters")
			Expect(listArgs.Page).To(Equal(tt.expectedPage), "Page mismatch")
			Expect(listArgs.Size).To(Equal(tt.expectedSize), "Size mismatch")
		})
	}
}

func TestNewListArguments_Search(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name           string
		queryParams    url.Values
		expectedSearch string
	}{
		{
			name:           "no search parameter",
			queryParams:    url.Values{},
			expectedSearch: "",
		},
		{
			name:           "search with value",
			queryParams:    url.Values{"search": []string{"name='test'"}},
			expectedSearch: "name='test'",
		},
		{
			name:           "search with spaces should be trimmed",
			queryParams:    url.Values{"search": []string{"  name='test'  "}},
			expectedSearch: "name='test'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			listArgs, err := NewListArguments(tt.queryParams)
			Expect(err).To(BeNil(), "Should not return error for valid search parameters")
			Expect(listArgs.Search).To(Equal(tt.expectedSearch), "Search mismatch")
		})
	}
}

func TestNewListArguments_Fields(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name           string
		queryParams    url.Values
		expectedFields []string
	}{
		{
			name:           "no fields parameter",
			queryParams:    url.Values{},
			expectedFields: nil,
		},
		{
			name:           "fields without id - should add id automatically",
			queryParams:    url.Values{"fields": []string{"name,status"}},
			expectedFields: []string{"name", "status", "id"},
		},
		{
			name:           "fields with id - should not duplicate",
			queryParams:    url.Values{"fields": []string{"id,name,status"}},
			expectedFields: []string{"id", "name", "status"},
		},
		{
			name:           "fields with spaces and commas",
			queryParams:    url.Values{"fields": []string{" name , status , id "}},
			expectedFields: []string{"name", "status", "id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			listArgs, err := NewListArguments(tt.queryParams)
			Expect(err).To(BeNil(), "Should not return error for valid fields parameters")
			if !reflect.DeepEqual(listArgs.Fields, tt.expectedFields) {
				t.Errorf("Fields = %v, want %v", listArgs.Fields, tt.expectedFields)
			}
		})
	}
}

// TestNewListArguments_Validation tests pagination parameter validation (HYPERFLEET-1241)
func TestNewListArguments_Validation(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name          string
		queryParams   url.Values
		errorContains string
		errorCode     string
		expectError   bool
	}{
		// Page validation tests
		// Page boundary checks — VAL-004 (CodeValidationRange) for values outside valid range
		{
			name:          "negative page returns error",
			queryParams:   url.Values{"page": []string{"-1"}},
			expectError:   true,
			errorContains: "Invalid page parameter",
			errorCode:     "HYPERFLEET-VAL-004",
		},
		{
			name:          "zero page returns error",
			queryParams:   url.Values{"page": []string{"0"}},
			expectError:   true,
			errorContains: "Invalid page parameter",
			errorCode:     "HYPERFLEET-VAL-004",
		},
		// Page parse errors — VAL-003 (CodeValidationFormat) for non-numeric input
		{
			name:          "non-numeric page returns error",
			queryParams:   url.Values{"page": []string{"abc"}},
			expectError:   true,
			errorContains: "Invalid page parameter",
			errorCode:     "HYPERFLEET-VAL-003",
		},
		{
			name:          "page with special characters returns error",
			queryParams:   url.Values{"page": []string{"<script>"}},
			expectError:   true,
			errorContains: "Invalid page parameter",
			errorCode:     "HYPERFLEET-VAL-003",
		},

		// Size validation tests
		// Size boundary checks — VAL-004 (CodeValidationRange) for values outside valid range (1-100)
		{
			name:          "negative size returns error",
			queryParams:   url.Values{"size": []string{"-1"}},
			expectError:   true,
			errorContains: "Invalid size parameter",
			errorCode:     "HYPERFLEET-VAL-004",
		},
		{
			name:          "zero size returns error",
			queryParams:   url.Values{"size": []string{"0"}},
			expectError:   true,
			errorContains: "Invalid size parameter",
			errorCode:     "HYPERFLEET-VAL-004",
		},
		{
			name:          "size exceeding MaxPageSize returns error",
			queryParams:   url.Values{"size": []string{"101"}},
			expectError:   true,
			errorContains: "exceeds maximum allowed value",
			errorCode:     "HYPERFLEET-VAL-004",
		},
		// Size parse errors — VAL-003 (CodeValidationFormat) for non-numeric input
		{
			name:          "non-numeric size returns error",
			queryParams:   url.Values{"size": []string{"xyz"}},
			expectError:   true,
			errorContains: "Invalid size parameter",
			errorCode:     "HYPERFLEET-VAL-003",
		},

		// PageSize validation tests (OpenAPI spec parameter)
		// PageSize boundary checks — VAL-004 (CodeValidationRange) for values outside valid range (1-100)
		{
			name:          "negative pageSize returns error",
			queryParams:   url.Values{"pageSize": []string{"-1"}},
			expectError:   true,
			errorContains: "Invalid pageSize parameter",
			errorCode:     "HYPERFLEET-VAL-004",
		},
		{
			name:          "zero pageSize returns error",
			queryParams:   url.Values{"pageSize": []string{"0"}},
			expectError:   true,
			errorContains: "Invalid pageSize parameter",
			errorCode:     "HYPERFLEET-VAL-004",
		},
		{
			name:          "pageSize exceeding MaxPageSize returns error",
			queryParams:   url.Values{"pageSize": []string{"101"}},
			expectError:   true,
			errorContains: "exceeds maximum allowed value",
			errorCode:     "HYPERFLEET-VAL-004",
		},
		// PageSize parse errors — VAL-003 (CodeValidationFormat) for non-numeric input
		{
			name:          "non-numeric pageSize returns error",
			queryParams:   url.Values{"pageSize": []string{"xyz"}},
			expectError:   true,
			errorContains: "Invalid pageSize parameter",
			errorCode:     "HYPERFLEET-VAL-003",
		},

		// Order parameter validation tests
		{
			name:          "invalid order value returns error",
			queryParams:   url.Values{"order": []string{"invalid"}},
			expectError:   true,
			errorContains: "must be 'asc' or 'desc'",
			errorCode:     "HYPERFLEET-VAL-003",
		},
		{
			name:          "order with uppercase returns error",
			queryParams:   url.Values{"order": []string{"ASC"}},
			expectError:   true,
			errorContains: "must be 'asc' or 'desc'",
			errorCode:     "HYPERFLEET-VAL-003",
		},
		{
			name:          "order with number returns error",
			queryParams:   url.Values{"order": []string{"1"}},
			expectError:   true,
			errorContains: "must be 'asc' or 'desc'",
			errorCode:     "HYPERFLEET-VAL-003",
		},

		// Valid cases
		{
			name:        "valid page=1 size=1",
			queryParams: url.Values{"page": []string{"1"}, "size": []string{"1"}},
			expectError: false,
		},
		{
			name:        "valid page=1 size=100 (max)",
			queryParams: url.Values{"page": []string{"1"}, "size": []string{"100"}},
			expectError: false,
		},
		{
			name:        "valid page=999 size=50",
			queryParams: url.Values{"page": []string{"999"}, "size": []string{"50"}},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			listArgs, err := NewListArguments(tt.queryParams)

			if tt.expectError {
				Expect(err).ToNot(BeNil(), "Expected error but got nil")
				Expect(err.Reason).To(ContainSubstring(tt.errorContains),
					"Error message should contain expected text")
				Expect(err.RFC9457Code).To(Equal(tt.errorCode),
					"Error code should match expected value")
				Expect(err.HTTPCode).To(Equal(400),
					"HTTP code should be 400 for validation errors")
				Expect(listArgs).To(BeNil(), "ListArgs should be nil on error")
			} else {
				Expect(err).To(BeNil(), "Should not return error for valid parameters")
				Expect(listArgs).ToNot(BeNil(), "ListArgs should not be nil")
			}
		})
	}
}
