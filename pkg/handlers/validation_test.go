package handlers

import (
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
)

func TestValidateName_Valid(t *testing.T) {
	RegisterTestingT(t)

	validNames := []string{
		"test",
		"test-cluster",
		"my-cluster-123",
		"123",
		"test-123-cluster",
		"a1b2c3",
		"abc",
	}

	for _, name := range validNames {
		req := openapi.ClusterCreateRequest{
			Name: name,
		}
		validator := validateName(&req, "Name", "name", 3, 53)
		err := validator()
		Expect(err).To(BeNil(), "Expected name '%s' to be valid", name)
	}
}

func TestValidateName_TooShort(t *testing.T) {
	RegisterTestingT(t)

	shortNames := []string{
		"",   // empty
		"a",  // 1 char
		"ab", // 2 chars
	}

	for _, name := range shortNames {
		req := openapi.ClusterCreateRequest{
			Name: name,
		}
		validator := validateName(&req, "Name", "name", 3, 53)
		err := validator()
		Expect(err).ToNot(BeNil(), "Expected name '%s' to be invalid (too short)", name)
		if name == "" {
			Expect(err.Reason).To(ContainSubstring("required"))
		} else {
			Expect(err.Reason).To(ContainSubstring("at least 3 characters"))
		}
	}
}

func TestValidateName_TooLong(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.ClusterCreateRequest{
		Name: "this-is-a-very-long-name-that-exceeds-the-maximum-allowed-length-for-cluster-names",
	}
	validator := validateName(&req, "Name", "name", 3, 53)
	err := validator()
	Expect(err).ToNot(BeNil())
	Expect(err.Reason).To(ContainSubstring("at most 53 characters"))
}

func TestValidateName_InvalidCharacters(t *testing.T) {
	RegisterTestingT(t)

	invalidNames := []string{
		"TEST",          // uppercase
		"Test",          // mixed case
		"test_cluster",  // underscore
		"test.cluster",  // dot
		"test cluster",  // space
		"test@cluster",  // special char
		"test/cluster",  // slash
		"test\\cluster", // backslash
		"-test",         // starts with hyphen
		"test-",         // ends with hyphen
		"-test-",        // starts and ends with hyphen
	}

	for _, name := range invalidNames {
		req := openapi.ClusterCreateRequest{
			Name: name,
		}
		validator := validateName(&req, "Name", "name", 3, 53)
		err := validator()
		Expect(err).ToNot(BeNil(), "Expected name '%s' to be invalid", name)
		Expect(err.Reason).To(ContainSubstring("lowercase letters, numbers, and hyphens"))
	}
}

func TestValidateKind_Valid(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.ClusterCreateRequest{
		Kind: "Cluster",
	}
	validator := validateKind(&req, "Kind", "kind", "Cluster")
	err := validator()
	Expect(err).To(BeNil())
}

func TestValidateKind_Invalid(t *testing.T) {
	RegisterTestingT(t)

	invalidKinds := []string{
		"cluster",  // lowercase
		"CLUSTER",  // uppercase
		"NodePool", // wrong kind
		"",         // empty
	}

	for _, kind := range invalidKinds {
		req := openapi.ClusterCreateRequest{
			Kind: kind,
		}
		validator := validateKind(&req, "Kind", "kind", "Cluster")
		err := validator()
		Expect(err).ToNot(BeNil(), "Expected kind '%s' to be invalid", kind)
	}
}

func TestValidateKind_Empty(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.ClusterCreateRequest{
		Kind: "",
	}
	validator := validateKind(&req, "Kind", "kind", "Cluster")
	err := validator()
	Expect(err).ToNot(BeNil())
	Expect(err.Reason).To(ContainSubstring("required"))
}

func TestValidateKind_WrongKind(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.ClusterCreateRequest{
		Kind: "WrongKind",
	}
	validator := validateKind(&req, "Kind", "kind", "Cluster")
	err := validator()
	Expect(err).ToNot(BeNil())
	Expect(err.Reason).To(ContainSubstring("must be 'Cluster'"))
}

