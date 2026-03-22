package services

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

// Fixed time fixtures — all tests use these so assertions are deterministic.
var (
	aggT0   = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	aggT1   = aggT0.Add(1 * time.Minute)
	aggT2   = aggT0.Add(2 * time.Minute)
	aggT3   = aggT0.Add(3 * time.Minute)
	aggTRef = aggT0.Add(10 * time.Minute)
)

// marshalConds encodes AdapterConditions to JSON (panics on error — test helper only).
func marshalConds(conds []api.AdapterCondition) []byte {
	b, err := json.Marshal(conds)
	if err != nil {
		panic(err)
	}
	return b
}

// availConds returns minimal conditions JSON with the given Available status.
func availConds(status api.AdapterConditionStatus) []byte {
	return marshalConds([]api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: status},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue},
	})
}

// makeAdapterStatus builds a minimal *api.AdapterStatus for normalise tests.
func makeAdapterStatus(adapter string, lastReport time.Time, obsGen int32, conds []byte) *api.AdapterStatus {
	return &api.AdapterStatus{
		Adapter:            adapter,
		LastReportTime:     lastReport,
		ObservedGeneration: obsGen,
		Conditions:         conds,
	}
}

// mkPrevReady builds a Ready ResourceCondition for use as a prev fixture.
func mkPrevReady(
	status api.ResourceConditionStatus, obsGen int32, lastTransition, lastUpdated time.Time,
) *api.ResourceCondition {
	return &api.ResourceCondition{
		Type:               api.ConditionTypeReady,
		Status:             status,
		ObservedGeneration: obsGen,
		LastTransitionTime: lastTransition,
		LastUpdatedTime:    lastUpdated,
		CreatedTime:        aggT0,
	}
}

// mkPrevAvail builds an Available ResourceCondition for use as a prev fixture.
func mkPrevAvail(
	status api.ResourceConditionStatus, obsGen int32, lastTransition, lastUpdated time.Time,
) *api.ResourceCondition {
	return &api.ResourceCondition{
		Type:               api.ConditionTypeAvailable,
		Status:             status,
		ObservedGeneration: obsGen,
		LastTransitionTime: lastTransition,
		LastUpdatedTime:    lastUpdated,
		CreatedTime:        aggT0,
	}
}

// snap builds an adapterAvailableSnapshot inline.
func snap(gen int32, avail bool, obsTime time.Time) adapterAvailableSnapshot {
	return adapterAvailableSnapshot{observedGeneration: gen, availableTrue: avail, observedTime: obsTime}
}

// ---------------------------------------------------------------------------
// parsePrevConditions
// ---------------------------------------------------------------------------

