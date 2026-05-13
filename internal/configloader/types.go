package configloader

import (
	"fmt"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleetapi"
	"gopkg.in/yaml.v3"
)

// Config is the unified configuration passed throughout the application.
// Created by merging AdapterConfig (deployment) and AdapterTaskConfig (task).
type Config struct {
	Post          *PostConfig    `yaml:"post,omitempty"`
	Log           LogConfig      `yaml:"log,omitempty"`
	Adapter       AdapterInfo    `yaml:"adapter"`
	Params        []Parameter    `yaml:"params,omitempty"`
	Preconditions []Precondition `yaml:"preconditions,omitempty"`
	Resources     []Resource     `yaml:"resources,omitempty"`
	Clients       ClientsConfig  `yaml:"clients"`
	DebugConfig   bool           `yaml:"debug_config,omitempty"`
}

// Merge combines AdapterConfig (deployment) and AdapterTaskConfig (task) into a unified Config.
// The adapter info and clients come from the deployment config.
// The params, preconditions, resources, and post-processing come from the task config.
func Merge(adapterCfg *AdapterConfig, taskCfg *AdapterTaskConfig) *Config {
	if adapterCfg == nil || taskCfg == nil {
		return nil
	}

	return &Config{
		Adapter:       adapterCfg.Adapter,
		Clients:       adapterCfg.Clients,
		DebugConfig:   adapterCfg.DebugConfig,
		Log:           adapterCfg.Log,
		Params:        taskCfg.Params,
		Preconditions: taskCfg.Preconditions,
		Resources:     taskCfg.Resources,
		Post:          taskCfg.Post,
	}
}

const redactedValue = "**REDACTED**"

// Redacted returns a copy of Config with sensitive fields replaced by redactedValue.
func (c *Config) Redacted() *Config {
	if c == nil {
		return nil
	}
	copy := *c
	copy.Clients = redactedClients(c.Clients)
	return &copy
}

func redactedClients(clients ClientsConfig) ClientsConfig {
	copy := clients
	if clients.Maestro != nil {
		maestroCopy := *clients.Maestro
		if maestroCopy.Auth.TLSConfig != nil {
			tlsCopy := *maestroCopy.Auth.TLSConfig
			if tlsCopy.CAFile != "" {
				tlsCopy.CAFile = redactedValue
			}
			if tlsCopy.CertFile != "" {
				tlsCopy.CertFile = redactedValue
			}
			if tlsCopy.KeyFile != "" {
				tlsCopy.KeyFile = redactedValue
			}
			if tlsCopy.HTTPCAFile != "" {
				tlsCopy.HTTPCAFile = redactedValue
			}
			maestroCopy.Auth.TLSConfig = &tlsCopy
		}
		copy.Maestro = &maestroCopy
	}
	return copy
}

// FieldExpressionDef represents a common pattern for value extraction.
// Used when a value should be computed via field extraction (JSONPath) or CEL expression.
// Only one of Field or Expression should be set.
type FieldExpressionDef struct {
	// Field uses JSONPath/dot notation to extract value (mutually exclusive with Expression)
	Field string `yaml:"field,omitempty" validate:"required_without=Expression,excluded_with=Expression"`
	// Expression uses CEL expression to evaluate (mutually exclusive with Field)
	Expression string `yaml:"expression,omitempty" validate:"required_without=Field,excluded_with=Field"`
}

// ValueDef represents a dynamic value definition in payload builds.
// Used when a payload field should be computed via field extraction (JSONPath)
// or CEL expression. Only one of Field or Expression should be set.
//
// Example YAML with field (JSONPath):
//
//	status:
//	  field: "response.status"
//	  default: "unknown"
//
// Example YAML with expression (CEL):
//
//	status:
//	  expression: "adapter.?errorMessage.orValue(\"\")"
//	  default: "success"
type ValueDef struct {
	// Default value if extraction fails or returns nil
	Default            any `yaml:"default"`
	FieldExpressionDef `yaml:",inline"`
}

// ParseValueDef attempts to parse a value as a ValueDef.
// Returns the parsed ValueDef and true if the value contains either field or expression.
// Returns nil and false if the value is not a value definition.
func ParseValueDef(v any) (*ValueDef, bool) {
	// Must be a map to be a value definition
	if _, ok := v.(map[string]any); !ok {
		return nil, false
	}

	// Marshal to YAML then unmarshal to ValueDef
	data, err := yaml.Marshal(v)
	if err != nil {
		return nil, false
	}

	var valueDef ValueDef
	if err := yaml.Unmarshal(data, &valueDef); err != nil {
		return nil, false
	}

	// Must have at least one of field or expression
	if valueDef.Field == "" && valueDef.Expression == "" {
		return nil, false
	}

	return &valueDef, true
}

