package configloader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/utils"
	"gopkg.in/yaml.v3"
)

// -----------------------------------------------------------------------------
// Constants
// -----------------------------------------------------------------------------

// Environment variable for config file paths
const (
	EnvAdapterConfig  = "HYPERFLEET_ADAPTER_CONFIG" // Path to deployment config
	EnvTaskConfigPath = "HYPERFLEET_TASK_CONFIG"    // Path to task config
)

// ValidHTTPMethods defines allowed HTTP methods for API calls
var ValidHTTPMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}

// -----------------------------------------------------------------------------
// Load Options (Functional Options Pattern)
// -----------------------------------------------------------------------------

// LoadOption configures the loading behavior
type LoadOption func(*loadOptions)

type loadOptions struct {
	flags                  interface{} // *pflag.FlagSet
	ctx                    context.Context
	logger                 logger.Logger
	adapterConfigPath      string
	taskConfigPath         string
	adapterVersion         string
	skipSemanticValidation bool
}

// WithAdapterConfigPath sets the path to the deployment config file
func WithAdapterConfigPath(path string) LoadOption {
	return func(o *loadOptions) {
		o.adapterConfigPath = path
	}
}

// WithTaskConfigPath sets the path to the task config file
func WithTaskConfigPath(path string) LoadOption {
	return func(o *loadOptions) {
		o.taskConfigPath = path
	}
}

// WithFlags sets the CLI flags for Viper binding
func WithFlags(flags interface{}) LoadOption {
	return func(o *loadOptions) {
		o.flags = flags
	}
}

// WithAdapterVersion validates config against expected adapter version
func WithAdapterVersion(version string) LoadOption {
	return func(o *loadOptions) {
		o.adapterVersion = version
	}
}

// WithSkipSemanticValidation skips CEL, template, and K8s manifest validation
func WithSkipSemanticValidation() LoadOption {
	return func(o *loadOptions) {
		o.skipSemanticValidation = true
	}
}

// WithContext sets the context for logging during config loading
func WithContext(ctx context.Context) LoadOption {
	return func(o *loadOptions) {
		o.ctx = ctx
	}
}

// WithLogger sets the logger for config loading
func WithLogger(l logger.Logger) LoadOption {
	return func(o *loadOptions) {
		o.logger = l
	}
}

// -----------------------------------------------------------------------------
// Public API
// -----------------------------------------------------------------------------

// LoadConfig loads both deployment and task configurations, validates them,
// and returns a unified Config struct.
// Priority for deployment config values: CLI flags > Environment variables > Config file > Defaults
func LoadConfig(opts ...LoadOption) (*Config, error) {
	o := &loadOptions{}
	for _, opt := range opts {
		opt(o)
	}
	if o.ctx == nil {
		o.ctx = context.Background()
	}
	if o.logger == nil {
		var logErr error
		o.logger, logErr = logger.NewLogger(logger.DefaultConfig())
		if logErr != nil {
			return nil, fmt.Errorf("failed to create logger: %w", logErr)
		}
	}

	// 1. Load AdapterConfig with Viper (env/CLI overrides)
	// resolvedAdapterConfigPath is the actual path used (may come from standardConfigPaths fallback)
	resolvedAdapterConfigPath, adapterCfg, err := loadAdapterConfigWithViperGeneric(o.adapterConfigPath, o.flags)
	if err != nil {
		return nil, fmt.Errorf("failed to load adapter config: %w", err)
	}

	// Get base directory from the resolved config path for file references
	adapterBaseDir, errBaseDir := getBaseDir(resolvedAdapterConfigPath)
	if errBaseDir != nil {
		return nil, fmt.Errorf("failed to get base directory for adapter config: %w", errBaseDir)
	}

	// Validate AdapterConfig structure
	adapterValidator := NewAdapterConfigValidator(adapterCfg, adapterBaseDir)
	if err = adapterValidator.ValidateStructure(); err != nil {
		return nil, fmt.Errorf("adapter config validation failed: %w", err)
	}

	// Validate adapter version if specified
	if o.adapterVersion != "" {
		if err = ValidateAdapterVersion(o.ctx, o.logger, adapterCfg, o.adapterVersion); err != nil {
			return nil, fmt.Errorf("adapter version validation failed: %w", err)
		}
	}

	// 2. Load AdapterTaskConfig from YAML (no env binding)
	taskCfg, err := loadTaskConfig(o.taskConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load task config: %w", err)
	}

	// Get base directory from task config path
	taskConfigPath := o.taskConfigPath
	if taskConfigPath == "" {
		taskConfigPath = os.Getenv(EnvTaskConfigPath)
	}
	taskBaseDir := ""
	if taskConfigPath != "" {
		var errBaseDir error
		taskBaseDir, errBaseDir = getBaseDir(taskConfigPath)
		if errBaseDir != nil {
			return nil, fmt.Errorf("failed to get base directory for task config: %w", errBaseDir)
		}
	}

	// Validate AdapterTaskConfig structure
	taskValidator := NewTaskConfigValidator(taskCfg, taskBaseDir)
	if err := taskValidator.ValidateStructure(); err != nil {
		return nil, fmt.Errorf("task config validation failed: %w", err)
	}

	// Validate and load file references in task config
	if taskBaseDir != "" {
		if err := taskValidator.ValidateFileReferences(); err != nil {
			return nil, fmt.Errorf("task config file reference validation failed: %w", err)
		}

		if err := loadTaskConfigFileReferences(taskCfg, taskBaseDir); err != nil {
			return nil, fmt.Errorf("failed to load task config file references: %w", err)
		}
	}

	// Semantic validation for task config (optional)
	if !o.skipSemanticValidation {
		if err := taskValidator.ValidateSemantic(); err != nil {
			return nil, fmt.Errorf("task config semantic validation failed: %w", err)
		}
		for _, w := range taskValidator.Warnings() {
			o.logger.Warn(o.ctx, w)
		}
	}

	// 3. Merge into unified Config
	config := Merge(adapterCfg, taskCfg)
	if config == nil {
		return nil, fmt.Errorf("failed to merge configurations")
	}

	return config, nil
}