func TestParsePrevConditions(t *testing.T) {
	t.Parallel()
	encode := func(conds ...api.ResourceCondition) []byte {
		b, _ := json.Marshal(conds)
		return b
	}
	readyCond := api.ResourceCondition{Type: api.ConditionTypeReady, Status: api.ConditionTrue}
	availCond := api.ResourceCondition{Type: api.ConditionTypeAvailable, Status: api.ConditionFalse}
	adapterCond := api.ResourceCondition{Type: "Adapter1Successful", Status: api.ConditionTrue}

	t.Run("nil input", func(t *testing.T) {
		t.Parallel()
		r, a, m := parsePrevConditions(nil)
		if r != nil || a != nil || len(m) != 0 {
			t.Fatalf("expected (nil,nil,empty), got (%v,%v,%v)", r, a, m)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		t.Parallel()
		r, a, m := parsePrevConditions([]byte{})
		if r != nil || a != nil || len(m) != 0 {
			t.Fatalf("expected (nil,nil,empty), got (%v,%v,%v)", r, a, m)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()
		r, a, m := parsePrevConditions([]byte("not-json"))
		if r != nil || a != nil || len(m) != 0 {
			t.Fatalf("expected (nil,nil,empty) on bad JSON, got (%v,%v,%v)", r, a, m)
		}
	})

	t.Run("both Ready and Available", func(t *testing.T) {
		t.Parallel()
		r, a, _ := parsePrevConditions(encode(readyCond, availCond))
		if r == nil || r.Type != api.ConditionTypeReady {
			t.Fatalf("expected Ready condition, got %v", r)
		}
		if a == nil || a.Type != api.ConditionTypeAvailable {
			t.Fatalf("expected Available condition, got %v", a)
		}
	})

	t.Run("only Ready", func(t *testing.T) {
		t.Parallel()
		r, a, _ := parsePrevConditions(encode(readyCond))
		if r == nil || r.Type != api.ConditionTypeReady {
			t.Fatalf("expected Ready, got %v", r)
		}
		if a != nil {
			t.Fatalf("expected nil Available, got %v", a)
		}
	})

	t.Run("only Available", func(t *testing.T) {
		t.Parallel()
		r, a, _ := parsePrevConditions(encode(availCond))
		if r != nil {
			t.Fatalf("expected nil Ready, got %v", r)
		}
		if a == nil || a.Type != api.ConditionTypeAvailable {
			t.Fatalf("expected Available, got %v", a)
		}
	})

	t.Run("per-adapter condition is placed in map", func(t *testing.T) {
		t.Parallel()
		_, _, m := parsePrevConditions(encode(readyCond, availCond, adapterCond))
		prev, ok := m["Adapter1Successful"]
		if !ok || prev == nil || prev.Type != "Adapter1Successful" {
			t.Fatalf("expected Adapter1Successful in map, got %v", m)
		}
	})

	t.Run("unknown condition types go into the adapter map", func(t *testing.T) {
		t.Parallel()
		other := api.ResourceCondition{Type: "CustomType", Status: api.ConditionTrue}
		r, a, m := parsePrevConditions(encode(other))
		if r != nil || a != nil {
			t.Fatalf("expected (nil,nil) for synthetic conditions, got (%v,%v)", r, a)
		}
		if _, ok := m["CustomType"]; !ok {
			t.Fatal("expected CustomType in adapter map")
		}
	})
}

// ---------------------------------------------------------------------------
// normalizeAdapterReportsForAggregation
// ---------------------------------------------------------------------------

func TestNormalizeAdapterReportsForAggregation(t *testing.T) {
	t.Parallel()
	required := []string{"alpha", "beta"}
	resourceGen := int32(2)

	t.Run("adapter not in required set is skipped", func(t *testing.T) {
		t.Parallel()
		list := api.AdapterStatusList{
			makeAdapterStatus("gamma", aggT1, 2, availConds(api.AdapterConditionTrue)),
		}
		out := normalizeAdapterReportsForAggregation(list, required, resourceGen)
		if len(out) != 0 {
			t.Fatalf("expected empty map, got %v", out)
		}
	})

	t.Run("observedGeneration greater than resourceGen is skipped", func(t *testing.T) {
		t.Parallel()
		list := api.AdapterStatusList{
			makeAdapterStatus("alpha", aggT1, resourceGen+1, availConds(api.AdapterConditionTrue)),
		}
		out := normalizeAdapterReportsForAggregation(list, required, resourceGen)
		if _, ok := out["alpha"]; ok {
			t.Fatal("expected alpha to be skipped: observedGeneration > resourceGen")
		}
	})

	t.Run("observedGeneration equal to resourceGen is included", func(t *testing.T) {
		t.Parallel()
		list := api.AdapterStatusList{
			makeAdapterStatus("alpha", aggT1, resourceGen, availConds(api.AdapterConditionTrue)),
		}
		out := normalizeAdapterReportsForAggregation(list, required, resourceGen)
		if _, ok := out["alpha"]; !ok {
			t.Fatal("expected alpha in output when observedGeneration == resourceGen")
		}
	})

	t.Run("invalid conditions JSON is skipped", func(t *testing.T) {
		t.Parallel()
		list := api.AdapterStatusList{
			makeAdapterStatus("alpha", aggT1, 2, []byte("bad-json")),
		}
		out := normalizeAdapterReportsForAggregation(list, required, resourceGen)
		if _, ok := out["alpha"]; ok {
			t.Fatal("expected alpha to be skipped on bad conditions JSON")
		}
	})

	t.Run("missing Available condition is skipped", func(t *testing.T) {
		t.Parallel()
		conds := marshalConds([]api.AdapterCondition{
			{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue},
			{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue},
		})
		list := api.AdapterStatusList{makeAdapterStatus("alpha", aggT1, 2, conds)}
		out := normalizeAdapterReportsForAggregation(list, required, resourceGen)
		if _, ok := out["alpha"]; ok {
			t.Fatal("expected alpha to be skipped: no Available condition")
		}
	})

	t.Run("Available=Unknown is skipped", func(t *testing.T) {
		t.Parallel()
		list := api.AdapterStatusList{
			makeAdapterStatus("alpha", aggT1, 2, availConds(api.AdapterConditionUnknown)),
		}
		out := normalizeAdapterReportsForAggregation(list, required, resourceGen)
		if _, ok := out["alpha"]; ok {
			t.Fatal("expected alpha to be skipped: Available=Unknown is not True or False")
		}
	})

	t.Run("Available=True yields availableTrue=true", func(t *testing.T) {
		t.Parallel()
		list := api.AdapterStatusList{
			makeAdapterStatus("alpha", aggT1, 2, availConds(api.AdapterConditionTrue)),
		}
		out := normalizeAdapterReportsForAggregation(list, required, resourceGen)
		s, ok := out["alpha"]
		if !ok {
			t.Fatal("expected alpha in output")
		}
		if !s.availableTrue {
			t.Error("expected availableTrue=true")
		}
		if s.observedGeneration != 2 {
			t.Errorf("observedGeneration got %d, want 2", s.observedGeneration)
		}
		if !s.observedTime.Equal(aggT1) {
			t.Errorf("observedTime got %v, want %v", s.observedTime, aggT1)
		}
	})

	t.Run("Available=False yields availableTrue=false", func(t *testing.T) {
		t.Parallel()
		list := api.AdapterStatusList{
			makeAdapterStatus("beta", aggT2, 1, availConds(api.AdapterConditionFalse)),
		}
		out := normalizeAdapterReportsForAggregation(list, required, resourceGen)
		s, ok := out["beta"]
		if !ok {
			t.Fatal("expected beta in output")
		}
		if s.availableTrue {
			t.Error("expected availableTrue=false")
		}
	})

	t.Run("mixed list: only valid adapters land in output", func(t *testing.T) {
		t.Parallel()
		list := api.AdapterStatusList{
			makeAdapterStatus("alpha", aggT1, 2, availConds(api.AdapterConditionTrue)),
			// gen too high → skip
			makeAdapterStatus("beta", aggT1, resourceGen+1, availConds(api.AdapterConditionTrue)),
			// not required → skip
			makeAdapterStatus("gamma", aggT1, 2, availConds(api.AdapterConditionTrue)),
		}
		out := normalizeAdapterReportsForAggregation(list, required, resourceGen)
		if len(out) != 1 {
			t.Fatalf("expected 1 adapter in output, got %d: %v", len(out), out)
		}
		if _, ok := out["alpha"]; !ok {
			t.Fatal("expected alpha in output")
		}
	})
}

// ---------------------------------------------------------------------------
// sameGenerationAllTrue
// ---------------------------------------------------------------------------

func TestSameGenerationAllTrue(t *testing.T) {
	t.Parallel()
	t.Run("empty required returns (true, 1, false)", func(t *testing.T) {
		t.Parallel()
		allTrue, gen, mixed := sameGenerationAllTrue(nil, map[string]adapterAvailableSnapshot{})
		if !allTrue || gen != 1 || mixed {
			t.Fatalf("got (%v,%d,%v), want (true,1,false)", allTrue, gen, mixed)
		}
	})

	t.Run("single adapter True", func(t *testing.T) {
		t.Parallel()
		byAdapter := map[string]adapterAvailableSnapshot{"a": snap(3, true, aggT1)}
		allTrue, gen, mixed := sameGenerationAllTrue([]string{"a"}, byAdapter)
		if !allTrue || gen != 3 || mixed {
			t.Fatalf("got (%v,%d,%v), want (true,3,false)", allTrue, gen, mixed)
		}
	})

	t.Run("two adapters True same generation", func(t *testing.T) {
		t.Parallel()
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(2, true, aggT1),
			"b": snap(2, true, aggT2),
		}
		allTrue, gen, mixed := sameGenerationAllTrue([]string{"a", "b"}, byAdapter)
		if !allTrue || gen != 2 || mixed {
			t.Fatalf("got (%v,%d,%v), want (true,2,false)", allTrue, gen, mixed)
		}
	})

	t.Run("two adapters True different generations returns mixed=true", func(t *testing.T) {
		t.Parallel()
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(1, true, aggT1),
			"b": snap(2, true, aggT2),
		}
		allTrue, _, mixed := sameGenerationAllTrue([]string{"a", "b"}, byAdapter)
		if !allTrue || !mixed {
			t.Fatalf("expected (true, _, true), got (%v,_,%v)", allTrue, mixed)
		}
	})

	t.Run("one adapter False returns (false, 0, false)", func(t *testing.T) {
		t.Parallel()
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(2, true, aggT1),
			"b": snap(2, false, aggT2),
		}
		allTrue, gen, mixed := sameGenerationAllTrue([]string{"a", "b"}, byAdapter)
		if allTrue || gen != 0 || mixed {
			t.Fatalf("got (%v,%d,%v), want (false,0,false)", allTrue, gen, mixed)
		}
	})

	t.Run("missing adapter returns (false, 0, false)", func(t *testing.T) {
		t.Parallel()
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(2, true, aggT1),
			// "b" absent
		}
		allTrue, gen, mixed := sameGenerationAllTrue([]string{"a", "b"}, byAdapter)
		if allTrue || gen != 0 || mixed {
			t.Fatalf("got (%v,%d,%v), want (false,0,false)", allTrue, gen, mixed)
		}
	})
}