// AdapterInfo contains basic adapter information
type AdapterInfo struct {
	Name    string `yaml:"name" mapstructure:"name" validate:"required"`
	Version string `yaml:"version,omitempty" mapstructure:"version"`
}

// LogConfig contains logging configuration.
// Priority (lowest to highest): config file < LOG_LEVEL env < --log-level flag
type LogConfig struct {
	Level  string `yaml:"level,omitempty" mapstructure:"level"`
	Format string `yaml:"format,omitempty" mapstructure:"format"`
	Output string `yaml:"output,omitempty" mapstructure:"output"`
}

// HyperfleetAPIConfig is the HyperFleet API client configuration.
// Alias to hyperfleetapi.ClientConfig to ensure shared schema.
type HyperfleetAPIConfig = hyperfleetapi.ClientConfig

// BrokerConfig contains broker consumer configuration
type BrokerConfig struct {
	SubscriptionID string `yaml:"subscription_id,omitempty" mapstructure:"subscription_id"`
	Topic          string `yaml:"topic,omitempty" mapstructure:"topic"`
}

// KubernetesConfig contains Kubernetes configuration
type KubernetesConfig struct {
	APIVersion string `yaml:"api_version" mapstructure:"api_version"`
	// KubeConfigPath is the path to a kubeconfig file. Empty means in-cluster auth.
	KubeConfigPath string `yaml:"kube_config_path,omitempty" mapstructure:"kube_config_path"`
	// QPS is the client-side rate limit. Zero uses defaults.
	QPS float32 `yaml:"qps,omitempty" mapstructure:"qps"`
	// Burst is the client-side burst rate. Zero uses defaults.
	Burst int `yaml:"burst,omitempty" mapstructure:"burst"`
}

// Parameter represents a parameter extraction configuration.
// Parameters are extracted from external sources (event data, env vars) using Source.
type Parameter struct {
	Default     interface{} `yaml:"default,omitempty"`
	Name        string      `yaml:"name" validate:"required"`
	Source      string      `yaml:"source,omitempty" validate:"required"`
	Type        string      `yaml:"type,omitempty"`
	Description string      `yaml:"description,omitempty"`
	Required    bool        `yaml:"required,omitempty"`
}

// Payload represents a dynamically built payload for post-processing.
// Payloads are computed internally using expressions and build definitions.
//
// IMPORTANT: Build and BuildRef are mutually exclusive - exactly one must be set.
// Setting both or neither will result in a validation error.
// - Use Build for inline payload definitions directly in the config
// - Use BuildRef to reference an external YAML file containing the build definition
type Payload struct {
	// Build contains a structure that will be evaluated and converted to JSON at runtime.
	// The structure is kept as raw interface{} to allow flexible schema definitions.
	// Mutually exclusive with BuildRef.
	Build interface{} `yaml:"build,omitempty" validate:"required_without=BuildRef,excluded_with=BuildRef"`
	// BuildRefContent holds the loaded content from BuildRef file (populated by loader)
	BuildRefContent map[string]interface{} `yaml:"-"`
	Name            string                 `yaml:"name" validate:"required"`
	// BuildRef references an external YAML file containing the build definition.
	// Mutually exclusive with Build.
	BuildRef string `yaml:"build_ref,omitempty" validate:"required_without=Build,excluded_with=Build"`
}

// Validate checks that exactly one of Build or BuildRef is set.
func (p *Payload) Validate() error {
	hasBuild := p.Build != nil
	hasBuildRef := p.BuildRef != ""

	if !hasBuild && !hasBuildRef {
		return fmt.Errorf("either 'build' or 'build_ref' must be set")
	}
	if hasBuild && hasBuildRef {
		return fmt.Errorf("'build' and 'build_ref' are mutually exclusive")
	}
	return nil
}

// ActionBase contains common fields for action-like configurations.
// Used by Precondition and PostAction to reduce duplication.
type ActionBase struct {
	APICall *APICall   `yaml:"api_call,omitempty" validate:"omitempty"`
	Log     *LogAction `yaml:"log,omitempty"`
	Name    string     `yaml:"name" validate:"required,resourcename"`
}

