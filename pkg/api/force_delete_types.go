package api

// TODO(HYPERFLEET-1075): Move to hyperfleet-api-spec and generate via oapi-codegen.
// ForceDeleteRequest is the request body for force-delete endpoints.
type ForceDeleteRequest struct {
	Reason string `json:"reason"`
}