// ---------------------------------------------------------------------------
// computeReadyLastTransitionTime
// ---------------------------------------------------------------------------

func TestComputeReadyLastTransitionTime(t *testing.T) {
	t.Parallel()
	t.Run("nil prev returns newLastUpdated", func(t *testing.T) {
		t.Parallel()
		got := computeReadyLastTransitionTime(2, aggTRef, nil, api.ConditionFalse, aggT1)
		if !got.Equal(aggT1) {
			t.Errorf("got %v, want %v", got, aggT1)
		}
	})

	t.Run("same status returns prev.LastTransitionTime", func(t *testing.T) {
		t.Parallel()
		prev := mkPrevReady(api.ConditionFalse, 1, aggT0, aggT1)
		got := computeReadyLastTransitionTime(2, aggTRef, prev, api.ConditionFalse, aggT2)
		if !got.Equal(aggT0) {
			t.Errorf("got %v, want prev.LastTransitionTime=%v", got, aggT0)
		}
	})

	t.Run("True→False with generation bump uses refTime", func(t *testing.T) {
		t.Parallel()
		// resourceGen(2) > prev.ObservedGeneration(1): generation-bump branch
		prev := mkPrevReady(api.ConditionTrue, 1, aggT0, aggT1)
		got := computeReadyLastTransitionTime(2, aggTRef, prev, api.ConditionFalse, aggT2)
		if !got.Equal(aggTRef) {
			t.Errorf("got %v, want refTime=%v", got, aggTRef)
		}
	})

	t.Run("True→False without generation bump uses newLastUpdated", func(t *testing.T) {
		t.Parallel()
		// resourceGen == prev.ObservedGeneration → not a gen bump
		prev := mkPrevReady(api.ConditionTrue, 2, aggT0, aggT1)
		got := computeReadyLastTransitionTime(2, aggTRef, prev, api.ConditionFalse, aggT2)
		if !got.Equal(aggT2) {
			t.Errorf("got %v, want newLastUpdated=%v", got, aggT2)
		}
	})

	t.Run("False→True uses newLastUpdated", func(t *testing.T) {
		t.Parallel()
		prev := mkPrevReady(api.ConditionFalse, 1, aggT0, aggT1)
		got := computeReadyLastTransitionTime(2, aggTRef, prev, api.ConditionTrue, aggT2)
		if !got.Equal(aggT2) {
			t.Errorf("got %v, want newLastUpdated=%v", got, aggT2)
		}
	})
}

// ---------------------------------------------------------------------------
// computeReady
// ---------------------------------------------------------------------------

func TestComputeReady(t *testing.T) {
	t.Parallel()
	t.Run("empty required list → False", func(t *testing.T) {
		t.Parallel()
		cond := computeReady(1, aggTRef, nil, nil, map[string]adapterAvailableSnapshot{})
		if cond.Status != api.ConditionFalse {
			t.Errorf("got %v, want False", cond.Status)
		}
		if cond.Type != api.ConditionTypeReady {
			t.Errorf("type got %v, want Ready", cond.Type)
		}
	})

	t.Run("all adapters at current gen and True → True", func(t *testing.T) {
		t.Parallel()
		required := []string{"a", "b"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(2, true, aggT1),
			"b": snap(2, true, aggT2),
		}
		cond := computeReady(2, aggTRef, nil, required, byAdapter)
		if cond.Status != api.ConditionTrue {
			t.Errorf("got %v, want True", cond.Status)
		}
		// LastUpdatedTime = min(t1,t2) = t1
		if !cond.LastUpdatedTime.Equal(aggT1) {
			t.Errorf("LastUpdatedTime got %v, want %v", cond.LastUpdatedTime, aggT1)
		}
	})

	t.Run("adapter missing from byAdapter → False", func(t *testing.T) {
		t.Parallel()
		required := []string{"a", "b"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(2, true, aggT1),
		}
		cond := computeReady(2, aggTRef, nil, required, byAdapter)
		if cond.Status != api.ConditionFalse {
			t.Errorf("got %v, want False", cond.Status)
		}
	})

	t.Run("adapter at old generation → False", func(t *testing.T) {
		t.Parallel()
		required := []string{"a", "b"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(2, true, aggT1),
			"b": snap(1, true, aggT2), // gen 1 ≠ current gen 2
		}
		cond := computeReady(2, aggTRef, nil, required, byAdapter)
		if cond.Status != api.ConditionFalse {
			t.Errorf("got %v, want False", cond.Status)
		}
	})

	t.Run("adapter availableTrue=false → False", func(t *testing.T) {
		t.Parallel()
		required := []string{"a", "b"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(2, true, aggT1),
			"b": snap(2, false, aggT2),
		}
		cond := computeReady(2, aggTRef, nil, required, byAdapter)
		if cond.Status != api.ConditionFalse {
			t.Errorf("got %v, want False", cond.Status)
		}
	})

	t.Run("False status → LastUpdatedTime is refTime", func(t *testing.T) {
		t.Parallel()
		required := []string{"a"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(1, true, aggT1), // old gen
		}
		cond := computeReady(2, aggTRef, nil, required, byAdapter)
		if !cond.LastUpdatedTime.Equal(aggTRef) {
			t.Errorf("LastUpdatedTime got %v, want refTime=%v", cond.LastUpdatedTime, aggTRef)
		}
	})

	t.Run("CreatedTime carried from prev", func(t *testing.T) {
		t.Parallel()
		required := []string{"a"}
		byAdapter := map[string]adapterAvailableSnapshot{"a": snap(2, true, aggT1)}
		prev := mkPrevReady(api.ConditionTrue, 1, aggT0, aggT0)
		cond := computeReady(2, aggTRef, prev, required, byAdapter)
		if !cond.CreatedTime.Equal(aggT0) {
			t.Errorf("CreatedTime got %v, want prev.CreatedTime=%v", cond.CreatedTime, aggT0)
		}
	})

	t.Run("some adapters at current gen (Ready=False) → LastUpdatedTime is refTime", func(t *testing.T) {
		t.Parallel()
		// "a" is at current gen 2, "b" is still at gen 1 → not all at current gen → False.
		// Doc: when no/partial adapters at current generation, last_updated_time = resource.last_updated_time.
		required := []string{"a", "b"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(2, true, aggT1),
			"b": snap(1, true, aggT2),
		}
		cond := computeReady(2, aggTRef, nil, required, byAdapter)
		if cond.Status != api.ConditionFalse {
			t.Errorf("Status got %v, want False", cond.Status)
		}
		if !cond.LastUpdatedTime.Equal(aggTRef) {
			t.Errorf("LastUpdatedTime got %v, want refTime=%v", cond.LastUpdatedTime, aggTRef)
		}
	})

	t.Run("ObservedGeneration always equals resourceGen", func(t *testing.T) {
		t.Parallel()
		cond := computeReady(5, aggTRef, nil, nil, map[string]adapterAvailableSnapshot{})
		if cond.ObservedGeneration != 5 {
			t.Errorf("ObservedGeneration got %d, want 5", cond.ObservedGeneration)
		}
	})
}