func TestValidateNodePoolName_Valid(t *testing.T) {
	RegisterTestingT(t)

	validNames := []string{
		"abc",
		"worker-pool-1",
		"np1",
		"a1b",
		"my-pool",
		"pool-123456789", // 15 chars
	}

	for _, name := range validNames {
		req := openapi.NodePoolCreateRequest{
			Name: name,
		}
		validator := validateName(&req, "Name", "name", 3, 15)
		err := validator()
		Expect(err).To(BeNil(), "Expected nodepool name '%s' to be valid", name)
	}
}

func TestValidateNodePoolName_TooShort(t *testing.T) {
	RegisterTestingT(t)

	shortNames := []string{
		"",   // empty
		"a",  // 1 char
		"ab", // 2 chars
	}

	for _, name := range shortNames {
		req := openapi.NodePoolCreateRequest{
			Name: name,
		}
		validator := validateName(&req, "Name", "name", 3, 15)
		err := validator()
		Expect(err).ToNot(BeNil(), "Expected nodepool name '%s' to be invalid (too short)", name)
		if name == "" {
			Expect(err.Reason).To(ContainSubstring("required"))
		} else {
			Expect(err.Reason).To(ContainSubstring("at least 3 characters"))
		}
	}
}

func TestValidateNodePoolName_TooLong(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.NodePoolCreateRequest{
		Name: "this-is-too-long", // 16 chars
	}
	validator := validateName(&req, "Name", "name", 3, 15)
	err := validator()
	Expect(err).ToNot(BeNil())
	Expect(err.Reason).To(ContainSubstring("at most 15 characters"))
}

func TestValidateNodePoolName_InvalidCharacters(t *testing.T) {
	RegisterTestingT(t)

	invalidNames := []string{
		"TEST",      // uppercase
		"Test",      // mixed case
		"test_pool", // underscore
		"test.pool", // dot
		"test pool", // space
		"-test",     // starts with hyphen
		"test-",     // ends with hyphen
	}

	for _, name := range invalidNames {
		req := openapi.NodePoolCreateRequest{
			Name: name,
		}
		validator := validateName(&req, "Name", "name", 3, 15)
		err := validator()
		Expect(err).ToNot(BeNil(), "Expected nodepool name '%s' to be invalid", name)
		Expect(err.Reason).To(ContainSubstring("lowercase letters, numbers, and hyphens"))
	}
}

func TestValidateMaxLength(t *testing.T) {
	testCases := []struct {
		name        string
		reason      string
		errContains string
		maxLen      int
		expectError bool
	}{
		{
			name:        "valid short reason",
			reason:      "Stuck in finalizing for 2 hours",
			maxLen:      1024,
			expectError: false,
		},
		{
			name:        "exact limit",
			reason:      strings.Repeat("a", 1024),
			maxLen:      1024,
			expectError: false,
		},
		{
			name:        "exceeds limit by one",
			reason:      strings.Repeat("a", 1025),
			maxLen:      1024,
			expectError: true,
			errContains: "at most 1024 characters",
		},
		{
			name:        "empty string",
			reason:      "",
			maxLen:      1024,
			expectError: false,
		},
		{
			name:        "multibyte at exact limit counts runes not bytes",
			reason:      strings.Repeat("é", 1024),
			maxLen:      1024,
			expectError: false,
		},
		{
			name:        "multibyte over limit counts runes not bytes",
			reason:      strings.Repeat("é", 1025),
			maxLen:      1024,
			expectError: true,
			errContains: "at most 1024 characters",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			req := openapi.ForceDeleteRequest{Reason: tc.reason}
			validator := validateMaxLength(&req, "Reason", "reason", tc.maxLen)
			err := validator()
			if tc.expectError {
				Expect(err).ToNot(BeNil())
				Expect(err.Reason).To(ContainSubstring(tc.errContains))
			} else {
				Expect(err).To(BeNil())
			}
		})
	}
}