// Precondition represents a precondition check.
// Must have at least one of: APICall (from ActionBase), Expression, or Conditions.
type Precondition struct {
	ActionBase `yaml:",inline"`
	Expression string         `yaml:"expression,omitempty" validate:"required_without_all=ActionBase.APICall Conditions"`
	Capture    []CaptureField `yaml:"capture,omitempty" validate:"dive"`
	//nolint:lll
	Conditions []Condition `yaml:"conditions,omitempty" validate:"dive,required_without_all=ActionBase.APICall Expression"`
}

// APICall represents an API call configuration
type APICall struct {
	Method        string   `yaml:"method" validate:"required,oneof=GET POST PUT PATCH DELETE"`
	URL           string   `yaml:"url" validate:"required"`
	Timeout       string   `yaml:"timeout,omitempty"`
	RetryBackoff  string   `yaml:"retry_backoff,omitempty"`
	Body          string   `yaml:"body,omitempty"`
	Headers       []Header `yaml:"headers,omitempty"`
	RetryAttempts int      `yaml:"retry_attempts,omitempty"`
}

// Header represents an HTTP header
type Header struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

// CaptureField represents a field capture configuration from API response.
//
// Supports two modes (mutually exclusive):
//   - Field: JSONPath expression for simple field extraction (e.g., "{.items[0].name}")
//   - Expression: CEL expression for complex transformations
//     (e.g., "response.items.filter(i, i.adapter == 'x')")
//
// Default applies only to field: captures. When the field is absent from the API response,
// Default is used and no WARN is logged. Ignored for expression: captures.
// Note: null/nil defaults are not supported — use a typed value (false, "", 0).
type CaptureField struct {
	// Default value to use when the field is absent from the API response.
	// Only effective for field: captures; ignored for expression: captures.
	Default            interface{} `yaml:"default,omitempty"`
	Name               string      `yaml:"name" validate:"required"`
	FieldExpressionDef `yaml:",inline"`
}

// Condition represents a structured condition
type Condition struct {
	// Populated by UnmarshalYAML from "value" or "values"
	Value    interface{} `yaml:"-"`
	Field    string      `yaml:"field"`
	Operator string      `yaml:"operator" validate:"required,validoperator"`
}

// conditionRaw is used for custom unmarshaling to support both "value" and "values" keys
type conditionRaw struct {
	Value interface{} `yaml:"value"`
	// Alias for Value
	Values   interface{} `yaml:"values"`
	Field    string      `yaml:"field"`
	Operator string      `yaml:"operator"`
}

// UnmarshalYAML implements custom unmarshaling to support both "value" and "values" keys
func (c *Condition) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var raw conditionRaw
	if err := unmarshal(&raw); err != nil {
		return err
	}

	c.Field = raw.Field
	c.Operator = raw.Operator

	// Fail if both "value" and "values" are specified
	if raw.Value != nil && raw.Values != nil {
		return fmt.Errorf("condition has both 'value' and 'values' keys; use only one")
	}

	// Use whichever key is provided
	if raw.Values != nil {
		c.Value = raw.Values
	} else {
		c.Value = raw.Value
	}

	return nil
}

// TransportConfig specifies which transport client to use for a resource
type TransportConfig struct {
	// Maestro contains maestro-specific transport settings (required when Client is "maestro")
	Maestro *MaestroTransportConfig `yaml:"maestro,omitempty"`
	// Client is the transport client type: "kubernetes" or "maestro"
	Client string `yaml:"client" validate:"required,oneof=kubernetes maestro"`
}

// MaestroTransportConfig contains maestro-specific transport settings
type MaestroTransportConfig struct {
	// TargetCluster is the name of the target cluster (consumer) for ManifestWork delivery
	TargetCluster string `yaml:"target_cluster" validate:"required"`
}

// Resource represents a resource configuration.
// The manifest field holds either a K8s resource (for kubernetes transport)
// or a ManifestWork (for maestro transport). The transport client determines
// how to parse and apply it.
type Resource struct {
	Name      string           `yaml:"name" validate:"required,resourcename"`
	Transport *TransportConfig `yaml:"transport,omitempty"`
	Manifest  interface{}      `yaml:"manifest,omitempty"`
	Discovery *DiscoveryConfig `yaml:"discovery,omitempty" validate:"required"`
	// NestedDiscoveries defines how to discover individual sub-resources
	// within the applied manifest. For example, discovering resources
	// inside a ManifestWork's workload.
	// Lifecycle defines the resource lifecycle behavior, including deletion triggers and policy.
	// If not set, the resource uses the default apply-only behavior.
	Lifecycle         *ResourceLifecycle `yaml:"lifecycle,omitempty"`
	NestedDiscoveries []NestedDiscovery  `yaml:"nested_discoveries,omitempty" validate:"dive"`
	RecreateOnChange  bool               `yaml:"recreate_on_change,omitempty"`
}