// ---------------------------------------------------------------------------
// computeAvailableLastUpdatedTime
// ---------------------------------------------------------------------------

func TestComputeAvailableLastUpdatedTime(t *testing.T) {
	t.Parallel()
	t.Run("empty required → refTime", func(t *testing.T) {
		t.Parallel()
		got := computeAvailableLastUpdatedTime(api.ConditionFalse, nil, aggTRef, nil, nil, 1, true, false)
		if !got.Equal(aggTRef) {
			t.Errorf("got %v, want refTime=%v", got, aggTRef)
		}
	})

	t.Run("allTrue same generation → min observed time", func(t *testing.T) {
		t.Parallel()
		required := []string{"a", "b"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(2, true, aggT3), // later
			"b": snap(2, true, aggT1), // earlier → min
		}
		got := computeAvailableLastUpdatedTime(api.ConditionTrue, nil, aggTRef, required, byAdapter, 2, true, false)
		if !got.Equal(aggT1) {
			t.Errorf("got %v, want min=%v", got, aggT1)
		}
	})

	t.Run("allTrue mixed gens, status True, prev set → prev.LastUpdatedTime", func(t *testing.T) {
		t.Parallel()
		required := []string{"a", "b"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(1, true, aggT1),
			"b": snap(2, true, aggT2),
		}
		prev := mkPrevAvail(api.ConditionTrue, 1, aggT0, aggT0)
		got := computeAvailableLastUpdatedTime(api.ConditionTrue, prev, aggTRef, required, byAdapter, 1, true, true)
		if !got.Equal(aggT0) {
			t.Errorf("got %v, want prev.LastUpdatedTime=%v", got, aggT0)
		}
	})

	t.Run("allTrue mixed gens, status True, no prev → refTime", func(t *testing.T) {
		t.Parallel()
		required := []string{"a", "b"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(1, true, aggT1),
			"b": snap(2, true, aggT2),
		}
		got := computeAvailableLastUpdatedTime(api.ConditionTrue, nil, aggTRef, required, byAdapter, 1, true, true)
		if !got.Equal(aggTRef) {
			t.Errorf("got %v, want refTime=%v", got, aggTRef)
		}
	})

	// allTrue && mixed && status==False: all adapters are True at different gens but prev was not True,
	// so Available stays False. hasFalseAtX is never set (all availableTrue), falls to fallback.
	t.Run("allTrue mixed gens, status False → fallback to prev.LastUpdatedTime", func(t *testing.T) {
		t.Parallel()
		required := []string{"a", "b"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(1, true, aggT1),
			"b": snap(2, true, aggT2),
		}
		prev := mkPrevAvail(api.ConditionFalse, 1, aggT0, aggT0)
		got := computeAvailableLastUpdatedTime(api.ConditionFalse, prev, aggTRef, required, byAdapter, 2, true, true)
		if !got.Equal(aggT0) {
			t.Errorf("got %v, want prev.LastUpdatedTime=%v", got, aggT0)
		}
	})

	t.Run("status False, False adapter at observedGen → min time of adapters at that gen", func(t *testing.T) {
		t.Parallel()
		required := []string{"a", "b"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(2, false, aggT3), // False at gen 2
			"b": snap(2, true, aggT1),  // True at gen 2 (still included in atX)
		}
		// observedGen=2, hasFalseAtX=true; atX=[t3,t1], min=t1 (oldest, matches Ready semantics)
		got := computeAvailableLastUpdatedTime(api.ConditionFalse, nil, aggTRef, required, byAdapter, 2, false, false)
		if !got.Equal(aggT1) {
			t.Errorf("got %v, want min of gen-2 adapters=%v", got, aggT1)
		}
	})

	t.Run("status False, no False at observedGen → fallback to prev.LastUpdatedTime", func(t *testing.T) {
		t.Parallel()
		// observedGen=3 but adapters are at gen 2 → no False at gen 3 → fallback
		required := []string{"a", "b"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(2, false, aggT1),
			"b": snap(2, false, aggT2),
		}
		prev := mkPrevAvail(api.ConditionFalse, 2, aggT0, aggT0)
		got := computeAvailableLastUpdatedTime(api.ConditionFalse, prev, aggTRef, required, byAdapter, 3, false, false)
		if !got.Equal(aggT0) {
			t.Errorf("got %v, want prev.LastUpdatedTime=%v", got, aggT0)
		}
	})

	t.Run("status False, no False at observedGen, no prev → refTime", func(t *testing.T) {
		t.Parallel()
		// adapter is False at gen 1, observedGen=2 → hasFalseAtX=false (wrong gen), no prev → refTime
		required := []string{"a"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(1, false, aggT1),
		}
		got := computeAvailableLastUpdatedTime(api.ConditionFalse, nil, aggTRef, required, byAdapter, 2, false, false)
		if !got.Equal(aggTRef) {
			t.Errorf("got %v, want refTime=%v", got, aggTRef)
		}
	})
}

// ---------------------------------------------------------------------------
// computeAvailable
// ---------------------------------------------------------------------------

