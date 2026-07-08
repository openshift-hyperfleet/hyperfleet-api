package dryrun

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/executor"
)

const (
	statusSuccess = "SUCCESS"
	statusFailed  = "FAILED"
)

// ExecutionTrace contains all data needed to produce the trace output.
type ExecutionTrace struct {
	Result    *executor.ExecutionResult
	APIClient *DryrunAPIClient
	Transport *DryrunTransportClient
	EventID   string
	EventType string
	Verbose   bool
}

// TraceJSON is the JSON-serializable representation of the execution trace.
type TraceJSON struct {
	Event               TraceEvent             `json:"event"`
	Status              string                 `json:"status"`
	Params              map[string]interface{} `json:"params,omitempty"`
	Preconditions       []TracePrecondition    `json:"preconditions,omitempty"`
	Resources           []TraceResource        `json:"resources,omitempty"`
	DiscoveredResources map[string]interface{} `json:"discoveredResources,omitempty"`
	PostActions         []TracePostAction      `json:"postActions,omitempty"`
	Errors              map[string]string      `json:"errors,omitempty"`
	APIRequests         []TraceAPIRequest      `json:"apiRequests,omitempty"`
	TransportOps        []TraceTransportOp     `json:"transportOperations,omitempty"`
}

// TraceEvent is the JSON representation of the event.
type TraceEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// TracePrecondition is the JSON representation of a precondition result.
type TracePrecondition struct {
	Error   string `json:"error,omitempty"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Matched bool   `json:"matched"`
}

// TraceResource is the JSON representation of a resource result.
type TraceResource struct {
	DiscoveredState map[string]interface{} `json:"discoveredState,omitempty"`
	Name            string                 `json:"name"`
	Kind            string                 `json:"kind"`
	Namespace       string                 `json:"namespace,omitempty"`
	ResName         string                 `json:"resourceName,omitempty"`
	Status          string                 `json:"status"`
	Operation       string                 `json:"operation"`
	Reason          string                 `json:"reason,omitempty"`
	Error           string                 `json:"error,omitempty"`
}

// TracePostAction is the JSON representation of a post-action result.
type TracePostAction struct {
	Error   string `json:"error,omitempty"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Skipped bool   `json:"skipped,omitempty"`
}

// TraceAPIRequest is the JSON representation of a recorded API request.
type TraceAPIRequest struct {
	Request    string `json:"requestBody,omitempty"`
	Response   string `json:"responseBody,omitempty"`
	Method     string `json:"method"`
	URL        string `json:"url"`
	StatusCode int    `json:"statusCode"`
}