func TestValidateMaxLength_PointerField(t *testing.T) {
	type ptrReq struct {
		Reason *string
	}

	t.Run("nil pointer skips validation", func(t *testing.T) {
		RegisterTestingT(t)
		req := ptrReq{Reason: nil}
		validator := validateMaxLength(&req, "Reason", "reason", 10)
		Expect(validator()).To(BeNil())
	})

	t.Run("pointer to too-long string returns error", func(t *testing.T) {
		RegisterTestingT(t)
		long := strings.Repeat("a", 11)
		req := ptrReq{Reason: &long}
		validator := validateMaxLength(&req, "Reason", "reason", 10)
		err := validator()
		Expect(err).ToNot(BeNil())
		Expect(err.Reason).To(ContainSubstring("at most 10 characters"))
	})
}

func TestValidateSpec_Valid(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.ClusterCreateRequest{
		Spec: map[string]interface{}{"test": "value"},
	}
	validator := validateSpec(&req, "Spec", "spec")
	err := validator()
	Expect(err).To(BeNil(), "Expected existing spec to be valid")
}

func TestValidateSpec_EmptyMap(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.ClusterCreateRequest{
		Spec: map[string]interface{}{},
	}
	validator := validateSpec(&req, "Spec", "spec")
	err := validator()
	Expect(err).To(BeNil(), "Expected empty map spec to be valid")
}

func TestValidateSpec_Nil(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.ClusterCreateRequest{
		Spec: nil,
	}
	validator := validateSpec(&req, "Spec", "spec")
	err := validator()
	Expect(err).ToNot(BeNil(), "Expected nil spec to be invalid")
	Expect(err.Reason).To(ContainSubstring("spec is required"))
}

func TestValidatePatchRequest(t *testing.T) {
	spec := openapi.ClusterSpec{"key": "value"}
	labels := map[string]string{"env": "prod"}

	testCases := []struct {
		req         openapi.ClusterPatchRequest
		name        string
		expectError bool
	}{
		{
			name:        "both nil returns error",
			req:         openapi.ClusterPatchRequest{},
			expectError: true,
		},
		{
			name:        "spec only is valid",
			req:         openapi.ClusterPatchRequest{Spec: &spec},
			expectError: false,
		},
		{
			name:        "labels only is valid",
			req:         openapi.ClusterPatchRequest{Labels: &labels},
			expectError: false,
		},
		{
			name:        "both spec and labels is valid",
			req:         openapi.ClusterPatchRequest{Spec: &spec, Labels: &labels},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			validator := validatePatchRequest(&tc.req)
			err := validator()
			if tc.expectError {
				Expect(err).ToNot(BeNil())
				Expect(err.Reason).To(ContainSubstring("at least one field must be provided for update"))
			} else {
				Expect(err).To(BeNil())
			}
		})
	}
}

func TestValidateObservedGeneration(t *testing.T) {
	testCases := []struct {
		name        string
		generation  int32
		expectError bool
	}{
		{"valid generation 1", 1, false},
		{"valid generation 100", 100, false},
		{"zero is invalid", 0, true},
		{"negative is invalid", -1, true},
		{"large negative is invalid", -999, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			req := openapi.AdapterStatusCreateRequest{
				ObservedGeneration: tc.generation,
			}
			validator := validateObservedGeneration(&req)
			err := validator()
			if tc.expectError {
				Expect(err).ToNot(BeNil())
				Expect(err.Reason).To(ContainSubstring("observed_generation must be >= 1"))
			} else {
				Expect(err).To(BeNil())
			}
		})
	}
}

func TestValidateConditions_Valid(t *testing.T) {
	RegisterTestingT(t)

	validStatuses := []openapi.AdapterConditionStatus{
		openapi.AdapterConditionStatusTrue,
		openapi.AdapterConditionStatusFalse,
		openapi.AdapterConditionStatusUnknown,
	}

	for _, status := range validStatuses {
		req := openapi.AdapterStatusCreateRequest{
			Conditions: []openapi.ConditionRequest{
				{
					Type:   api.AdapterConditionTypeReconciled,
					Status: status,
				},
			},
		}
		validator := validateConditions(&req, "Conditions")
		err := validator()
		Expect(err).To(BeNil(), "Expected status '%s' to be valid", status)
	}
}