func TestComputeAvailable(t *testing.T) {
	t.Parallel()
	t.Run("empty required list → False", func(t *testing.T) {
		t.Parallel()
		cond := computeAvailable(aggTRef, nil, nil, map[string]adapterAvailableSnapshot{})
		if cond.Status != api.ConditionFalse {
			t.Errorf("got %v, want False", cond.Status)
		}
	})

	t.Run("required adapter missing from byAdapter → False", func(t *testing.T) {
		t.Parallel()
		required := []string{"a", "b"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(1, true, aggT1),
			// "b" absent
		}
		cond := computeAvailable(aggTRef, nil, required, byAdapter)
		if cond.Status != api.ConditionFalse {
			t.Errorf("got %v, want False", cond.Status)
		}
	})

	t.Run("all True same generation → True", func(t *testing.T) {
		t.Parallel()
		required := []string{"a", "b"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(1, true, aggT1),
			"b": snap(1, true, aggT2),
		}
		cond := computeAvailable(aggTRef, nil, required, byAdapter)
		if cond.Status != api.ConditionTrue {
			t.Errorf("got %v, want True", cond.Status)
		}
	})

	t.Run("all True mixed generations, no prev → False", func(t *testing.T) {
		t.Parallel()
		required := []string{"a", "b"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(1, true, aggT1),
			"b": snap(2, true, aggT2),
		}
		cond := computeAvailable(aggTRef, nil, required, byAdapter)
		if cond.Status != api.ConditionFalse {
			t.Errorf("got %v, want False (mixed gens, no prev=True)", cond.Status)
		}
	})

	t.Run("all True mixed generations, prev True → True (sticky)", func(t *testing.T) {
		t.Parallel()
		required := []string{"a", "b"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(1, true, aggT1),
			"b": snap(2, true, aggT2),
		}
		prev := mkPrevAvail(api.ConditionTrue, 1, aggT0, aggT0)
		cond := computeAvailable(aggTRef, prev, required, byAdapter)
		if cond.Status != api.ConditionTrue {
			t.Errorf("got %v, want True (sticky from prev=True)", cond.Status)
		}
	})

	t.Run("some adapter False, no prev → False", func(t *testing.T) {
		t.Parallel()
		required := []string{"a", "b"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(1, true, aggT1),
			"b": snap(1, false, aggT2),
		}
		cond := computeAvailable(aggTRef, nil, required, byAdapter)
		if cond.Status != api.ConditionFalse {
			t.Errorf("got %v, want False", cond.Status)
		}
	})

	t.Run("some adapter False at tracked generation, prev True → False (breaks sticky)", func(t *testing.T) {
		t.Parallel()
		// prev tracked gen=1; adapter b is False at gen 1 → falseAtTracked=true → no sticky
		required := []string{"a", "b"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(1, true, aggT1),
			"b": snap(1, false, aggT2),
		}
		prev := mkPrevAvail(api.ConditionTrue, 1, aggT0, aggT0)
		cond := computeAvailable(aggTRef, prev, required, byAdapter)
		if cond.Status != api.ConditionFalse {
			t.Errorf("got %v, want False (False at tracked gen breaks sticky)", cond.Status)
		}
	})

	t.Run("some adapter False at NEW generation, prev True → True (sticky)", func(t *testing.T) {
		t.Parallel()
		// prev tracked gen=1; adapter b is False at gen 2 (new gen) → falseAtTracked=false → sticky
		required := []string{"a", "b"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(1, true, aggT1),
			"b": snap(2, false, aggT2), // False at gen 2 ≠ tracked gen 1
		}
		prev := mkPrevAvail(api.ConditionTrue, 1, aggT0, aggT0)
		cond := computeAvailable(aggTRef, prev, required, byAdapter)
		if cond.Status != api.ConditionTrue {
			t.Errorf("got %v, want True (False not at tracked gen → stays sticky)", cond.Status)
		}
	})

	t.Run("ObservedGeneration for True all-same-gen equals that gen", func(t *testing.T) {
		t.Parallel()
		required := []string{"a"}
		byAdapter := map[string]adapterAvailableSnapshot{"a": snap(3, true, aggT1)}
		cond := computeAvailable(aggTRef, nil, required, byAdapter)
		if cond.ObservedGeneration != 3 {
			t.Errorf("ObservedGeneration got %d, want 3", cond.ObservedGeneration)
		}
	})

	t.Run("False with mixed gens: observed_generation is max adapter gen", func(t *testing.T) {
		t.Parallel()
		// "a" False at gen 1, "b" True at gen 2 → allTrue=false → False.
		// Doc: Available=False → observed_generation = max(adapter observed_generations).
		required := []string{"a", "b"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(1, false, aggT1),
			"b": snap(2, true, aggT2),
		}
		cond := computeAvailable(aggTRef, nil, required, byAdapter)
		if cond.Status != api.ConditionFalse {
			t.Errorf("Status got %v, want False", cond.Status)
		}
		if cond.ObservedGeneration != 2 {
			t.Errorf("ObservedGeneration got %d, want 2 (max of reported gens)", cond.ObservedGeneration)
		}
	})

	t.Run("True sticky mixed gens: observed_generation kept from prev", func(t *testing.T) {
		t.Parallel()
		// All adapters True but at different gens; prev=True at gen 1 → sticky True.
		// Doc: Available=True with mixed gens → observed_generation remains at its current (prev) value.
		required := []string{"a", "b"}
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": snap(1, true, aggT1),
			"b": snap(2, true, aggT2),
		}
		prev := mkPrevAvail(api.ConditionTrue, 1, aggT0, aggT0)
		cond := computeAvailable(aggTRef, prev, required, byAdapter)
		if cond.Status != api.ConditionTrue {
			t.Errorf("Status got %v, want True", cond.Status)
		}
		if cond.ObservedGeneration != 1 {
			t.Errorf("ObservedGeneration got %d, want 1 (prev.ObservedGeneration, unchanged)", cond.ObservedGeneration)
		}
	})

	t.Run("CreatedTime carried from prev", func(t *testing.T) {
		t.Parallel()
		required := []string{"a"}
		byAdapter := map[string]adapterAvailableSnapshot{"a": snap(1, true, aggT1)}
		prev := mkPrevAvail(api.ConditionTrue, 1, aggT0, aggT0)
		cond := computeAvailable(aggTRef, prev, required, byAdapter)
		if !cond.CreatedTime.Equal(aggT0) {
			t.Errorf("CreatedTime got %v, want %v", cond.CreatedTime, aggT0)
		}
	})

	t.Run("LastTransitionTime unchanged when status same as prev", func(t *testing.T) {
		t.Parallel()
		required := []string{"a"}
		byAdapter := map[string]adapterAvailableSnapshot{"a": snap(1, true, aggT1)}
		prev := mkPrevAvail(api.ConditionTrue, 1, aggT0, aggT0)
		cond := computeAvailable(aggTRef, prev, required, byAdapter)
		if !cond.LastTransitionTime.Equal(aggT0) {
			t.Errorf("LastTransitionTime got %v, want prev.LastTransitionTime=%v", cond.LastTransitionTime, aggT0)
		}
	})
}

// ---------------------------------------------------------------------------
// AggregateResourceStatus — integration
// ---------------------------------------------------------------------------