// TraceTransportOp is the JSON representation of a recorded transport operation.
type TraceTransportOp struct {
	Operation string `json:"operation"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	Result    string `json:"result,omitempty"`
}

// FormatText formats the execution trace as human-readable text.
func (t *ExecutionTrace) FormatText() string {
	var b strings.Builder
	result := t.Result

	b.WriteString("Dry-Run Execution Trace\n")
	b.WriteString("========================\n")
	fmt.Fprintf(&b, "Event: id=%s type=%s\n\n", t.EventID, t.EventType)

	// Compute how many API requests belong to each phase
	precondAPICallCount := 0
	for _, pr := range result.PreconditionResults {
		if pr.APICallMade {
			precondAPICallCount++
		}
	}
	postActionAPICallCount := 0
	for _, pa := range result.PostActionResults {
		if pa.APICallMade {
			postActionAPICallCount++
		}
	}
	paramAPICallCount := max(0, len(t.APIClient.Requests)-precondAPICallCount-postActionAPICallCount)

	// Phase 1: Parameter Extraction
	paramStatus := statusSuccess
	if _, ok := result.Errors[executor.PhaseParamExtraction]; ok {
		paramStatus = statusFailed
	}
	fmt.Fprintf(&b, "Phase 1: Parameter Extraction .............. %s\n", paramStatus)
	if paramStatus == statusSuccess {
		for name, val := range result.Params {
			fmt.Fprintf(&b, "  %-16s = %v\n", name, formatValue(val))
		}
		for i := range paramAPICallCount {
			req := t.APIClient.Requests[i]
			fmt.Fprintf(&b, "  API Call: %s %s -> %d\n", req.Method, req.URL, req.StatusCode)
			if t.Verbose {
				if len(req.Body) > 0 {
					fmt.Fprintf(&b, "    [verbose] Request body:\n      %s\n", prettyJSON(req.Body))
				}
				if len(req.Response) > 0 {
					fmt.Fprintf(&b, "    [verbose] Response body:\n      %s\n", prettyJSON(req.Response))
				}
			}
		}
	} else {
		fmt.Fprintf(&b, "  Error: %v\n", result.Errors[executor.PhaseParamExtraction])
	}
	b.WriteString("\n")

	// Phase 2: Preconditions
	precondStatus := statusSuccess
	precondDetail := ""
	_, hasPrecondErr := result.Errors[executor.PhasePreconditions]
	switch {
	case hasPrecondErr:
		precondStatus = statusFailed
	case result.ResourcesSkipped && result.SkipReason == executor.ResourceNotFoundReason &&
		len(result.PostActionResults) == 0:
		precondDetail = " (RESOURCE NOT FOUND)"
	case result.ResourcesSkipped && result.SkipReason != "" && result.SkipReason != executor.ResourceNotFoundReason:
		precondDetail = " (NOT MET)"
	case len(result.PreconditionResults) > 0:
		precondDetail = " (MET)"
	}
	fmt.Fprintf(&b, "Phase 2: Preconditions ..................... %s%s\n", precondStatus, precondDetail)

	apiReqIdx := paramAPICallCount
	for i, pr := range result.PreconditionResults {
		status := "PASS"
		if pr.Status == executor.StatusFailed {
			status = "FAIL"
		} else if !pr.Matched {
			status = "NOT MET"
		}
		fmt.Fprintf(&b, "  [%d/%d] %-30s %s\n", i+1, len(result.PreconditionResults), pr.Name, status)

		if pr.APICallMade && apiReqIdx < len(t.APIClient.Requests) {
			req := t.APIClient.Requests[apiReqIdx]
			fmt.Fprintf(&b, "    API Call: %s %s -> %d\n", req.Method, req.URL, req.StatusCode)
			if t.Verbose {
				if len(req.Body) > 0 {
					fmt.Fprintf(&b, "    [verbose] Request body:\n      %s\n", prettyJSON(req.Body))
				}
				if len(req.Response) > 0 {
					fmt.Fprintf(&b, "    [verbose] Response body:\n      %s\n", prettyJSON(req.Response))
				}
			}
			apiReqIdx++
		}

		if len(pr.CapturedFields) > 0 {
			for name, val := range pr.CapturedFields {
				fmt.Fprintf(&b, "    Captured: %s = %v\n", name, formatValue(val))
			}
		}

		if pr.Error != nil {
			fmt.Fprintf(&b, "    Error: %v\n", pr.Error)
		}
	}
	b.WriteString("\n")

	// Phase 3: Resources
	resStatus := statusSuccess
	if _, ok := result.Errors[executor.PhaseResources]; ok {
		resStatus = statusFailed
	} else if result.ResourcesSkipped {
		resStatus = "SKIPPED"
	}
	fmt.Fprintf(&b, "Phase 3: Resources ........................ %s\n", resStatus)

	if result.ResourcesSkipped {
		fmt.Fprintf(&b, "  Reason: %s\n", result.SkipReason)
	} else {
		for i, rr := range result.ResourceResults {
			opStr := strings.ToUpper(string(rr.Operation))
			if opStr == "" {
				opStr = "UNKNOWN"
			}
			status := opStr
			if rr.Status == executor.StatusFailed {
				status = statusFailed
			}
			fmt.Fprintf(&b, "  [%d/%d] %-30s %s\n", i+1, len(result.ResourceResults), rr.Name, status)
			fmt.Fprintf(&b, "    Kind: %-12s Namespace: %-12s Name: %s\n", rr.Kind, rr.Namespace, rr.ResourceName)

			if rr.DiscoveredState != nil && rr.DiscoveredState.Object != nil {
				if stateBytes, err := json.Marshal(rr.DiscoveredState.Object); err == nil {
					fmt.Fprintf(&b, "    Pre-delete state:\n      %s\n", prettyJSON(stateBytes))
				}
			}

			if t.Verbose {
				for _, tr := range t.Transport.Records {
					if tr.Operation == operationApply && tr.GVK.Kind == rr.Kind &&
						tr.Name == rr.ResourceName && tr.Namespace == rr.Namespace {
						fmt.Fprintf(&b, "    [verbose] Rendered manifest:\n      %s\n", prettyJSON(tr.Manifest))
						break
					}
				}
			}

			if rr.Error != nil {
				fmt.Fprintf(&b, "    Error: %v\n", rr.Error)
			}
		}
	}

	// Discovery results (resources available for payload CEL: resources.<name>)
	if result.ExecutionContext != nil && result.ExecutionContext.Resources != nil &&
		len(result.ExecutionContext.Resources) > 0 {
		msg := "\nPhase 3.5: Discovery Results ................. (available as resources.* in payload)\n"
		b.WriteString(msg)
		celVars := result.ExecutionContext.GetCELVariables()
		if r, ok := celVars["resources"].(map[string]interface{}); ok {
			for name, val := range r {
				fmt.Fprintf(&b, "  %s:\n", name)
				if val == nil {
					b.WriteString("    null\n")
					continue
				}
				raw, err := json.Marshal(val)
				if err != nil {
					fmt.Fprintf(&b, "    %v\n", val)
					continue
				}
				fmt.Fprintf(&b, "    %s\n", prettyJSONWithPrefix(raw, "    "))
			}
		}
	}
	b.WriteString("\n")

	// Phase 4: Post Actions
	postStatus := statusSuccess
	postDetail := ""
	if _, ok := result.Errors[executor.PhasePostActions]; ok {
		postStatus = statusFailed
	} else if result.ResourcesSkipped && result.SkipReason == executor.ResourceNotFoundReason &&
		len(result.PostActionResults) > 0 {
		postDetail = " (RESOURCE NOT FOUND)"
	}
	fmt.Fprintf(&b, "Phase 4: Post Actions ..................... %s%s\n", postStatus, postDetail)

	for i, pa := range result.PostActionResults {
		status := "EXECUTED"
		if pa.Skipped {
			status = "SKIPPED"
		} else if pa.Status == executor.StatusFailed {
			status = statusFailed
		}
		fmt.Fprintf(&b, "  [%d/%d] %-30s %s\n", i+1, len(result.PostActionResults), pa.Name, status)

		if pa.Skipped {
			fmt.Fprintf(&b, "    Reason: %s\n", pa.SkipReason)
		}

		if pa.APICallMade && apiReqIdx < len(t.APIClient.Requests) {
			req := t.APIClient.Requests[apiReqIdx]
			fmt.Fprintf(&b, "    API Call: %s %s -> %d\n", req.Method, req.URL, req.StatusCode)
			if t.Verbose {
				if len(req.Body) > 0 {
					fmt.Fprintf(&b, "    [verbose] Request body:\n      %s\n", prettyJSON(req.Body))
				}
				if len(req.Response) > 0 {
					fmt.Fprintf(&b, "    [verbose] Response body:\n      %s\n", prettyJSON(req.Response))
				}
			}
			apiReqIdx++
		}

		if pa.Error != nil {
			fmt.Fprintf(&b, "    Error: %v\n", pa.Error)
		}
	}
	b.WriteString("\n")

	// Final result
	resultStr := statusSuccess
	if result.Status == executor.StatusFailed {
		resultStr = statusFailed
	}
	fmt.Fprintf(&b, "Result: %s\n", resultStr)

	return b.String()
}

// FormatJSON formats the execution trace as JSON.
func (t *ExecutionTrace) FormatJSON() ([]byte, error) {
	result := t.Result

	trace := TraceJSON{
		Event:  TraceEvent{ID: t.EventID, Type: t.EventType},
		Status: string(result.Status),
		Params: result.Params,
	}

	// Discovered resources (from discovery phase, used in payload CEL)
	if result.ExecutionContext != nil {
		celVars := result.ExecutionContext.GetCELVariables()
		if r, ok := celVars["resources"].(map[string]interface{}); ok {
			trace.DiscoveredResources = r
		}
	}

	// Preconditions
	for _, pr := range result.PreconditionResults {
		tp := TracePrecondition{
			Name:    pr.Name,
			Status:  string(pr.Status),
			Matched: pr.Matched,
		}
		if pr.Error != nil {
			tp.Error = pr.Error.Error()
		}
		trace.Preconditions = append(trace.Preconditions, tp)
	}

	// Resources
	for _, rr := range result.ResourceResults {
		tr := TraceResource{
			Name:      rr.Name,
			Kind:      rr.Kind,
			Namespace: rr.Namespace,
			ResName:   rr.ResourceName,
			Status:    string(rr.Status),
			Operation: string(rr.Operation),
			Reason:    rr.OperationReason,
		}
		if rr.DiscoveredState != nil && rr.DiscoveredState.Object != nil {
			tr.DiscoveredState = rr.DiscoveredState.Object
		}
		if rr.Error != nil {
			tr.Error = rr.Error.Error()
		}
		trace.Resources = append(trace.Resources, tr)
	}

	// Post Actions
	for _, pa := range result.PostActionResults {
		tp := TracePostAction{
			Name:    pa.Name,
			Status:  string(pa.Status),
			Skipped: pa.Skipped,
		}
		if pa.Error != nil {
			tp.Error = pa.Error.Error()
		}
		trace.PostActions = append(trace.PostActions, tp)
	}

	// Errors
	if len(result.Errors) > 0 {
		trace.Errors = make(map[string]string)
		for phase, err := range result.Errors {
			trace.Errors[string(phase)] = err.Error()
		}
	}

	// API Requests
	for _, req := range t.APIClient.Requests {
		tr := TraceAPIRequest{
			Method:     req.Method,
			URL:        req.URL,
			StatusCode: req.StatusCode,
		}
		if t.Verbose {
			if len(req.Body) > 0 {
				tr.Request = string(req.Body)
			}
			if len(req.Response) > 0 {
				tr.Response = string(req.Response)
			}
		}
		trace.APIRequests = append(trace.APIRequests, tr)
	}

	// Transport Operations
	for _, rec := range t.Transport.Records {
		op := TraceTransportOp{
			Operation: rec.Operation,
			Kind:      rec.GVK.Kind,
			Namespace: rec.Namespace,
			Name:      rec.Name,
		}
		if rec.Result != nil {
			op.Result = string(rec.Result.Operation)
		}
		trace.TransportOps = append(trace.TransportOps, op)
	}

	return json.MarshalIndent(trace, "", "  ")
}

// prettyJSON attempts to indent raw JSON bytes for readable output using a 6-space prefix.
// If the input is not valid JSON, it is returned as-is.
func prettyJSON(raw []byte) string {
	return prettyJSONWithPrefix(raw, "      ")
}

// prettyJSONWithPrefix attempts to indent raw JSON bytes with the given line prefix.
// If the input is not valid JSON, it is returned as-is.
func prettyJSONWithPrefix(raw []byte, prefix string) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, prefix, "  "); err != nil {
		return string(raw)
	}
	return buf.String()
}

func formatValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val)
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	}
}