func TestValidateConditions_InvalidStatusValues(t *testing.T) {
	RegisterTestingT(t)

	testCases := []struct {
		name          string
		invalidStatus openapi.AdapterConditionStatus
	}{
		{"empty string", ""},
		{"lowercase true", "true"},
		{"lowercase false", "false"},
		{"lowercase unknown", "unknown"},
		{"random string", "InvalidValue"},
		{"mixed case", "TrUe"},
	}

	for _, tc := range testCases {
		req := openapi.AdapterStatusCreateRequest{
			Conditions: []openapi.ConditionRequest{
				{
					Type:   api.AdapterConditionTypeAvailable,
					Status: tc.invalidStatus,
				},
			},
		}
		validator := validateConditions(&req, "Conditions")
		err := validator()
		Expect(err).ToNot(BeNil(), "Expected error for: "+tc.name)
		Expect(err.Reason).To(ContainSubstring("invalid status value"), "Expected validation error for: "+tc.name)
		Expect(err.Reason).To(ContainSubstring(string(tc.invalidStatus)),
			"Expected error to mention the invalid value for: "+tc.name)
	}
}

func TestValidateConditions_EmptyType(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.AdapterStatusCreateRequest{
		Conditions: []openapi.ConditionRequest{
			{
				Type:   "",
				Status: openapi.AdapterConditionStatusTrue,
			},
		},
	}
	validator := validateConditions(&req, "Conditions")
	err := validator()
	Expect(err).ToNot(BeNil())
	Expect(err.Reason).To(ContainSubstring("condition type cannot be empty"))
	Expect(err.Reason).To(ContainSubstring("index 0"))
}

func TestValidateConditions_DuplicateType(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.AdapterStatusCreateRequest{
		Conditions: []openapi.ConditionRequest{
			{
				Type:   api.AdapterConditionTypeAvailable,
				Status: openapi.AdapterConditionStatusTrue,
			},
			{
				Type:   api.AdapterConditionTypeApplied,
				Status: openapi.AdapterConditionStatusTrue,
			},
			{
				Type:   api.AdapterConditionTypeAvailable, // Duplicate
				Status: openapi.AdapterConditionStatusFalse,
			},
		},
	}
	validator := validateConditions(&req, "Conditions")
	err := validator()
	Expect(err).ToNot(BeNil())
	Expect(err.Reason).To(ContainSubstring("duplicate condition type"))
	Expect(err.Reason).To(ContainSubstring(api.AdapterConditionTypeAvailable))
	Expect(err.Reason).To(ContainSubstring("index 2"))
}

func TestValidateConditions_MultipleIssues(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.AdapterStatusCreateRequest{
		Conditions: []openapi.ConditionRequest{
			{
				Type:   api.AdapterConditionTypeReconciled,
				Status: openapi.AdapterConditionStatusTrue,
			},
			{
				Type:   api.AdapterConditionTypeAvailable,
				Status: "", // Invalid status - should be caught first at this index
			},
			{
				Type:   "Degraded",
				Status: "invalid", // Also invalid but should report the first one
			},
		},
	}

	validator := validateConditions(&req, "Conditions")
	err := validator()
	Expect(err).ToNot(BeNil())
	Expect(err.Reason).To(ContainSubstring("index 1"))                         // Second condition (index 1)
	Expect(err.Reason).To(ContainSubstring(api.AdapterConditionTypeAvailable)) // Type of the invalid condition
}