func TestAggregateResourceStatus(t *testing.T) {
	t.Parallel()
	makeStatus := func(
		adapter string, obsGen int32, obsTime time.Time, availStatus api.AdapterConditionStatus,
	) *api.AdapterStatus {
		return &api.AdapterStatus{
			Adapter:            adapter,
			LastReportTime:     obsTime,
			ObservedGeneration: obsGen,
			Conditions:         availConds(availStatus),
		}
	}
	encodePrev := func(conds ...api.ResourceCondition) []byte {
		b, _ := json.Marshal(conds)
		return b
	}

	t.Run("initial creation: req adapters, no reports → both False, observed_gen=1, times=refTime", func(t *testing.T) {
		t.Parallel()
		// Doc: when resource is created at gen=1, no adapter has reported yet.
		// observed_generation for both Ready and Available must be 1.
		// last_updated_time and last_transition_time must equal resource.last_updated_time (refTime).
		required := []string{"a", "b"}
		in := AggregateResourceStatusInput{
			ResourceGeneration: 1,
			RefTime:            aggTRef,
			RequiredAdapters:   required,
		}
		ready, avail, _ := AggregateResourceStatus(in)
		if ready.Status != api.ConditionFalse {
			t.Errorf("ready.Status got %v, want False", ready.Status)
		}
		if avail.Status != api.ConditionFalse {
			t.Errorf("avail.Status got %v, want False", avail.Status)
		}
		if ready.ObservedGeneration != 1 {
			t.Errorf("ready.ObservedGeneration got %d, want 1", ready.ObservedGeneration)
		}
		if avail.ObservedGeneration != 1 {
			t.Errorf("avail.ObservedGeneration got %d, want 1", avail.ObservedGeneration)
		}
		if !ready.LastUpdatedTime.Equal(aggTRef) {
			t.Errorf("ready.LastUpdatedTime got %v, want refTime=%v", ready.LastUpdatedTime, aggTRef)
		}
		if !avail.LastUpdatedTime.Equal(aggTRef) {
			t.Errorf("avail.LastUpdatedTime got %v, want refTime=%v", avail.LastUpdatedTime, aggTRef)
		}
		if !ready.LastTransitionTime.Equal(aggTRef) {
			t.Errorf("ready.LastTransitionTime got %v, want refTime=%v", ready.LastTransitionTime, aggTRef)
		}
		if !avail.LastTransitionTime.Equal(aggTRef) {
			t.Errorf("avail.LastTransitionTime got %v, want refTime=%v", avail.LastTransitionTime, aggTRef)
		}
	})

	t.Run("no required adapters → both False", func(t *testing.T) {
		t.Parallel()
		in := AggregateResourceStatusInput{
			ResourceGeneration: 1,
			RefTime:            aggTRef,
		}
		ready, avail, _ := AggregateResourceStatus(in)
		if ready.Status != api.ConditionFalse {
			t.Errorf("ready: got %v, want False", ready.Status)
		}
		if avail.Status != api.ConditionFalse {
			t.Errorf("avail: got %v, want False", avail.Status)
		}
	})

	t.Run("all adapters True at current generation → both True", func(t *testing.T) {
		t.Parallel()
		required := []string{"a", "b"}
		in := AggregateResourceStatusInput{
			ResourceGeneration: 2,
			RefTime:            aggTRef,
			RequiredAdapters:   required,
			AdapterStatuses: api.AdapterStatusList{
				makeStatus("a", 2, aggT1, api.AdapterConditionTrue),
				makeStatus("b", 2, aggT2, api.AdapterConditionTrue),
			},
		}
		ready, avail, _ := AggregateResourceStatus(in)
		if ready.Status != api.ConditionTrue {
			t.Errorf("ready: got %v, want True", ready.Status)
		}
		if avail.Status != api.ConditionTrue {
			t.Errorf("avail: got %v, want True", avail.Status)
		}
	})

	t.Run("adapters at old generation → Ready False, Available True (same-gen all-True)", func(t *testing.T) {
		t.Parallel()
		// Both adapters True at gen 1; resource is at gen 2.
		// Ready=False (not at current gen), Available=True (all True at same old gen).
		required := []string{"a", "b"}
		in := AggregateResourceStatusInput{
			ResourceGeneration: 2,
			RefTime:            aggTRef,
			RequiredAdapters:   required,
			AdapterStatuses: api.AdapterStatusList{
				makeStatus("a", 1, aggT1, api.AdapterConditionTrue),
				makeStatus("b", 1, aggT2, api.AdapterConditionTrue),
			},
		}
		ready, avail, _ := AggregateResourceStatus(in)
		if ready.Status != api.ConditionFalse {
			t.Errorf("ready: got %v, want False", ready.Status)
		}
		if avail.Status != api.ConditionTrue {
			t.Errorf("avail: got %v, want True (all True at same old gen)", avail.Status)
		}
	})

	t.Run("split generations, no prev → Ready False, Available False", func(t *testing.T) {
		t.Parallel()
		required := []string{"a", "b"}
		in := AggregateResourceStatusInput{
			ResourceGeneration: 2,
			RefTime:            aggTRef,
			RequiredAdapters:   required,
			AdapterStatuses: api.AdapterStatusList{
				makeStatus("a", 1, aggT1, api.AdapterConditionTrue),
				makeStatus("b", 2, aggT2, api.AdapterConditionTrue),
			},
		}
		ready, avail, _ := AggregateResourceStatus(in)
		if ready.Status != api.ConditionFalse {
			t.Errorf("ready: got %v, want False", ready.Status)
		}
		if avail.Status != api.ConditionFalse {
			t.Errorf("avail: got %v, want False (mixed gens, no prev=True)", avail.Status)
		}
	})

	t.Run("split generations with prev Available=True → Available stays True (sticky)", func(t *testing.T) {
		t.Parallel()
		required := []string{"a", "b"}
		prevConds := encodePrev(
			api.ResourceCondition{
				Type: api.ConditionTypeAvailable, Status: api.ConditionTrue, ObservedGeneration: 1,
				CreatedTime: aggT0, LastUpdatedTime: aggT0, LastTransitionTime: aggT0,
			},
		)
		in := AggregateResourceStatusInput{
			ResourceGeneration: 2,
			RefTime:            aggTRef,
			PrevConditionsJSON: prevConds,
			RequiredAdapters:   required,
			AdapterStatuses: api.AdapterStatusList{
				makeStatus("a", 1, aggT1, api.AdapterConditionTrue),
				makeStatus("b", 2, aggT2, api.AdapterConditionTrue),
			},
		}
		_, avail, _ := AggregateResourceStatus(in)
		if avail.Status != api.ConditionTrue {
			t.Errorf("avail: got %v, want True (sticky: mixed gens but prev=True)", avail.Status)
		}
	})

	t.Run("generation-bump Ready True→False: LastTransitionTime=refTime", func(t *testing.T) {
		t.Parallel()
		// Was Ready=True at gen 1; bumped to gen 2, adapter still at old gen.
		required := []string{"a"}
		prevConds := encodePrev(
			api.ResourceCondition{
				Type: api.ConditionTypeReady, Status: api.ConditionTrue, ObservedGeneration: 1,
				CreatedTime: aggT0, LastUpdatedTime: aggT1, LastTransitionTime: aggT1,
			},
		)
		in := AggregateResourceStatusInput{
			ResourceGeneration: 2,
			RefTime:            aggTRef,
			PrevConditionsJSON: prevConds,
			RequiredAdapters:   required,
			AdapterStatuses: api.AdapterStatusList{
				makeStatus("a", 1, aggT1, api.AdapterConditionTrue), // still at old gen
			},
		}
		ready, _, _ := AggregateResourceStatus(in)
		if ready.Status != api.ConditionFalse {
			t.Errorf("ready: got %v, want False", ready.Status)
		}
		if !ready.LastTransitionTime.Equal(aggTRef) {
			t.Errorf("ready.LastTransitionTime got %v, want refTime=%v (gen-bump branch)", ready.LastTransitionTime, aggTRef)
		}
	})

	t.Run("Available False at tracked gen breaks prev=True sticky", func(t *testing.T) {
		t.Parallel()
		required := []string{"a", "b"}
		prevConds := encodePrev(
			api.ResourceCondition{
				Type: api.ConditionTypeAvailable, Status: api.ConditionTrue, ObservedGeneration: 1,
				CreatedTime: aggT0, LastUpdatedTime: aggT0, LastTransitionTime: aggT0,
			},
		)
		in := AggregateResourceStatusInput{
			ResourceGeneration: 1,
			RefTime:            aggTRef,
			PrevConditionsJSON: prevConds,
			RequiredAdapters:   required,
			AdapterStatuses: api.AdapterStatusList{
				makeStatus("a", 1, aggT1, api.AdapterConditionTrue),
				makeStatus("b", 1, aggT2, api.AdapterConditionFalse), // False at tracked gen 1
			},
		}
		_, avail, _ := AggregateResourceStatus(in)
		if avail.Status != api.ConditionFalse {
			t.Errorf("avail: got %v, want False (False at tracked gen)", avail.Status)
		}
	})

	t.Run("condition Type fields are set correctly", func(t *testing.T) {
		t.Parallel()
		in := AggregateResourceStatusInput{ResourceGeneration: 1, RefTime: aggTRef}
		ready, avail, _ := AggregateResourceStatus(in)
		if ready.Type != api.ConditionTypeReady {
			t.Errorf("ready.Type=%q, want %q", ready.Type, api.ConditionTypeReady)
		}
		if avail.Type != api.ConditionTypeAvailable {
			t.Errorf("avail.Type=%q, want %q", avail.Type, api.ConditionTypeAvailable)
		}
	})

	t.Run("per-adapter conditions are included alongside Ready and Available", func(t *testing.T) {
		t.Parallel()
		required := []string{"adapter1", "adapter2"}
		in := AggregateResourceStatusInput{
			ResourceGeneration: 1,
			RefTime:            aggTRef,
			RequiredAdapters:   required,
			AdapterStatuses: api.AdapterStatusList{
				makeStatus("adapter1", 1, aggT1, api.AdapterConditionTrue),
				makeStatus("adapter2", 1, aggT2, api.AdapterConditionTrue),
			},
		}
		_, _, adapterConds := AggregateResourceStatus(in)
		if len(adapterConds) != 2 {
			t.Fatalf("expected 2 per-adapter conditions, got %d", len(adapterConds))
		}
		byType := make(map[string]api.ResourceCondition, len(adapterConds))
		for _, c := range adapterConds {
			byType[c.Type] = c
		}
		if c, ok := byType["Adapter1Successful"]; !ok || c.Status != api.ConditionTrue {
			t.Errorf("Adapter1Successful: got %+v", byType["Adapter1Successful"])
		}
		if c, ok := byType["Adapter2Successful"]; !ok || c.Status != api.ConditionTrue {
			t.Errorf("Adapter2Successful: got %+v", byType["Adapter2Successful"])
		}
	})
}

