package services

import (
	"net/url"
	"reflect"
	"testing"

	. "github.com/onsi/gomega"
)

func TestNewListArguments_Order(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name          string
		queryParams   url.Values
		expectedOrder []string
	}{
		{
			name:          "no order - should use default created_time desc",
			queryParams:   url.Values{},
			expectedOrder: []string{"created_time desc"},
		},
		{
			name:          "order with asc direction",
			queryParams:   url.Values{"order": []string{"name asc"}},
			expectedOrder: []string{"name asc"},
		},
		{
			name:          "order with desc direction",
			queryParams:   url.Values{"order": []string{"name desc"}},
			expectedOrder: []string{"name desc"},
		},
		{
			name:          "order without direction",
			queryParams:   url.Values{"order": []string{"name"}},
			expectedOrder: []string{"name"},
		},
		{
			name:          "multiple order fields",
			queryParams:   url.Values{"order": []string{"name asc,created_time desc"}},
			expectedOrder: []string{"name asc", "created_time desc"},
		},
		{
			name:          "order with spaces should be trimmed",
			queryParams:   url.Values{"order": []string{" name asc "}},
			expectedOrder: []string{"name asc"},
		},
		{
			name:          "empty order string - should use default",
			queryParams:   url.Values{"order": []string{""}},
			expectedOrder: []string{"created_time desc"},
		},
		{
			name:          "order with whitespace only - should use default",
			queryParams:   url.Values{"order": []string{"   "}},
			expectedOrder: []string{"created_time desc"},
		},
		{
			name:          "order with empty tokens - should filter out",
			queryParams:   url.Values{"order": []string{"name,,created_time"}},
			expectedOrder: []string{"name", "created_time"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			listArgs, err := NewListArguments(tt.queryParams)
			Expect(err).To(BeNil(), "Should not return error for valid order parameters")
			Expect(listArgs.Order).To(Equal(tt.expectedOrder),
				"Order mismatch for test case: %s", tt.name)
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
	Expect(listArgs.Order).To(Equal([]string{"created_time desc"}), "Default order should be created_time desc")
}

func TestNewListArguments_Size(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		queryParams  url.Values
		name         string
		expectedPage int
		expectedSize int64
	}{
		{
			name:         "custom page and size",
			queryParams:  url.Values{"page": []string{"2"}, "size": []string{"50"}},
			expectedPage: 2,
			expectedSize: 50,
		},
		{
			name:         "custom page and size (different values)",
			queryParams:  url.Values{"page": []string{"3"}, "size": []string{"25"}},
			expectedPage: 3,
			expectedSize: 25,
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

func TestNewListArguments_RefTypeWithoutTargetID_Returns400(t *testing.T) {
	RegisterTestingT(t)

	params := url.Values{"ref_type": []string{"dep"}}
	listArgs, err := NewListArguments(params)
	Expect(listArgs).To(BeNil())
	Expect(err).ToNot(BeNil())
	Expect(err.HTTPCode).To(Equal(400))
	Expect(err.Reason).To(ContainSubstring("ref_type and ref_target_id must be provided together"))
}

func TestNewListArguments_RefTargetIDWithoutRefType_Returns400(t *testing.T) {
	RegisterTestingT(t)

	params := url.Values{"ref_target_id": []string{"some-id"}}
	listArgs, err := NewListArguments(params)
	Expect(listArgs).To(BeNil())
	Expect(err).ToNot(BeNil())
	Expect(err.HTTPCode).To(Equal(400))
	Expect(err.Reason).To(ContainSubstring("ref_type and ref_target_id must be provided together"))
}

func TestNewListArguments_RefTypePairValid(t *testing.T) {
	RegisterTestingT(t)

	params := url.Values{
		"ref_type":      []string{"dep"},
		"ref_target_id": []string{"target-1"},
	}
	listArgs, err := NewListArguments(params)
	Expect(err).To(BeNil())
	Expect(listArgs.RefType).To(Equal("dep"))
	Expect(listArgs.RefTargetID).To(Equal("target-1"))
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

		// Additional size edge cases
		{
			name:          "size with special characters returns error",
			queryParams:   url.Values{"size": []string{"<script>"}},
			expectError:   true,
			errorContains: "Invalid size parameter",
			errorCode:     "HYPERFLEET-VAL-003",
		},
		{
			name:          "very large size returns error",
			queryParams:   url.Values{"size": []string{"999999"}},
			expectError:   true,
			errorContains: "exceeds maximum allowed value",
			errorCode:     "HYPERFLEET-VAL-004",
		},
		{
			name:        "pageSize is ignored (no longer a valid parameter)",
			queryParams: url.Values{"pageSize": []string{"50"}},
			expectError: false,
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