// ResourceLifecycle defines the lifecycle behavior for a resource.
type ResourceLifecycle struct {
	Delete *LifecycleDelete `yaml:"delete,omitempty"`
}

// LifecycleDelete defines the deletion behavior for a resource.
type LifecycleDelete struct {
	// When defines the CEL expression that determines when to delete the resource.
	When *LifecycleWhen `yaml:"when,omitempty"`
	// PropagationPolicy is the Kubernetes deletion propagation policy: Background (default), Foreground, Orphan.
	// For Maestro transport, this is ignored — ManifestWork handles its own cleanup semantics.
	PropagationPolicy string `yaml:"propagationPolicy,omitempty"`
}

// LifecycleWhen defines the condition for when deletion should occur.
type LifecycleWhen struct {
	// Expression is a CEL expression evaluated each reconciliation loop.
	// The resource is deleted only when the expression evaluates to true.
	// Required when lifecycle.delete is configured.
	Expression string `yaml:"expression,omitempty"`
}

// NestedDiscovery defines a named discovery for a sub-resource within the parent manifest.
type NestedDiscovery struct {
	Discovery *DiscoveryConfig `yaml:"discovery" validate:"required"`
	Name      string           `yaml:"name" validate:"required,resourcename"`
}

// DiscoveryConfig represents resource discovery configuration
type DiscoveryConfig struct {
	BySelectors *SelectorConfig `yaml:"by_selectors,omitempty" validate:"required_without=ByName,excluded_with=ByName"`
	Namespace   string          `yaml:"namespace,omitempty"`
	//nolint:lll
	ByName string `yaml:"by_name,omitempty" validate:"required_without=BySelectors,excluded_with=BySelectors"`
}

// SelectorConfig represents label selector configuration
type SelectorConfig struct {
	LabelSelector map[string]string `yaml:"label_selector,omitempty" validate:"required,min=1"`
}

// PostConfig represents post-processing configuration
type PostConfig struct {
	Payloads    []Payload    `yaml:"payloads,omitempty" validate:"dive"`
	PostActions []PostAction `yaml:"post_actions,omitempty" validate:"dive"`
}

// PostAction represents a post-processing action
//
//nolint:govet // fieldalignment: padding is insignificant for a config struct loaded once at startup; keeping ActionBase first maintains consistency with Precondition
type PostAction struct {
	ActionBase `yaml:",inline"`
	// When defines a CEL expression that gates execution of this post-action.
	// If the expression evaluates to false, the action is skipped (not failed).
	// Follows the same nested pattern as lifecycle.delete.when for consistency.
	When *PostActionWhen `yaml:"when,omitempty"`
}

// PostActionWhen defines the condition for when a post-action should execute.
type PostActionWhen struct {
	// Expression is a CEL expression evaluated against the execution context.
	// The post-action executes only when the expression evaluates to true.
	// Available variables: adapter.resourcesSkipped, adapter.skipReason, params, resources.
	Expression string `yaml:"expression" validate:"required"`
}

// LogAction represents a logging action that can be configured in the adapter config
type LogAction struct {
	Message string `yaml:"message"`
	Level   string `yaml:"level,omitempty"` // debug, info, warning, error (default: info)
}

// ManifestRef represents a manifest reference
type ManifestRef struct {
	Ref string `yaml:"ref"`
}

// -----------------------------------------------------------------------------
// Validation Errors
// -----------------------------------------------------------------------------

