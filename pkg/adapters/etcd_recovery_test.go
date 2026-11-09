package adapters

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

func healthyMember(name, mc string) MemberObservation {
	return MemberObservation{Name: name, MC: mc, Healthy: true}
}

func failingMember(name, mc string) MemberObservation {
	return MemberObservation{Name: name, MC: mc}
}

func TestDecideRecoveryScenarioTable(t *testing.T) {
	RegisterTestingT(t)
	fleet := []string{"mc-a", "mc-b", "mc-c"}

	//nolint:govet // fieldalignment: table columns read left to right
	tests := []struct {
		name   string
		state  RecoveryState
		action RecoveryAction
		target string
		reason string
	}{
		{
			name: "healthy",
			state: RecoveryState{FleetMCs: fleet, Members: []MemberObservation{
				healthyMember("m0", "mc-a"),
				healthyMember("m1", "mc-b"),
				healthyMember("m2", "mc-c"),
			}},
			action: RecoveryNone,
			reason: "Healthy",
		},
		{
			name: "one-failing",
			state: RecoveryState{FleetMCs: fleet, Members: []MemberObservation{
				healthyMember("m0", "mc-a"),
				failingMember("m1", "mc-b"),
				healthyMember("m2", "mc-c"),
			}},
			action: RecoveryReplace,
			target: "m1",
			reason: "ReplaceMember",
		},
		{
			name: "two-failing",
			state: RecoveryState{FleetMCs: fleet, Members: []MemberObservation{
				healthyMember("m0", "mc-a"),
				failingMember("m1", "mc-b"),
				failingMember("m2", "mc-c"),
			}},
			action: RecoveryBlocked,
			reason: "NoQuorum",
		},
		{
			name: "rollout-in-progress",
			state: RecoveryState{FleetMCs: fleet, RolloutInProgress: true,
				Members: []MemberObservation{
					healthyMember("m0", "mc-a"),
					healthyMember("m1", "mc-b"),
					healthyMember("m2", "mc-c"),
				}},
			action: RecoveryWait,
			reason: "RolloutInProgress",
		},
		{
			name: "MC-lost",
			state: RecoveryState{
				FleetMCs: []string{"mc-a", "mc-b", "mc-d"},
				Members: []MemberObservation{
					healthyMember("m0", "mc-a"),
					healthyMember("m1", "mc-b"),
					healthyMember("m2", "mc-c"),
				}},
			action: RecoveryRepairMCLoss,
			target: "m2",
			reason: "MCLost",
		},
		{
			name: "MC-lost without a spare MC",
			state: RecoveryState{
				FleetMCs: []string{"mc-a", "mc-b"},
				Members: []MemberObservation{
					healthyMember("m0", "mc-a"),
					healthyMember("m1", "mc-b"),
					healthyMember("m2", "mc-c"),
				}},
			action: RecoveryBlocked,
			reason: "NoReplacementMC",
		},
		{
			name: "learner-mid-promotion",
			state: RecoveryState{
				FleetMCs: []string{"mc-a", "mc-b", "mc-c", "mc-d"},
				Members: []MemberObservation{
					healthyMember("m0", "mc-a"),
					healthyMember("m1", "mc-b"),
					healthyMember("m2", "mc-c"),
					{Name: "m3", MC: "mc-d", Healthy: true, Learner: true, CaughtUp: true},
				}},
			action: RecoveryPromote,
			target: "m3",
			reason: "LearnerCaughtUp",
		},
		{
			name: "learner-still-catching-up",
			state: RecoveryState{
				FleetMCs: []string{"mc-a", "mc-b", "mc-c", "mc-d"},
				Members: []MemberObservation{
					healthyMember("m0", "mc-a"),
					healthyMember("m1", "mc-b"),
					healthyMember("m2", "mc-c"),
					{Name: "m3", MC: "mc-d", Healthy: true, Learner: true},
				}},
			action: RecoveryWait,
			target: "m3",
			reason: "LearnerCatchingUp",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			RegisterTestingT(t)
			decision := DecideRecovery(test.state)

			Expect(decision.Action).To(Equal(test.action))
			Expect(decision.Target).To(Equal(test.target))
			Expect(decision.Reason).To(Equal(test.reason))
		})
	}
}

func TestDecideRecoveryRefusesTwoFailuresEvenWithQuorum(t *testing.T) {
	RegisterTestingT(t)
	members := []MemberObservation{
		healthyMember("m0", "mc-a"),
		healthyMember("m1", "mc-b"),
		healthyMember("m2", "mc-c"),
		failingMember("m3", "mc-d"),
		failingMember("m4", "mc-e"),
	}

	decision := DecideRecovery(RecoveryState{
		Members:  members,
		FleetMCs: []string{"mc-a", "mc-b", "mc-c", "mc-d", "mc-e"},
	})

	Expect(decision.Action).To(Equal(RecoveryBlocked))
	Expect(decision.Reason).To(Equal("MultipleFailures"))
}

func TestRecoveryReportMarksRefusalUnavailable(t *testing.T) {
	RegisterTestingT(t)
	cluster := ClusterView{Generation: 7}

	report := RecoveryReport(cluster, DecideRecovery(RecoveryState{
		Members: []MemberObservation{
			healthyMember("m0", "mc-a"),
			failingMember("m1", "mc-b"),
			failingMember("m2", "mc-c"),
		},
	}))

	Expect(report.ObservedGeneration).To(Equal(int32(7)))
	Expect(conditionStatus(report, "Available")).To(Equal(api.AdapterConditionFalse))
	Expect(conditionStatus(report, "Health")).To(Equal(api.AdapterConditionFalse))
	Expect(*report.Conditions[0].Reason).To(Equal("NoQuorum"))
}