func TestValidateLabels_Valid(t *testing.T) {
	RegisterTestingT(t)

	validCases := []map[string]string{
		nil,                                  // nil labels: always valid
		{},                                   // empty map
		{"env": "prod"},                      // simple key/value
		{"app": ""},                          // empty value is valid (Kubernetes allows it)
		{"my-key": "my-value"},               // hyphens
		{"key.with.dots": "value.with.dots"}, // dots
		{"key_with_underscores": "val_underscore"}, // underscores
		{"A1": "B2"},                         // uppercase
		{"app.kubernetes.io/name": "my-app"}, // prefixed key
		{"a": "b"},                           // single char key and value
		{"123": "456"},                       // numeric keys/values
	}

	for _, labels := range validCases {
		req := openapi.ClusterCreateRequest{Labels: &labels}
		if labels == nil {
			req = openapi.ClusterCreateRequest{}
		}
		validator := validateLabels(&req, "Labels")
		err := validator()
		Expect(err).To(BeNil(), "Expected labels %v to be valid", labels)
	}
}

func TestValidateLabels_InvalidKeys(t *testing.T) {
	RegisterTestingT(t)

	invalidKeys := []string{
		"",                                   // empty key not allowed
		"<script>alert(xss)</script>",        // XSS payload
		"../../etc/passwd",                   // path traversal
		"<img src=x onerror=alert(1)>",       // HTML injection
		"key with spaces",                    // spaces not allowed
		"-starts-with-hyphen",                // key cannot start with hyphen
		"ends-with-hyphen-",                  // key cannot end with hyphen
		"key/with/two/slashes",               // only one slash (prefix separator) allowed
		"/missing-prefix",                    // slash with no prefix
		"key\x00null",                        // null byte
		"APP.io/name",                        // uppercase in DNS prefix
		"my_app.io/name",                     // underscore in DNS prefix
		strings.Repeat("a", 64) + ".io/name", // prefix DNS segment exceeds 63 chars
		strings.Repeat("a", 64),              // name without prefix exceeds 63 chars
	}

	for _, k := range invalidKeys {
		labels := map[string]string{k: "valid-value"}
		req := openapi.ClusterCreateRequest{Labels: &labels}
		validator := validateLabels(&req, "Labels")
		err := validator()
		Expect(err).ToNot(BeNil(), "Expected key %q to be invalid", k)
		Expect(err.Reason).To(ContainSubstring("label validation failed"))
	}
}

func TestValidateLabels_InvalidValues(t *testing.T) {
	RegisterTestingT(t)

	invalidValues := []string{
		"value with spaces",                      // spaces not allowed
		"-starts-with-hyphen",                    // value cannot start with hyphen
		"ends-with-hyphen-",                      // value cannot end with hyphen
		string(make([]byte, maxLabelValueLen+1)), // exceeds 63 chars
	}

	for _, v := range invalidValues {
		labels := map[string]string{"valid-key": v}
		req := openapi.ClusterCreateRequest{Labels: &labels}
		validator := validateLabels(&req, "Labels")
		err := validator()
		Expect(err).ToNot(BeNil(), "Expected value %q to be invalid", v)
		Expect(err.Reason).To(ContainSubstring("label validation failed"))
	}
}

// TestValidateLabels_KeyExceedsDBColumnLimit verifies that a label key which is
// valid per the Kubernetes label-key spec (max 317 chars: 253-char DNS prefix +
// "/" + 63-char name) but exceeds the resource_labels.key VARCHAR(255) column
// is rejected here with a clean 400, instead of passing this check and later
// blowing up as a 500 during resource conversion (api.ValidateLabel).
func TestValidateLabels_KeyExceedsDBColumnLimit(t *testing.T) {
	RegisterTestingT(t)

	seg := strings.Repeat("a", 62)
	prefix := strings.Join([]string{seg, seg, seg, seg}, ".") // 4*62 + 3 = 251 chars, each segment valid DNS-1123
	name := strings.Repeat("b", 63)
	key := prefix + "/" + name // 251 + 1 + 63 = 315 chars: valid K8s key, exceeds VARCHAR(255)
	Expect(len(key)).To(Equal(315))
	Expect(isValidLabelKeyPrefix(prefix)).To(BeTrue(), "prefix must be spec-valid for this test to be meaningful")

	labels := map[string]string{key: "valid-value"}
	req := openapi.ClusterCreateRequest{Labels: &labels}
	validator := validateLabels(&req, "Labels")
	err := validator()

	Expect(err).ToNot(BeNil(), "expected an oversized-but-K8s-valid key to fail validation")
	Expect(err.Reason).To(ContainSubstring("label validation failed"))
}

