package dao

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

func TestResetReadyConditionOnSpecChange(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name               string
		conditions         []api.ResourceCondition
		expectReadyFalse   bool
		expectReasonChange bool
	}{
		{
			name: "Ready=True is flipped to False",
			conditions: []api.ResourceCondition{
				{Type: "Available", Status: api.ConditionTrue},
				{Type: "Ready", Status: api.ConditionTrue},
			},
			expectReadyFalse:   true,
			expectReasonChange: true,
		},
		{
			name: "Ready=False stays False",
			conditions: []api.ResourceCondition{
				{Type: "Available", Status: api.ConditionTrue},
				{Type: "Ready", Status: api.ConditionFalse},
			},
			expectReadyFalse:   true,
			expectReasonChange: false,
		},
		{
			name: "Available is not changed",
			conditions: []api.ResourceCondition{
				{Type: "Available", Status: api.ConditionTrue},
				{Type: "Ready", Status: api.ConditionTrue},
			},
			expectReadyFalse: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			input, err := json.Marshal(tt.conditions)
			Expect(err).ToNot(HaveOccurred())

			result, err := resetReadyConditionOnSpecChange(input, now)
			Expect(err).ToNot(HaveOccurred())

			var resultConditions []api.ResourceCondition
			Expect(json.Unmarshal(result, &resultConditions)).To(Succeed())

			for _, cond := range resultConditions {
				switch cond.Type {
				case "Ready":
					if tt.expectReadyFalse {
						Expect(cond.Status).To(Equal(api.ConditionFalse))
					}
					if tt.expectReasonChange {
						Expect(cond.Reason).ToNot(BeNil())
						Expect(*cond.Reason).To(Equal("SpecChanged"))
						Expect(cond.LastTransitionTime.Equal(now)).To(BeTrue())
					}
				case "Available":
					Expect(cond.Status).To(Equal(api.ConditionTrue))
				}
			}
		})
	}
}

func TestResetReadyConditionOnSpecChange_EmptyConditions(t *testing.T) {
	RegisterTestingT(t)
	now := time.Now()

	result, err := resetReadyConditionOnSpecChange(nil, now)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(BeNil())
}

func TestResetReadyConditionOnSpecChange_InvalidJSON(t *testing.T) {
	RegisterTestingT(t)
	now := time.Now()

	input := []byte(`not valid json`)
	result, err := resetReadyConditionOnSpecChange(input, now)
	Expect(err).ToNot(HaveOccurred())
	Expect(string(result)).To(Equal(string(input)))
}
