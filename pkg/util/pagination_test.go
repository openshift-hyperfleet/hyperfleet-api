package util

import "testing"

func TestParsePageParams(t *testing.T) {
	params, err := ParsePageParams("2", "10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.Limit != 10 {
		t.Errorf("expected limit 10, got %d", params.Limit)
	}
}

func TestParsePageParamsDefaults(t *testing.T) {
	params, _ := ParsePageParams("", "")
	if params.Limit != DefaultPageSize {
		t.Errorf("expected default limit, got %d", params.Limit)
	}
}

func TestTotalPages(t *testing.T) {
	if got := TotalPages(100, 10); got != 10 {
		t.Errorf("expected 10 pages, got %d", got)
	}
}