// ---------------------------------------------------------------------------
// computeAdapterConditions
// ---------------------------------------------------------------------------

func TestComputeAdapterConditions(t *testing.T) {
	t.Parallel()

	t.Run("adapter not in byAdapter is skipped", func(t *testing.T) {
		t.Parallel()
		conds := computeAdapterConditions(
			[]string{"missing"},
			map[string]adapterAvailableSnapshot{},
			map[string]*api.ResourceCondition{},
			aggTRef,
		)
		if len(conds) != 0 {
			t.Fatalf("expected empty, got %v", conds)
		}
	})

	t.Run("available=True produces True condition with Successful type", func(t *testing.T) {
		t.Parallel()
		byAdapter := map[string]adapterAvailableSnapshot{
			"adapter1": snap(1, true, aggT1),
		}
		conds := computeAdapterConditions([]string{"adapter1"}, byAdapter, map[string]*api.ResourceCondition{}, aggTRef)
		if len(conds) != 1 {
			t.Fatalf("expected 1 condition, got %d", len(conds))
		}
		c := conds[0]
		if c.Type != "Adapter1Successful" {
			t.Errorf("Type got %q, want Adapter1Successful", c.Type)
		}
		if c.Status != api.ConditionTrue {
			t.Errorf("Status got %v, want True", c.Status)
		}
		if c.ObservedGeneration != 1 {
			t.Errorf("ObservedGeneration got %d, want 1", c.ObservedGeneration)
		}
		if !c.LastUpdatedTime.Equal(aggT1) {
			t.Errorf("LastUpdatedTime got %v, want %v", c.LastUpdatedTime, aggT1)
		}
	})

	t.Run("available=False produces False condition", func(t *testing.T) {
		t.Parallel()
		byAdapter := map[string]adapterAvailableSnapshot{
			"adapter1": snap(2, false, aggT2),
		}
		conds := computeAdapterConditions([]string{"adapter1"}, byAdapter, map[string]*api.ResourceCondition{}, aggTRef)
		if len(conds) != 1 || conds[0].Status != api.ConditionFalse {
			t.Fatalf("expected False, got %+v", conds)
		}
	})

	t.Run("reason and message are copied from snapshot", func(t *testing.T) {
		t.Parallel()
		r, m := "MyReason", "my message"
		byAdapter := map[string]adapterAvailableSnapshot{
			"a": {observedGeneration: 1, availableTrue: true, observedTime: aggT1, reason: &r, message: &m},
		}
		conds := computeAdapterConditions([]string{"a"}, byAdapter, map[string]*api.ResourceCondition{}, aggTRef)
		if len(conds) != 1 {
			t.Fatalf("expected 1 condition, got %d", len(conds))
		}
		if conds[0].Reason == nil || *conds[0].Reason != r {
			t.Errorf("Reason got %v, want %q", conds[0].Reason, r)
		}
		if conds[0].Message == nil || *conds[0].Message != m {
			t.Errorf("Message got %v, want %q", conds[0].Message, m)
		}
	})

	t.Run("no prev: CreatedTime and LastTransitionTime equal LastUpdatedTime", func(t *testing.T) {
		t.Parallel()
		byAdapter := map[string]adapterAvailableSnapshot{"a": snap(1, true, aggT1)}
		conds := computeAdapterConditions([]string{"a"}, byAdapter, map[string]*api.ResourceCondition{}, aggTRef)
		c := conds[0]
		if !c.CreatedTime.Equal(aggTRef) {
			t.Errorf("CreatedTime got %v, want refTime=%v", c.CreatedTime, aggTRef)
		}
		if !c.LastTransitionTime.Equal(aggT1) {
			t.Errorf("LastTransitionTime got %v, want observedTime=%v", c.LastTransitionTime, aggT1)
		}
	})

	t.Run("prev same status: CreatedTime carried, LastTransitionTime unchanged", func(t *testing.T) {
		t.Parallel()
		byAdapter := map[string]adapterAvailableSnapshot{"a": snap(2, true, aggT2)}
		prevByType := map[string]*api.ResourceCondition{
			"ASuccessful": {
				Type:               "ASuccessful",
				Status:             api.ConditionTrue,
				CreatedTime:        aggT0,
				LastTransitionTime: aggT0,
			},
		}
		conds := computeAdapterConditions([]string{"a"}, byAdapter, prevByType, aggTRef)
		c := conds[0]
		if !c.CreatedTime.Equal(aggT0) {
			t.Errorf("CreatedTime got %v, want prev.CreatedTime=%v", c.CreatedTime, aggT0)
		}
		if !c.LastTransitionTime.Equal(aggT0) {
			t.Errorf("LastTransitionTime got %v, want prev.LastTransitionTime=%v (no transition)", c.LastTransitionTime, aggT0)
		}
	})

	t.Run("prev status differs: LastTransitionTime updated to LastUpdatedTime", func(t *testing.T) {
		t.Parallel()
		byAdapter := map[string]adapterAvailableSnapshot{"a": snap(2, false, aggT2)}
		prevByType := map[string]*api.ResourceCondition{
			"ASuccessful": {
				Type:               "ASuccessful",
				Status:             api.ConditionTrue,
				CreatedTime:        aggT0,
				LastTransitionTime: aggT0,
			},
		}
		conds := computeAdapterConditions([]string{"a"}, byAdapter, prevByType, aggTRef)
		c := conds[0]
		if !c.LastTransitionTime.Equal(aggT2) {
			t.Errorf("LastTransitionTime got %v, want observedTime=%v (status changed)", c.LastTransitionTime, aggT2)
		}
	})

	t.Run("hyphenated adapter name produces PascalCase type", func(t *testing.T) {
		t.Parallel()
		byAdapter := map[string]adapterAvailableSnapshot{"my-adapter": snap(1, true, aggT1)}
		conds := computeAdapterConditions([]string{"my-adapter"}, byAdapter, map[string]*api.ResourceCondition{}, aggTRef)
		if len(conds) != 1 || conds[0].Type != "MyAdapterSuccessful" {
			t.Fatalf("expected MyAdapterSuccessful, got %+v", conds)
		}
	})
}