// -----------------------------------------------------------------------------
// Internal Functions
// -----------------------------------------------------------------------------

// loadTaskConfigFileReferences loads content from file references into the task config
func loadTaskConfigFileReferences(config *AdapterTaskConfig, baseDir string) error {
	// Load manifest.ref in resources as raw strings to support Go template syntax.
	// Files are stored as raw strings so that structural Go templates ({{ if }}, {{ range }}, etc.)
	// are preserved and rendered at execution time before YAML parsing.
	for i := range config.Resources {
		resource := &config.Resources[i]
		ref := resource.GetManifestRef()
		if ref == "" {
			continue
		}

		content, err := loadRawFile(baseDir, ref)
		if err != nil {
			return fmt.Errorf("%s[%d].%s.%s: %w", FieldResources, i, FieldManifest, FieldRef, err)
		}

		// Replace manifest with raw string content for template rendering at execution time
		resource.Manifest = content
	}

	// Load buildRef in post.payloads
	if config.Post != nil {
		for i := range config.Post.Payloads {
			payload := &config.Post.Payloads[i]
			if payload.BuildRef != "" {
				content, err := loadYAMLFile(baseDir, payload.BuildRef)
				if err != nil {
					return fmt.Errorf("%s.%s[%d].%s: %w", FieldPost, FieldPayloads, i, FieldBuildRef, err)
				}
				payload.BuildRefContent = content
			}
		}
	}

	return nil
}

// loadRawFile reads a file and returns its content as a raw string.
// Used for manifest ref files to preserve Go template syntax for later rendering.
func loadRawFile(baseDir, refPath string) (string, error) {
	fullPath, err := resolvePath(baseDir, refPath)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(filepath.Clean(fullPath))
	if err != nil {
		return "", fmt.Errorf("failed to read file %q: %w", fullPath, err)
	}

	return string(data), nil
}

// loadYAMLFile loads and parses a YAML file
func loadYAMLFile(baseDir, refPath string) (map[string]interface{}, error) {
	fullPath, err := resolvePath(baseDir, refPath)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filepath.Clean(fullPath))
	if err != nil {
		return nil, fmt.Errorf("failed to read file %q: %w", fullPath, err)
	}

	var content map[string]interface{}
	if err := yaml.Unmarshal(data, &content); err != nil {
		return nil, fmt.Errorf("failed to parse YAML file %q: %w", fullPath, err)
	}

	return content, nil
}

// resolvePath resolves a relative path against the base directory and validates
// that the resolved path does not escape the base directory.
// This delegates to utils.ResolveSecurePath.
func resolvePath(baseDir, refPath string) (string, error) {
	return utils.ResolveSecurePath(baseDir, refPath)
}