// ValidationError represents a validation error with context
type ValidationError struct {
	Path    string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// ValidationErrors holds multiple validation errors
type ValidationErrors struct {
	Errors []ValidationError
}

func (ve *ValidationErrors) Error() string {
	if len(ve.Errors) == 0 {
		return "no validation errors"
	}
	var msgs []string
	for _, e := range ve.Errors {
		msgs = append(msgs, e.Error())
	}
	return fmt.Sprintf("validation failed with %d error(s):\n  - %s",
		len(ve.Errors), strings.Join(msgs, "\n  - "))
}

func (ve *ValidationErrors) Add(path, message string) {
	ve.Errors = append(ve.Errors, ValidationError{Path: path, Message: message})
}

// Extend appends all errors from another ValidationErrors
func (ve *ValidationErrors) Extend(other *ValidationErrors) {
	if other != nil {
		ve.Errors = append(ve.Errors, other.Errors...)
	}
}

// First returns the first error message, or empty string if no errors
func (ve *ValidationErrors) First() string {
	if len(ve.Errors) == 0 {
		return ""
	}
	return ve.Errors[0].Message
}

// Count returns the number of errors
func (ve *ValidationErrors) Count() int {
	return len(ve.Errors)
}

func (ve *ValidationErrors) HasErrors() bool {
	return len(ve.Errors) > 0
}

// AdapterConfig represents the deployment-level configuration.
// Contains infrastructure settings that can be overridden via environment variables
// and CLI flags using Viper.
type AdapterConfig struct {
	Adapter     AdapterInfo   `yaml:"adapter" mapstructure:"adapter"`
	Log         LogConfig     `yaml:"log,omitempty" mapstructure:"log"`
	Clients     ClientsConfig `yaml:"clients" mapstructure:"clients"`
	DebugConfig bool          `yaml:"debug_config,omitempty" mapstructure:"debug_config"`
}

// ClientsConfig contains configuration for all external clients
type ClientsConfig struct {
	Maestro       *MaestroClientConfig `yaml:"maestro,omitempty" mapstructure:"maestro"`
	Broker        BrokerConfig         `yaml:"broker,omitempty" mapstructure:"broker"`
	Kubernetes    KubernetesConfig     `yaml:"kubernetes" mapstructure:"kubernetes"`
	HyperfleetAPI HyperfleetAPIConfig  `yaml:"hyperfleet_api" mapstructure:"hyperfleet_api"`
}

// MaestroClientConfig contains Maestro client configuration
type MaestroClientConfig struct {
	GRPCServerAddress string `yaml:"grpc_server_address" mapstructure:"grpc_server_address"`
	HTTPServerAddress string `yaml:"http_server_address" mapstructure:"http_server_address"`
	SourceID          string `yaml:"source_id" mapstructure:"source_id"`
	ClientID          string `yaml:"client_id" mapstructure:"client_id"`
	Timeout           string `yaml:"timeout" mapstructure:"timeout"`
	//nolint:lll
	ServerHealthinessTimeout string            `yaml:"server_healthiness_timeout,omitempty" mapstructure:"server_healthiness_timeout"`
	Keepalive                *KeepaliveConfig  `yaml:"keepalive,omitempty" mapstructure:"keepalive"`
	Auth                     MaestroAuthConfig `yaml:"auth" mapstructure:"auth"`
	RetryAttempts            int               `yaml:"retry_attempts" mapstructure:"retry_attempts"`
	Insecure                 bool              `yaml:"insecure,omitempty" mapstructure:"insecure"`
}

// MaestroAuthConfig contains authentication configuration for Maestro
type MaestroAuthConfig struct {
	TLSConfig *TLSConfig `yaml:"tls_config,omitempty" mapstructure:"tls_config"`
	// "tls" or "none"
	Type string `yaml:"type" mapstructure:"type"`
}

// TLSConfig contains TLS certificate configuration
type TLSConfig struct {
	CAFile     string `yaml:"ca_file" mapstructure:"ca_file"`
	CertFile   string `yaml:"cert_file" mapstructure:"cert_file"`
	KeyFile    string `yaml:"key_file" mapstructure:"key_file"`
	HTTPCAFile string `yaml:"http_ca_file,omitempty" mapstructure:"http_ca_file"`
}

// KeepaliveConfig contains gRPC keepalive configuration
type KeepaliveConfig struct {
	Time    string `yaml:"time" mapstructure:"time"`
	Timeout string `yaml:"timeout" mapstructure:"timeout"`
}

// AdapterTaskConfig represents the business logic configuration.
// Contains params, preconditions, resources, and post-processing actions.
// This config is loaded from YAML without environment variable overrides.
type AdapterTaskConfig struct {
	Post          *PostConfig    `yaml:"post,omitempty" validate:"omitempty"`
	Params        []Parameter    `yaml:"params,omitempty" validate:"dive"`
	Preconditions []Precondition `yaml:"preconditions,omitempty" validate:"dive"`
	Resources     []Resource     `yaml:"resources,omitempty" validate:"unique=Name,dive"`
}