func TestValidateLabels_NilLabels(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.ClusterCreateRequest{}
	validator := validateLabels(&req, "Labels")
	err := validator()
	Expect(err).To(BeNil())
}

func TestValidateLabels_WorksOnPatchRequest(t *testing.T) {
	RegisterTestingT(t)

	labels := map[string]string{"<xss>": "payload"}
	patch := openapi.ClusterPatchRequest{Labels: &labels}
	validator := validateLabels(&patch, "Labels")
	err := validator()
	Expect(err).ToNot(BeNil())
	Expect(err.Reason).To(ContainSubstring("label validation failed"))
}

func TestValidateConditions_EmptyConditions(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.AdapterStatusCreateRequest{
		Conditions: []openapi.ConditionRequest{},
	}
	validator := validateConditions(&req, "Conditions")
	err := validator()
	Expect(err).To(BeNil(), "Expected empty conditions to be valid")
}

func TestValidateObservedTimeRange(t *testing.T) {
	now := time.Now()
	testCases := []struct {
		observedTime time.Time
		name         string
		expectError  bool
	}{
		{
			name:         "zero time is valid (skipped)",
			observedTime: time.Time{},
			expectError:  false,
		},
		{
			name:         "current time is valid",
			observedTime: now,
			expectError:  false,
		},
		{
			name:         "2 minutes in the future is valid (within tolerance)",
			observedTime: now.Add(2 * time.Minute),
			expectError:  false,
		},
		{
			name:         "4 minutes in the future is valid (within tolerance)",
			observedTime: now.Add(4 * time.Minute),
			expectError:  false,
		},
		{
			name:         "just inside 5 minutes in the future is valid (boundary)",
			observedTime: now.Add(5*time.Minute - 1*time.Second),
			expectError:  false,
		},
		{
			name:         "just over 5 minutes in the future is rejected (boundary)",
			observedTime: now.Add(5*time.Minute + 1*time.Second),
			expectError:  true,
		},
		{
			name:         "10 minutes in the future is rejected",
			observedTime: now.Add(10 * time.Minute),
			expectError:  true,
		},
		{
			name:         "far future (year 2099) is rejected",
			observedTime: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
			expectError:  true,
		},
		{
			name:         "10 minutes in the past is valid (within 30 min tolerance)",
			observedTime: now.Add(-10 * time.Minute),
			expectError:  false,
		},
		{
			name:         "29 minutes in the past is valid (within tolerance)",
			observedTime: now.Add(-29 * time.Minute),
			expectError:  false,
		},
		{
			name:         "just inside 30 minutes in the past is valid (boundary)",
			observedTime: now.Add(-30*time.Minute + 1*time.Second),
			expectError:  false,
		},
		{
			name:         "just over 30 minutes in the past is rejected (boundary)",
			observedTime: now.Add(-30*time.Minute - 1*time.Second),
			expectError:  true,
		},
		{
			name:         "1 hour in the past is rejected",
			observedTime: now.Add(-1 * time.Hour),
			expectError:  true,
		},
		{
			name:         "far past (year 2000) is rejected",
			observedTime: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
			expectError:  true,
		},
	}

	t.Run("nil pointer is valid (no panic)", func(t *testing.T) {
		RegisterTestingT(t)
		validator := validateObservedTimeRange(nil)
		err := validator()
		Expect(err).To(BeNil())
	})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			observedTime := tc.observedTime
			validator := validateObservedTimeRange(&observedTime)
			err := validator()
			if tc.expectError {
				Expect(err).ToNot(BeNil(), "Expected error for: %s", tc.name)
				Expect(err.Reason).To(ContainSubstring("observed_time must not be more than"))
			} else {
				Expect(err).To(BeNil(), "Expected no error for: %s", tc.name)
			}
		})
	}
}
