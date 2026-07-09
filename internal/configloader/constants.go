package configloader

// Field path constants for configuration structure.
// These constants define the known field names used in adapter configuration
// to avoid hardcoding strings throughout the codebase.

// Field names
const (
	FieldAdapter       = "adapter"
	FieldHyperfleetAPI = "hyperfleet_api"
	FieldKubernetes    = "kubernetes"
	FieldParams        = "params"
	FieldPreconditions = "preconditions"
	FieldResources     = "resources"
	FieldPost          = "post"
)

// Adapter field names
const (
	FieldVersion = "version"
)

// Parameter field names
const (
	FieldName        = "name"
	FieldSource      = "source"
	FieldType        = "type"
	FieldDescription = "description"
	FieldRequired    = "required"
	FieldDefault     = "default"
)

// Payload field names (for post.payloads)
const (
	FieldPayloads = "payloads"
	FieldBuild    = "build"
	FieldBuildRef = "build_ref"
)

// Precondition field names
const (
	FieldAPICall    = "api_call"
	FieldCapture    = "capture"
	FieldConditions = "conditions"
	FieldExpression = "expression"
)

// API call field names
const (
	FieldMethod  = "method"
	FieldURL     = "url"
	FieldTimeout = "timeout"
	FieldHeaders = "headers"
	FieldBody    = "body"
)

// Header field names
const (
	FieldHeaderValue = "value"
)

// Condition field names
const (
	FieldField    = "field"
	FieldOperator = "operator"
	FieldValue    = "value"  // Supports any type including lists for operators like "in", "notIn"
	FieldValues   = "values" // YAML alias for Value - both "value" and "values" are accepted in YAML
)

// Transport field names
const (
	FieldTransport     = "transport"
	FieldClient        = "client"
	FieldMaestro       = "maestro"
	FieldTargetCluster = "target_cluster"
)

// Transport client types
const (
	TransportClientKubernetes = "kubernetes"
	TransportClientMaestro    = "maestro"
)

// Resource field names
const (
	FieldManifest          = "manifest"
	FieldRecreateOnChange  = "recreate_on_change"
	FieldDiscovery         = "discovery"
	FieldNestedDiscoveries = "nested_discoveries"
	FieldLifecycle         = "lifecycle"
)

// Lifecycle field names
const (
	FieldLifecycleCreate            = "create"
	FieldLifecycleDelete            = "delete"
	FieldLifecyclePropagationPolicy = "propagationPolicy"
	FieldLifecycleWhen              = "when"
)

// Manifest reference field names
const (
	FieldRef = "ref"
)

// Discovery field names
const (
	FieldNamespace   = "namespace"
	FieldByName      = "by_name"
	FieldBySelectors = "by_selectors"
)

// Selector field names
const (
	FieldLabelSelector = "label_selector"
)

// Post config field names
const (
	FieldPostActions = "post_actions"
)

// Kubernetes manifest field names
const (
	FieldAPIVersion = "apiVersion"
	FieldKind       = "kind"
)
