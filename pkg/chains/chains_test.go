package chains

import (
	"testing"

	. "github.com/onsi/gomega"
)

var testChains = []Chain{
	{Topology: "hosted", Platform: "coreshift", Adapters: []string{"coreshift"}},
}

func TestLookup(t *testing.T) {
	RegisterTestingT(t)

	adapters, ok := Lookup(testChains, "hosted", "coreshift")
	Expect(ok).To(BeTrue())
	Expect(adapters).To(Equal([]string{"coreshift"}))

	adapters, ok = Lookup(testChains, "standalone", "coreshift")
	Expect(ok).To(BeFalse())
	Expect(adapters).To(BeNil())
}

func TestLookupSpec(t *testing.T) {
	tests := []struct {
		spec   map[string]interface{}
		name   string
		wantOK bool
	}{
		{
			name:   "reads both axes from spec",
			spec:   map[string]interface{}{"topology": "hosted", "platform": "coreshift"},
			wantOK: true,
		},
		{
			name: "missing platform returns no chain",
			spec: map[string]interface{}{"topology": "hosted"},
		},
		{
			name: "non-string platform returns no chain",
			spec: map[string]interface{}{"topology": "hosted", "platform": 42},
		},
		{
			name: "nil spec returns no chain",
			spec: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			adapters, ok := LookupSpec(testChains, tc.spec)
			Expect(ok).To(Equal(tc.wantOK))
			if tc.wantOK {
				Expect(adapters).To(Equal([]string{"coreshift"}))
			} else {
				Expect(adapters).To(BeNil())
			}
		})
	}
}

func TestValidate(t *testing.T) {
	known := map[string]bool{"coreshift": true, "dns": true}

	tests := []struct {
		name      string
		errSubstr string
		chains    []Chain
	}{
		{
			name:   "valid chain passes",
			chains: testChains,
		},
		{
			name:      "missing topology fails",
			chains:    []Chain{{Platform: "coreshift", Adapters: []string{"coreshift"}}},
			errSubstr: "topology is required",
		},
		{
			name:      "missing platform fails",
			chains:    []Chain{{Topology: "hosted", Adapters: []string{"coreshift"}}},
			errSubstr: "platform is required",
		},
		{
			name:      "empty adapter list fails",
			chains:    []Chain{{Topology: "hosted", Platform: "coreshift"}},
			errSubstr: "requires at least one adapter",
		},
		{
			name: "duplicate pair fails",
			chains: []Chain{
				{Topology: "hosted", Platform: "coreshift", Adapters: []string{"coreshift"}},
				{Topology: "hosted", Platform: "coreshift", Adapters: []string{"dns"}},
			},
			errSubstr: "duplicate chain for hosted/coreshift",
		},
		{
			name:      "unknown adapter fails",
			chains:    []Chain{{Topology: "hosted", Platform: "coreshift", Adapters: []string{"missing"}}},
			errSubstr: `chain hosted/coreshift references unknown adapter "missing"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := Validate(tc.chains, known)
			if tc.errSubstr == "" {
				Expect(err).NotTo(HaveOccurred())
				return
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(tc.errSubstr))
		})
	}
}
