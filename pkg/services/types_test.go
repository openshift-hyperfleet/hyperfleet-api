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
			name:            "orderBy with whitespace only - should use default",
			queryParams:     url.Values{"orderBy": []string{"   "}},
			expectedOrderBy: []string{"created_time desc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			listArgs := NewListArguments(tt.queryParams)
			Expect(listArgs.OrderBy).To(Equal(tt.expectedOrderBy),
				"OrderBy mismatch for test case: %s", tt.name)
		})
	}
}

func TestNewListArguments_DefaultValues(t *testing.T) {
	RegisterTestingT(t)

	listArgs := NewListArguments(url.Values{})

	Expect(listArgs.Page).To(Equal(1), "Default page should be 1")
	Expect(listArgs.Size).To(Equal(int64(100)), "Default size should be 100")
	Expect(listArgs.Search).To(Equal(""), "Default search should be empty")
	Expect(listArgs.OrderBy).To(Equal([]string{"created_time desc"}), "Default orderBy should be created_time desc")
}

func TestNewListArguments_PageSize(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name         string
		queryParams  url.Values
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
		{
			name:         "negative size defaults to MaxListSize",
			queryParams:  url.Values{"pageSize": []string{"-1"}},
			expectedPage: 1,
			expectedSize: MaxListSize,
		},
		{
			name:         "size exceeding MaxListSize defaults to MaxListSize",
			queryParams:  url.Values{"pageSize": []string{"100000"}},
			expectedPage: 1,
			expectedSize: MaxListSize,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			listArgs := NewListArguments(tt.queryParams)
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
			listArgs := NewListArguments(tt.queryParams)
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
			listArgs := NewListArguments(tt.queryParams)
			if !reflect.DeepEqual(listArgs.Fields, tt.expectedFields) {
				t.Errorf("Fields = %v, want %v", listArgs.Fields, tt.expectedFields)
			}
		})
	}
}
