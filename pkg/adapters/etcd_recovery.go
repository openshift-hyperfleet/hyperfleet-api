package adapters

import (
	"sort"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

// RecoveryAction is the membership surgery the adapter is allowed to perform.
type RecoveryAction string

const (
	RecoveryNone         RecoveryAction = "None"
	RecoveryReplace      RecoveryAction = "Replace"
	RecoveryPromote      RecoveryAction = "Promote"
	RecoveryRepairMCLoss RecoveryAction = "RepairMCLoss"
	RecoveryWait         RecoveryAction = "Wait"
	RecoveryBlocked      RecoveryAction = "Blocked"
)

// MemberObservation is one member's live health and placement.
//
//nolint:govet // fieldalignment: grouping beats padding for readability
type MemberObservation struct {
	Name     string
	MC       string
	Healthy  bool
	Learner  bool
	CaughtUp bool
}

// RecoveryState is everything the recovery decision needs to see.
//
//nolint:govet // fieldalignment: grouping beats padding for readability
type RecoveryState struct {
	Members           []MemberObservation
	FleetMCs          []string
	RolloutInProgress bool
}

// RecoveryDecision is either an allowed action or a refusal with a reason.
type RecoveryDecision struct {
	Action  RecoveryAction
	Target  string
	Reason  string
	Message string
}

// DecideRecovery applies the safety rules to one observation. A nil FleetMCs
// means the fleet was not checked; an empty non-nil slice means no MCs remain.
func DecideRecovery(state RecoveryState) RecoveryDecision {
	if state.RolloutInProgress {
		return refusal(RecoveryWait, "RolloutInProgress",
			"Etcd member rollout is not settled; refusing membership changes")
	}
	if !state.quorumPresent() {
		return refusal(RecoveryBlocked, "NoQuorum",
			"Etcd quorum is not present; membership changes need a human")
	}

	lost := state.lostMembers()
	failing := state.failingMembers()
	otherFailures := withoutLost(failing, lost)

	if len(lost) > 0 {
		if len(lost) != 1 || len(otherFailures) > 0 {
			return refusal(RecoveryBlocked, "MultipleFailures",
				"More than one etcd member is failing; refusing automatic repair")
		}
		if _, ok := state.replacementMC(lost[0]); !ok {
			return refusal(RecoveryBlocked, "NoReplacementMC",
				"No healthy management cluster is available for the replacement")
		}
		return action(RecoveryRepairMCLoss, lost[0].Name, "MCLost",
			"The management cluster hosting this member is gone; repairing topology")
	}

	learners := state.learners()
	switch {
	case len(learners) > 1:
		return refusal(RecoveryWait, "MultipleLearners",
			"More than one etcd learner is present; waiting for a single promotion")
	case len(learners) == 1 && len(failing) == 0 && learners[0].CaughtUp:
		return action(RecoveryPromote, learners[0].Name, "LearnerCaughtUp",
			"The etcd learner is caught up and can be promoted")
	case len(learners) == 1 && len(failing) == 0:
		return RecoveryDecision{
			Action: RecoveryWait, Target: learners[0].Name,
			Reason: "LearnerCatchingUp", Message: "The etcd learner is still catching up",
		}
	}

	switch len(failing) {
	case 0:
		return action(RecoveryNone, "", "Healthy", "Etcd membership is healthy")
	case 1:
		return action(RecoveryReplace, failing[0].Name, "ReplaceMember",
			"Exactly one etcd member is failing; replacing it")
	default:
		return refusal(RecoveryBlocked, "MultipleFailures",
			"More than one etcd member is failing; refusing automatic repair")
	}
}

func action(action RecoveryAction, target, reason, message string) RecoveryDecision {
	return RecoveryDecision{Action: action, Target: target, Reason: reason, Message: message}
}

func refusal(action RecoveryAction, reason, message string) RecoveryDecision {
	return RecoveryDecision{Action: action, Reason: reason, Message: message}
}

func (s RecoveryState) quorumPresent() bool {
	voters, healthy := 0, 0
	for _, member := range s.Members {
		if member.Learner {
			continue
		}
		voters++
		if member.Healthy {
			healthy++
		}
	}
	return voters > 0 && healthy >= voters/2+1
}

func (s RecoveryState) lostMembers() []MemberObservation {
	if s.FleetMCs == nil {
		return nil
	}
	fleet := make(map[string]bool, len(s.FleetMCs))
	for _, mc := range s.FleetMCs {
		fleet[mc] = true
	}
	lost := make([]MemberObservation, 0, len(s.Members))
	for _, member := range s.Members {
		if !fleet[member.MC] {
			lost = append(lost, member)
		}
	}
	return lost
}

func (s RecoveryState) failingMembers() []MemberObservation {
	failing := make([]MemberObservation, 0, len(s.Members))
	for _, member := range s.Members {
		if !member.Healthy {
			failing = append(failing, member)
		}
	}
	return failing
}

func (s RecoveryState) learners() []MemberObservation {
	learners := make([]MemberObservation, 0, len(s.Members))
	for _, member := range s.Members {
		if member.Learner {
			learners = append(learners, member)
		}
	}
	return learners
}

func (s RecoveryState) replacementMC(dead MemberObservation) (string, bool) {
	used := make(map[string]bool, len(s.Members))
	for _, member := range s.Members {
		used[member.MC] = true
	}
	candidates := make([]string, 0, len(s.FleetMCs))
	for _, mc := range s.FleetMCs {
		if mc != dead.MC && !used[mc] {
			candidates = append(candidates, mc)
		}
	}
	if len(candidates) == 0 {
		return "", false
	}
	sort.Strings(candidates)
	return candidates[0], true
}

func withoutLost(
	failing, lost []MemberObservation,
) []MemberObservation {
	lostNames := make(map[string]bool, len(lost))
	for _, member := range lost {
		lostNames[member.Name] = true
	}
	remaining := make([]MemberObservation, 0, len(failing))
	for _, member := range failing {
		if !lostNames[member.Name] {
			remaining = append(remaining, member)
		}
	}
	return remaining
}

// RecoveryReport publishes the decision as adapter conditions.
func RecoveryReport(cluster ClusterView, decision RecoveryDecision) StatusReport {
	available, applied, healthy := api.AdapterConditionTrue,
		api.AdapterConditionTrue, api.AdapterConditionTrue
	switch decision.Action {
	case RecoveryNone:
	case RecoveryPromote, RecoveryReplace, RecoveryRepairMCLoss:
		applied = api.AdapterConditionFalse
		healthy = api.AdapterConditionFalse
	case RecoveryWait:
		applied = api.AdapterConditionFalse
		healthy = api.AdapterConditionFalse
	case RecoveryBlocked:
		available = api.AdapterConditionFalse
		applied = api.AdapterConditionFalse
		healthy = api.AdapterConditionFalse
	}
	return StatusReport{
		ObservedGeneration: cluster.Generation,
		Conditions: []api.AdapterCondition{
			condition(api.AdapterConditionTypeAvailable, available, decision.Reason, decision.Message),
			condition(api.AdapterConditionTypeApplied, applied, decision.Reason, decision.Message),
			condition(api.AdapterConditionTypeHealth, healthy, decision.Reason, decision.Message),
		},
	}
}
