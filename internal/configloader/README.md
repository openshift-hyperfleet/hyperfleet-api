# Config Loader

The `config_loader` package loads and validates HyperFleet Adapter configuration files (YAML format).

## Features

- **YAML Parsing**: Load configurations from files or bytes
- **Structural Validation**: Required fields, formats, enums via `go-playground/validator`
- **Semantic Validation**: CEL expressions, template variables, K8s manifests
- **Type Safety**: Strongly-typed Go structs with struct embedding
- **Helper Methods**: Query params, resources, preconditions by name

## Package Structure

| File | Purpose |
|------|---------|
| `loader.go` | Load configs from file/bytes, resolve file references |
| `types.go` | All type definitions with validation tags |
| `validator.go` | Orchestrates structural + semantic validation |
| `struct_validator.go` | `go-playground/validator` integration |
| `accessors.go` | Helper methods for querying config |
| `constants.go` | Field names, API versions, regex patterns |

## Usage

```go
import "github.com/openshift-hyperfleet/hyperfleet-adapter/internal/configloader"

// Load from file (or set ADAPTER_CONFIG_PATH env var)
config, err := config_loader.Load("path/to/config.yaml")

// With adapter version validation
config, err := config_loader.Load("config.yaml", config_loader.WithAdapterVersion("1.0.0"))
```

### Accessing Configuration

```go
// Metadata
config.Adapter.Name

// API config
timeout := config.Clients.HyperfleetAPI.Timeout

// Query helpers
config.GetRequiredParams()
config.GetResourceByName("clusterNamespace")
config.GetPreconditionByName("clusterStatus")
config.GetPostActionByName("reportStatus")
```

## Configuration Structure

```yaml
# adapter-config.yaml (deployment config)
adapter:
  name: example-adapter
  version: "0.1.0"
clients:
  hyperfleet_api:
    timeout: 2s
    retry_attempts: 3
    retry_backoff: exponential

# adapter-task-config.yaml (task config — merged at runtime)
params: [...]
preconditions: [...]
resources: [...]
post: {...}
```

See `configs/adapter-task-config-template.yaml` for the complete configuration reference.

## Validation

### Two-Phase Validation

1. **Structural Validation** (`ValidateStructure`)
   - Uses `go-playground/validator` with struct tags
   - Required fields, enum values, mutual exclusivity
   - Custom validators: `resourcename`, `validoperator`

2. **Semantic Validation** (`ValidateSemantic`)
   - CEL expression syntax
   - Template variable references
   - Condition value types
   - K8s manifest required fields

### Validation Tags

```go
// Required field
Name string `yaml:"name" validate:"required"`

// Enum validation
Method string `yaml:"method" validate:"required,oneof=GET POST PUT PATCH DELETE"`

// Mutual exclusivity (field OR expression, not both)
Field      string `yaml:"field,omitempty" validate:"required_without=Expression,excluded_with=Expression"`
Expression string `yaml:"expression,omitempty" validate:"required_without=Field,excluded_with=Field"`

// Custom validators
Name     string `yaml:"name" validate:"required,resourcename"`
Operator string `yaml:"operator" validate:"required,validoperator"`
```

### Custom Validators

| Tag | Purpose |
|-----|---------|
| `resourcename` | CEL-compatible names (lowercase start, no hyphens) |
| `validoperator` | Valid condition operators (eq, neq, in, notIn, exists) |

### Error Messages

Validation errors are descriptive:

```text
params[0].name is required
preconditions[1].api_call.method "INVALID" is invalid (allowed: GET, POST, PUT, PATCH, DELETE)
resources[0].name "my-resource": must start with lowercase letter and contain only letters, numbers, underscores (no hyphens)
preconditions[0].capture[0]: must have either 'field' or 'expression' set
```

## Types

| Type | Description |
|------|-------------|
| `AdapterConfig` | Top-level configuration |
| `Parameter` | Parameter extraction config |
| `Precondition` | Pre-check with API call and conditions |
| `Resource` | K8s resource with manifest and discovery |
| `PostConfig` | Post-processing actions |
| `APICall` | HTTP request configuration |
| `Condition` | Field/operator/value condition |
| `CaptureField` | Field capture from API response |
| `ValueDef` | Dynamic value definition in payload builds |
| `ValidationErrors` | Collection of validation errors |

### Struct Embedding

The package uses struct embedding to reduce duplication:

```go
// ActionBase - common fields for actions (preconditions, post-actions)
type ActionBase struct {
    Name    string   `yaml:"name" validate:"required"`
    APICall *APICall `yaml:"api_call,omitempty"`
}

// FieldExpressionDef - field OR expression (mutually exclusive)
type FieldExpressionDef struct {
    Field      string `yaml:"field,omitempty" validate:"required_without=Expression,excluded_with=Expression"`
    Expression string `yaml:"expression,omitempty" validate:"required_without=Field,excluded_with=Field"`
}
```

### ValidationErrors

Collect and manage multiple validation errors:

```go
errors := &ValidationErrors{}
errors.Add("path.to.field", "error message")
errors.Extend(otherErrors)  // Merge from another ValidationErrors

if errors.HasErrors() {
    fmt.Println(errors.First())  // Get first error message
    fmt.Println(errors.Count())  // Number of errors
    return errors                // Implements error interface
}
```

### CaptureField

Captures values from API responses. Supports two modes (mutually exclusive):

| Field | Description |
|-------|-------------|
| `name` | Variable name for captured value (required) |
| `field` | Simple dot notation or JSONPath expression |
| `expression` | CEL expression for computed values |

```yaml
capture:
  # CEL expression for Reconciled condition status
  - name: "reconciledConditionStatus"
    expression: |
      status.conditions.filter(c, c.type == "Reconciled").size() > 0
        ? status.conditions.filter(c, c.type == "Reconciled")[0].status
        : "False"
  
  # JSONPath for complex extraction
  - name: "lzStatus"
    field: "{.items[?(@.adapter=='landing-zone-adapter')].data.namespace.status}"
  
  # CEL expression
  - name: "activeCount"
    expression: "items.filter(i, i.status == 'active').size()"
```

### ValueDef

Dynamic value definition for payload builds. Used when a field should be computed via field extraction (JSONPath) or CEL expression.

| Field | Description |
|-------|-------------|
| `field` | JSONPath/dot notation to extract value |
| `expression` | CEL expression to evaluate |
| `default` | Default value if extraction fails or returns nil |

```yaml
build:
  # Direct string (Go template supported)
  message: "Deployment successful"
  
  # Field extraction with default
  errorMessage:
    field: "adapter.errorMessage"
    default: ""
  
  # CEL expression with default
  isHealthy:
    expression: "resources.deployment.status.readyReplicas > 0"
    default: false
```

See `types.go` for complete definitions.

## Related

- `internal/criteria` - Evaluates conditions
- `internal/k8sclient` - Manages K8s resources