// ---------------------------------------------------------------------------
// MapAdapterToConditionType
// ---------------------------------------------------------------------------

func TestMapAdapterToConditionType(t *testing.T) {
	tests := []struct {
		adapter  string
		expected string
	}{
		{"validator", "ValidatorSuccessful"},
		{"dns", "DnsSuccessful"},
		{"gcp-provisioner", "GcpProvisionerSuccessful"},
		{"unknown-adapter", "UnknownAdapterSuccessful"},
		{"multi-word-adapter", "MultiWordAdapterSuccessful"},
		{"single", "SingleSuccessful"},
	}

	for _, tt := range tests {
		result := MapAdapterToConditionType(tt.adapter)
		if result != tt.expected {
			t.Errorf("MapAdapterToConditionType(%q) = %q, want %q",
				tt.adapter, result, tt.expected)
		}
	}
}

// Test custom suffix mapping (for future use).
func TestMapAdapterToConditionType_CustomSuffix(t *testing.T) {
	// Temporarily add a custom mapping.
	adapterConditionSuffixMap["test-adapter"] = "Ready"
	defer delete(adapterConditionSuffixMap, "test-adapter")

	result := MapAdapterToConditionType("test-adapter")
	expected := "TestAdapterReady"
	if result != expected {
		t.Errorf("MapAdapterToConditionType(%q) = %q, want %q",
			"test-adapter", result, expected)
	}
}

// Test that default behavior still works after custom suffix is removed.
func TestMapAdapterToConditionType_DefaultAfterCustom(t *testing.T) {
	// Add and then remove custom mapping.
	adapterConditionSuffixMap["dns"] = "Ready"
	delete(adapterConditionSuffixMap, "dns")

	result := MapAdapterToConditionType("dns")
	expected := "DnsSuccessful"
	if result != expected {
		t.Errorf("MapAdapterToConditionType(%q) = %q, want %q (should revert to default)",
			"dns", result, expected)
	}
}

// ---------------------------------------------------------------------------
// ValidateMandatoryConditions
// ---------------------------------------------------------------------------

func TestValidateMandatoryConditions_AllPresent(t *testing.T) {
	t.Parallel()
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionFalse, LastTransitionTime: time.Now()},
	}

	errorType, conditionName := ValidateMandatoryConditions(conditions)
	if errorType != "" {
		t.Errorf("Expected no errors, got errorType: %s, conditionName: %s", errorType, conditionName)
	}
}

func TestValidateMandatoryConditions_MissingAvailable(t *testing.T) {
	t.Parallel()
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}

	errorType, conditionName := ValidateMandatoryConditions(conditions)
	if errorType != ConditionValidationErrorMissing {
		t.Errorf("Expected errorType ConditionValidationErrorMissing, got: %s", errorType)
	}
	if conditionName != api.ConditionTypeAvailable {
		t.Errorf("Expected missing condition %s, got: %s", api.ConditionTypeAvailable, conditionName)
	}
}

func TestValidateMandatoryConditions_MandatoryConditionUnknown(t *testing.T) {
	t.Parallel()
	// Unknown status in Applied/Health is allowed; only Available=Unknown has special handling elsewhere.
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionUnknown, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}

	errorType, conditionName := ValidateMandatoryConditions(conditions)
	if errorType != "" {
		t.Errorf("Expected no errors (Unknown is allowed), got errorType: %s, conditionName: %s", errorType, conditionName)
	}
}

func TestValidateMandatoryConditions_WithCustomConditions(t *testing.T) {
	t.Parallel()
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: "CustomCondition", Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeReady, Status: api.AdapterConditionFalse, LastTransitionTime: time.Now()},
	}

	errorType, conditionName := ValidateMandatoryConditions(conditions)
	if errorType != "" {
		t.Errorf("Expected no errors, got errorType: %s, conditionName: %s", errorType, conditionName)
	}
}

func TestValidateMandatoryConditions_EmptyConditions(t *testing.T) {
	t.Parallel()
	conditions := []api.AdapterCondition{}

	errorType, conditionName := ValidateMandatoryConditions(conditions)
	if errorType != ConditionValidationErrorMissing {
		t.Errorf("Expected errorType ConditionValidationErrorMissing, got: %s", errorType)
	}
	if conditionName != api.ConditionTypeAvailable {
		t.Errorf("Expected missing condition %s, got: %s", api.ConditionTypeAvailable, conditionName)
	}
}

// TestValidateMandatoryConditions_MissingMultiple tests that when multiple conditions are missing,
// the function returns the first missing one.
func TestValidateMandatoryConditions_MissingMultiple(t *testing.T) {
	t.Parallel()
	// Test: Only Available present, missing Applied and Health.
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionUnknown, LastTransitionTime: time.Now()},
	}

	errorType, conditionName := ValidateMandatoryConditions(conditions)

	// Should return missing condition.
	if errorType != ConditionValidationErrorMissing {
		t.Errorf("Expected errorType ConditionValidationErrorMissing, got: %s", errorType)
	}
	if conditionName != api.ConditionTypeApplied && conditionName != api.ConditionTypeHealth {
		t.Errorf("Expected missing condition to be Applied or Health, got: %s", conditionName)
	}
}
