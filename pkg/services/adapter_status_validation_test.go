package services

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"gorm.io/datatypes"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

func testConditionsJSON(conditions ...api.AdapterCondition) datatypes.JSON {
	b, _ := json.Marshal(conditions)
	return b
}

func testMandatoryConditions(availableStatus api.AdapterConditionStatus) []api.AdapterCondition {
	return []api.AdapterCondition{
		{Type: api.AdapterConditionTypeAvailable, Status: availableStatus},
		{Type: api.AdapterConditionTypeApplied, Status: api.AdapterConditionTrue},
		{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue},
	}
}

func adapterStatusWithGenAndTime(gen int32, observedTime time.Time) *api.AdapterStatus {
	return &api.AdapterStatus{
		Adapter:            "test-adapter",
		ObservedGeneration: gen,
		LastReportTime:     observedTime,
		Conditions:         testConditionsJSON(testMandatoryConditions(api.AdapterConditionTrue)...),
	}
}

func testLog() *logger.ContextLogger {
	return logger.With(context.Background(), "test", "true")
}

func TestValidateAndClassify_FutureGeneration_Discards(t *testing.T) {
	RegisterTestingT(t)

	status := adapterStatusWithGenAndTime(5, time.Now())
	conditions, trigger, err := validateAndClassifyAdapterStatus(3, status, nil, testLog())

	Expect(err).To(BeNil())
	Expect(conditions).To(BeNil())
	Expect(trigger).To(BeFalse())
}

func TestValidateAndClassify_StaleGeneration_Discards(t *testing.T) {
	RegisterTestingT(t)

	existing := &api.AdapterStatus{ObservedGeneration: 3}
	status := adapterStatusWithGenAndTime(2, time.Now())
	conditions, trigger, err := validateAndClassifyAdapterStatus(5, status, existing, testLog())

	Expect(err).To(BeNil())
	Expect(conditions).To(BeNil())
	Expect(trigger).To(BeFalse())
}

func TestValidateAndClassify_ZeroObservedTime_Discards(t *testing.T) {
	RegisterTestingT(t)

	status := &api.AdapterStatus{
		Adapter:            "test-adapter",
		ObservedGeneration: 1,
		Conditions:         testConditionsJSON(testMandatoryConditions(api.AdapterConditionTrue)...),
	}
	conditions, trigger, err := validateAndClassifyAdapterStatus(1, status, nil, testLog())

	Expect(err).To(BeNil())
	Expect(conditions).To(BeNil())
	Expect(trigger).To(BeFalse())
}

func TestValidateAndClassify_StaleObservedTime_Discards(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now().UTC()
	existing := adapterStatusWithGenAndTime(1, now)
	status := adapterStatusWithGenAndTime(1, now.Add(-time.Minute))

	conditions, trigger, err := validateAndClassifyAdapterStatus(1, status, existing, testLog())

	Expect(err).To(BeNil())
	Expect(conditions).To(BeNil())
	Expect(trigger).To(BeFalse())
}

func TestValidateAndClassify_MissingMandatoryCondition_ReturnsError(t *testing.T) {
	RegisterTestingT(t)

	status := adapterStatusWithGenAndTime(1, time.Now())
	status.Conditions = testConditionsJSON(
		api.AdapterCondition{Type: api.AdapterConditionTypeAvailable, Status: api.AdapterConditionTrue},
	)

	_, _, err := validateAndClassifyAdapterStatus(1, status, nil, testLog())

	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("mandatory condition"))
}

func TestValidateAndClassify_InvalidAvailableStatus_ReturnsError(t *testing.T) {
	RegisterTestingT(t)

	status := adapterStatusWithGenAndTime(1, time.Now())
	status.Conditions = testConditionsJSON(
		api.AdapterCondition{Type: api.AdapterConditionTypeAvailable, Status: "Invalid"},
		api.AdapterCondition{Type: api.AdapterConditionTypeApplied, Status: api.AdapterConditionTrue},
		api.AdapterCondition{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue},
	)

	_, _, err := validateAndClassifyAdapterStatus(1, status, nil, testLog())

	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("invalid status"))
}

func TestValidateAndClassify_FirstUnknownAvailable_Accepted(t *testing.T) {
	RegisterTestingT(t)

	status := adapterStatusWithGenAndTime(1, time.Now())
	status.Conditions = testConditionsJSON(testMandatoryConditions(api.AdapterConditionUnknown)...)

	conditions, trigger, err := validateAndClassifyAdapterStatus(1, status, nil, testLog())

	Expect(err).To(BeNil())
	Expect(conditions).ToNot(BeNil())
	Expect(trigger).To(BeFalse())
}

func TestValidateAndClassify_SubsequentUnknownAvailable_Discards(t *testing.T) {
	RegisterTestingT(t)

	existing := adapterStatusWithGenAndTime(1, time.Now().Add(-time.Second))
	status := adapterStatusWithGenAndTime(1, time.Now())
	status.Conditions = testConditionsJSON(testMandatoryConditions(api.AdapterConditionUnknown)...)

	conditions, trigger, err := validateAndClassifyAdapterStatus(1, status, existing, testLog())

	Expect(err).To(BeNil())
	Expect(conditions).To(BeNil())
	Expect(trigger).To(BeFalse())
}

func TestValidateAndClassify_AvailableTrue_TriggersAggregation(t *testing.T) {
	RegisterTestingT(t)

	status := adapterStatusWithGenAndTime(1, time.Now())

	conditions, trigger, err := validateAndClassifyAdapterStatus(1, status, nil, testLog())

	Expect(err).To(BeNil())
	Expect(conditions).ToNot(BeNil())
	Expect(trigger).To(BeTrue())
}

func TestValidateAndClassify_AvailableFalse_TriggersAggregation(t *testing.T) {
	RegisterTestingT(t)

	status := adapterStatusWithGenAndTime(1, time.Now())
	status.Conditions = testConditionsJSON(testMandatoryConditions(api.AdapterConditionFalse)...)

	conditions, trigger, err := validateAndClassifyAdapterStatus(1, status, nil, testLog())

	Expect(err).To(BeNil())
	Expect(conditions).ToNot(BeNil())
	Expect(trigger).To(BeTrue())
}
