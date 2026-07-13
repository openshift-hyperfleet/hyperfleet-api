package configloader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/google/cel-go/cel"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/criteria"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
)

// templateVarRegex matches Go template variables like {{ .varName }} or {{ .nested.var }}
var templateVarRegex = regexp.MustCompile(`\{\{\s*\.([a-zA-Z_][a-zA-Z0-9_\.]*)\s*(?:\|[^}]*)?\}\}`)

// -----------------------------------------------------------------------------
// Validators
// -----------------------------------------------------------------------------

// AdapterConfigValidator validates AdapterConfig (deployment configuration)
type AdapterConfigValidator struct {
	config  *AdapterConfig
	errors  *ValidationErrors
	baseDir string
}

// NewAdapterConfigValidator creates a validator for AdapterConfig
func NewAdapterConfigValidator(config *AdapterConfig, baseDir string) *AdapterConfigValidator {
	return &AdapterConfigValidator{
		config:  config,
		baseDir: baseDir,
		errors:  &ValidationErrors{},
	}
}

// ValidateStructure validates the structural requirements of AdapterConfig
func (v *AdapterConfigValidator) ValidateStructure() error {
	if v.config == nil {
		return fmt.Errorf("adapter config is nil")
	}

	// Struct tag validation
	if errs := ValidateStruct(v.config); errs != nil && errs.HasErrors() {
		return fmt.Errorf("%s", errs.First())
	}

	if err := v.validateHyperfleetAuth(); err != nil {
		return err
	}

	return nil
}

func (v *AdapterConfigValidator) validateHyperfleetAuth() error {
	auth := v.config.Clients.HyperfleetAPI.Auth
	if auth == nil {
		return nil
	}
	if auth.TokenPath == "" {
		return fmt.Errorf("clients.hyperfleet_api.auth.token_path must be set when auth is configured")
	}
	if !filepath.IsAbs(auth.TokenPath) {
		return fmt.Errorf("clients.hyperfleet_api.auth.token_path must be an absolute path, got %q", auth.TokenPath)
	}
	if auth.TokenCacheTTL < 0 {
		return fmt.Errorf("clients.hyperfleet_api.auth.token_cache_ttl must not be negative")
	}
	return nil
}

// TaskConfigValidator validates AdapterTaskConfig (task configuration)
type TaskConfigValidator struct {
	config      *AdapterTaskConfig
	errors      *ValidationErrors
	celEnv      *cel.Env
	definedVars map[string]bool
	baseDir     string
	warnings    []string
}

// NewTaskConfigValidator creates a validator for AdapterTaskConfig
func NewTaskConfigValidator(config *AdapterTaskConfig, baseDir string) *TaskConfigValidator {
	return &TaskConfigValidator{
		config:  config,
		baseDir: baseDir,
		errors:  &ValidationErrors{},
	}
}

// Warnings returns deprecation warnings collected during validation.
func (v *TaskConfigValidator) Warnings() []string {
	return v.warnings
}

// ValidateStructure validates the structural requirements of AdapterTaskConfig
func (v *TaskConfigValidator) ValidateStructure() error {
	if v.config == nil {
		return fmt.Errorf("task config is nil")
	}

	// Struct tag validation
	if errs := ValidateStruct(v.config); errs != nil && errs.HasErrors() {
		return fmt.Errorf("%s", errs.First())
	}

	return nil
}

// ValidateFileReferences validates that all file references in the task config exist
func (v *TaskConfigValidator) ValidateFileReferences() error {
	if v.baseDir == "" {
		return nil
	}

	var errors []string

	// Validate build_ref in post.payloads
	if v.config.Post != nil {
		for i, payload := range v.config.Post.Payloads {
			if payload.BuildRef != "" {
				path := fmt.Sprintf("%s.%s[%d].%s", FieldPost, FieldPayloads, i, FieldBuildRef)
				if err := v.validateFileExists(payload.BuildRef, path); err != nil {
					errors = append(errors, err.Error())
				}
			}
		}
	}

	// Validate manifest.ref in resources
	for i, resource := range v.config.Resources {
		ref := resource.GetManifestRef()
		if ref != "" {
			path := fmt.Sprintf("%s[%d].%s.%s", FieldResources, i, FieldManifest, FieldRef)
			if err := v.validateFileExists(ref, path); err != nil {
				errors = append(errors, err.Error())
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("file reference errors:\n  - %s", strings.Join(errors, "\n  - "))
	}
	return nil
}

func (v *TaskConfigValidator) validateFileExists(refPath, configPath string) error {
	if refPath == "" {
		return fmt.Errorf("%s: file reference is empty", configPath)
	}

	fullPath, err := resolvePath(v.baseDir, refPath)
	if err != nil {
		return fmt.Errorf("%s: %w", configPath, err)
	}

	info, err := os.Stat(filepath.Clean(fullPath))
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s: referenced file %q does not exist (resolved to %q)", configPath, refPath, fullPath)
		}
		return fmt.Errorf("%s: error checking file %q: %w", configPath, refPath, err)
	}

	if info.IsDir() {
		return fmt.Errorf("%s: referenced path %q is a directory, not a file", configPath, refPath)
	}

	return nil
}

// ValidateSemantic performs semantic validation on the task config
func (v *TaskConfigValidator) ValidateSemantic() error {
	if v.config == nil {
		return fmt.Errorf("config is nil")
	}

	// Initialize validation context
	v.collectDefinedVariables()
	if err := v.initCELEnv(); err != nil {
		v.errors.Add("cel", fmt.Sprintf("failed to create CEL environment: %v", err))
	}

	// Run all semantic validators
	v.validatePreconditionAPICallForbidden()
	v.validateParamSources()
	v.validateParamAPICallTemplates()
	v.validateParamFileSources()
	v.validateTransportConfig()
	v.validateConditionValues()
	v.validateCaptureFieldExpressions()
	v.validateTemplateVariables()
	v.validateCELExpressions()
	v.validateK8sManifests()
	v.validateLifecycleConfig()

	if v.errors.HasErrors() {
		return v.errors
	}
	return nil
}

func (v *TaskConfigValidator) validatePreconditionAPICallForbidden() {
	for i, precond := range v.config.Preconditions {
		if precond.APICall != nil {
			path := fmt.Sprintf("%s[%d].%s", FieldPreconditions, i, FieldAPICall)
			v.warnings = append(v.warnings, fmt.Sprintf(
				"%s: DEPRECATED: precondition %q uses api_call directly. "+
					"Move the api_call block to a params entry with source.api_call instead. "+
					"Direct api_call on preconditions will be removed in a future release.\n"+
					"  params:\n"+
					"    - name: %q\n"+
					"      source:\n"+
					"        api_call:\n"+
					"          ...",
				path, precond.Name, precond.Name))
		}
	}
}

func (v *TaskConfigValidator) validateParamSources() {
	for i, param := range v.config.Params {
		if param.Source.IsZero() || (param.Source.IsString() && strings.TrimSpace(param.Source.StringVal) == "") {
			v.errors.Add(fmt.Sprintf("%s[%d].%s", FieldParams, i, FieldSource), "source is required")
		}
	}
}

func (v *TaskConfigValidator) validateParamFileSources() {
	for i, param := range v.config.Params {
		if !param.Source.IsFile() {
			continue
		}
		base := fmt.Sprintf("%s[%d].%s.file", FieldParams, i, FieldSource)
		if param.Source.File == nil || param.Source.File.Path == "" {
			v.errors.Add(base+".path", "path is required for file source")
			continue
		}
		if !strings.HasPrefix(param.Source.File.Path, "/") {
			v.warnings = append(v.warnings, fmt.Sprintf(
				"%s.path: file source path %q is not absolute; in container deployments, use absolute paths for mounted volumes",
				base, param.Source.File.Path))
		}
	}
}

func (v *TaskConfigValidator) validateParamAPICallTemplates() {
	available := make(map[string]bool)
	for _, b := range BuiltinVariables() {
		available[b] = true
	}

	for i, param := range v.config.Params {
		if param.Source.IsAPICall() && param.Source.APICall != nil {
			ac := param.Source.APICall
			base := fmt.Sprintf("%s[%d].%s.%s", FieldParams, i, FieldSource, FieldAPICall)
			v.validateTemplateStringWithVars(ac.URL, base+"."+FieldURL, available)
			v.validateTemplateStringWithVars(ac.Body, base+"."+FieldBody, available)
			for j, h := range ac.Headers {
				v.validateTemplateStringWithVars(h.Value,
					fmt.Sprintf("%s.%s[%d].%s", base, FieldHeaders, j, FieldHeaderValue), available)
			}
		}
		if param.Name != "" {
			available[param.Name] = true
		}
	}
}

func (v *TaskConfigValidator) validateTemplateStringWithVars(s, path string, vars map[string]bool) {
	if s == "" {
		return
	}
	matches := templateVarRegex.FindAllStringSubmatch(s, -1)
	for _, match := range matches {
		if len(match) > 1 {
			varName := match[1]
			if !v.isVariableDefinedIn(varName, vars) {
				v.errors.Add(path, fmt.Sprintf("undefined template variable %q", varName))
			}
		}
	}
}

func (v *TaskConfigValidator) isVariableDefinedIn(varName string, vars map[string]bool) bool {
	if vars[varName] {
		return true
	}
	parts := strings.Split(varName, ".")
	if len(parts) > 0 && vars[parts[0]] {
		return true
	}
	return false
}

func (v *TaskConfigValidator) collectDefinedVariables() {
	v.definedVars = v.config.GetDefinedVariables()
}

// GetDefinedVariables returns all variables defined in the task config
func (c *AdapterTaskConfig) GetDefinedVariables() map[string]bool {
	vars := make(map[string]bool)

	if c == nil {
		return vars
	}

	// Built-in variables
	for _, b := range BuiltinVariables() {
		vars[b] = true
	}

	// Parameters from params
	for _, p := range c.Params {
		if p.Name != "" {
			vars[p.Name] = true
		}
	}

	// Variables from precondition captures
	for _, precond := range c.Preconditions {
		for _, capture := range precond.Capture {
			if capture.Name != "" {
				vars[capture.Name] = true
			}
		}
	}

	// Post payloads
	if c.Post != nil {
		for _, p := range c.Post.Payloads {
			if p.Name != "" {
				vars[p.Name] = true
			}
		}
	}

	// Resource aliases
	for _, r := range c.Resources {
		if r.Name != "" {
			vars[FieldResources+"."+r.Name] = true
		}
	}

	return vars
}

func (v *TaskConfigValidator) initCELEnv() error {
	options := make([]cel.EnvOption, 0, len(v.definedVars)+2)
	options = append(options, cel.OptionalTypes())

	addedRoots := make(map[string]bool)

	for varName := range v.definedVars {
		root := varName
		if idx := strings.Index(varName, "."); idx > 0 {
			root = varName[:idx]
		}

		if addedRoots[root] {
			continue
		}
		addedRoots[root] = true

		options = append(options, cel.Variable(root, cel.DynType))
	}

	if !addedRoots[FieldResources] {
		options = append(options, cel.Variable(FieldResources, cel.MapType(cel.StringType, cel.DynType)))
	}

	if !addedRoots[FieldAdapter] {
		options = append(options, cel.Variable(FieldAdapter, cel.MapType(cel.StringType, cel.DynType)))
	}

	env, err := cel.NewEnv(options...)
	if err != nil {
		return err
	}
	v.celEnv = env
	return nil
}

func (v *TaskConfigValidator) validateTransportConfig() {
	for i, resource := range v.config.Resources {
		basePath := fmt.Sprintf("%s[%d]", FieldResources, i)

		if resource.Transport != nil {
			transportPath := basePath + "." + FieldTransport

			// Validate client type
			client := resource.Transport.Client
			if client != TransportClientKubernetes && client != TransportClientMaestro {
				v.errors.Add(transportPath+"."+FieldClient,
					fmt.Sprintf("unsupported transport client %q (supported: %s, %s)",
						client, TransportClientKubernetes, TransportClientMaestro))
				continue
			}

			if client == TransportClientMaestro {
				// Maestro transport requires maestro config
				if resource.Transport.Maestro == nil {
					v.errors.Add(transportPath,
						"maestro transport config is required when client is \"maestro\"")
					continue
				}

				maestroPath := transportPath + "." + TransportClientMaestro

				// Validate target_cluster is set
				if resource.Transport.Maestro.TargetCluster == "" {
					v.errors.Add(maestroPath+"."+FieldTargetCluster,
						"target_cluster is required for maestro transport")
				} else {
					// Validate template variables in target_cluster
					v.validateTemplateString(resource.Transport.Maestro.TargetCluster,
						maestroPath+"."+FieldTargetCluster)
				}

				// Validate manifest is set for maestro transport
				if resource.Manifest == nil {
					v.errors.Add(basePath+"."+FieldManifest,
						"manifest is required for maestro transport")
				}
			}
		}

		// Validate manifest is required for kubernetes transport (default)
		if resource.GetTransportClient() == TransportClientKubernetes && resource.Manifest == nil {
			v.errors.Add(basePath+"."+FieldManifest,
				"manifest is required for kubernetes transport")
		}
	}
}

func (v *TaskConfigValidator) validateConditionValues() {
	for i, precond := range v.config.Preconditions {
		for j, cond := range precond.Conditions {
			path := fmt.Sprintf("%s[%d].%s[%d]", FieldPreconditions, i, FieldConditions, j)
			v.validateConditionValue(cond.Operator, cond.Value, path)
		}
	}
}

func (v *TaskConfigValidator) validateConditionValue(operator string, value interface{}, path string) {
	op := criteria.Operator(operator)

	if op == criteria.OperatorExists {
		if value != nil {
			v.errors.Add(path, fmt.Sprintf("value/values should not be set for operator \"%s\"", operator))
		}
		return
	}

	if value == nil {
		v.errors.Add(path, fmt.Sprintf("value is required for operator %q", operator))
		return
	}

	if op == criteria.OperatorIn || op == criteria.OperatorNotIn {
		if !isSliceOrArray(value) {
			v.errors.Add(path, fmt.Sprintf("value must be a list for operator %q", operator))
		}
	}
}

func (v *TaskConfigValidator) validateCaptureFieldExpressions() {
	for i, precond := range v.config.Preconditions {
		for j, capture := range precond.Capture {
			if capture.Expression != "" && v.celEnv != nil {
				path := fmt.Sprintf("%s[%d].%s[%d].%s", FieldPreconditions, i, FieldCapture, j, FieldExpression)
				v.validateCELExpression(capture.Expression, path)
			}
		}
	}
}

func (v *TaskConfigValidator) validateTemplateVariables() {
	// Validate precondition API call URLs and bodies
	for i, precond := range v.config.Preconditions {
		if precond.APICall != nil {
			basePath := fmt.Sprintf("%s[%d].%s", FieldPreconditions, i, FieldAPICall)
			v.validateTemplateString(precond.APICall.URL, basePath+"."+FieldURL)
			v.validateTemplateString(precond.APICall.Body, basePath+"."+FieldBody)
			for j, header := range precond.APICall.Headers {
				v.validateTemplateString(header.Value,
					fmt.Sprintf("%s.%s[%d].%s", basePath, FieldHeaders, j, FieldHeaderValue))
			}
		}
	}

	// Validate resource manifests and transport config templates
	// All manifests are validated as template strings — map manifests are serialized
	// to YAML first since they are rendered as Go templates at execution time.
	for i, resource := range v.config.Resources {
		resourcePath := fmt.Sprintf("%s[%d]", FieldResources, i)
		manifestStr, err := manifest.ToYAMLString(resource.Manifest)
		if err == nil && manifestStr != "" {
			v.validateTemplateString(manifestStr, resourcePath+"."+FieldManifest)
		}
		// NOTE: For maestro transport, we skip template variable validation for manifest content.
		// ManifestWork templates may use variables provided at runtime by the framework
		// (e.g., adapterName, timestamp) that are not necessarily declared in params or captures.
		if resource.Discovery != nil {
			discoveryPath := resourcePath + "." + FieldDiscovery
			v.validateTemplateString(resource.Discovery.Namespace, discoveryPath+"."+FieldNamespace)
			v.validateTemplateString(resource.Discovery.ByName, discoveryPath+"."+FieldByName)
			if resource.Discovery.BySelectors != nil {
				for k, val := range resource.Discovery.BySelectors.LabelSelector {
					v.validateTemplateString(val,
						fmt.Sprintf("%s.%s.%s[%s]", discoveryPath, FieldBySelectors, FieldLabelSelector, k))
				}
			}
		}
		// Validate nestedDiscoveries template variables
		for j, md := range resource.NestedDiscoveries {
			mdPath := fmt.Sprintf("%s.%s[%d].%s", resourcePath, FieldNestedDiscoveries, j, FieldDiscovery)
			if md.Discovery != nil {
				v.validateTemplateString(md.Discovery.Namespace, mdPath+"."+FieldNamespace)
				v.validateTemplateString(md.Discovery.ByName, mdPath+"."+FieldByName)
				if md.Discovery.BySelectors != nil {
					for k, val := range md.Discovery.BySelectors.LabelSelector {
						v.validateTemplateString(val,
							fmt.Sprintf("%s.%s.%s[%s]", mdPath, FieldBySelectors, FieldLabelSelector, k))
					}
				}
			}
		}
	}

	// Validate post action API calls
	if v.config.Post != nil {
		for i, action := range v.config.Post.PostActions {
			if action.APICall != nil {
				basePath := fmt.Sprintf("%s.%s[%d].%s", FieldPost, FieldPostActions, i, FieldAPICall)
				v.validateTemplateString(action.APICall.URL, basePath+"."+FieldURL)
				v.validateTemplateString(action.APICall.Body, basePath+"."+FieldBody)
				for j, header := range action.APICall.Headers {
					v.validateTemplateString(header.Value,
						fmt.Sprintf("%s.%s[%d].%s", basePath, FieldHeaders, j, FieldHeaderValue))
				}
			}
		}

		// Validate post payload build value templates
		for i, payload := range v.config.Post.Payloads {
			if payload.Build != nil {
				if buildMap, ok := payload.Build.(map[string]interface{}); ok {
					v.validateTemplateMap(buildMap, fmt.Sprintf("%s.%s[%d].%s", FieldPost, FieldPayloads, i, FieldBuild))
				}
			}
		}
	}
}

func (v *TaskConfigValidator) validateTemplateString(s string, path string) {
	if s == "" {
		return
	}

	matches := templateVarRegex.FindAllStringSubmatch(s, -1)
	for _, match := range matches {
		if len(match) > 1 {
			varName := match[1]
			if !v.isVariableDefined(varName) {
				v.errors.Add(path, fmt.Sprintf("undefined template variable %q", varName))
			}
		}
	}
}

func (v *TaskConfigValidator) isVariableDefined(varName string) bool {
	if v.definedVars[varName] {
		return true
	}

	parts := strings.Split(varName, ".")
	if len(parts) > 0 {
		root := parts[0]

		if v.definedVars[root] {
			return true
		}

		if root == FieldResources && len(parts) > 1 {
			alias := root + "." + parts[1]
			if v.definedVars[alias] {
				return true
			}
		}
	}

	return false
}

func (v *TaskConfigValidator) validateTemplateMap(m map[string]interface{}, path string) {
	for key, value := range m {
		currentPath := fmt.Sprintf("%s.%s", path, key)
		switch val := value.(type) {
		case string:
			v.validateTemplateString(val, currentPath)
		case map[string]interface{}:
			v.validateTemplateMap(val, currentPath)
		case []interface{}:
			for i, item := range val {
				itemPath := fmt.Sprintf("%s[%d]", currentPath, i)
				if str, ok := item.(string); ok {
					v.validateTemplateString(str, itemPath)
				} else if m, ok := item.(map[string]interface{}); ok {
					v.validateTemplateMap(m, itemPath)
				}
			}
		}
	}
}

func (v *TaskConfigValidator) validateCELExpressions() {
	if v.celEnv == nil {
		return
	}

	for i, param := range v.config.Params {
		if param.Source.IsExpression() && param.Source.Expression != "" {
			path := fmt.Sprintf("%s[%d].%s.%s", FieldParams, i, FieldSource, FieldExpression)
			v.validateCELExpression(param.Source.Expression, path)
		}
	}

	for i, precond := range v.config.Preconditions {
		if precond.Expression != "" {
			path := fmt.Sprintf("%s[%d].%s", FieldPreconditions, i, FieldExpression)
			v.validateCELExpression(precond.Expression, path)
		}
	}

	if v.config.Post != nil {
		for i, payload := range v.config.Post.Payloads {
			if payload.When != nil && payload.When.Expression != "" {
				path := fmt.Sprintf("%s.%s[%d].%s.%s", FieldPost, FieldPayloads, i, FieldLifecycleWhen, FieldExpression)
				v.validateCELExpression(payload.When.Expression, path)
			}
			if payload.Build != nil {
				if buildMap, ok := payload.Build.(map[string]interface{}); ok {
					v.validateBuildExpressions(buildMap, fmt.Sprintf("%s.%s[%d].%s", FieldPost, FieldPayloads, i, FieldBuild))
				}
			}
		}

		for i, action := range v.config.Post.PostActions {
			if action.When != nil && action.When.Expression != "" {
				path := fmt.Sprintf("%s.%s[%d].%s.%s", FieldPost, FieldPostActions, i, FieldLifecycleWhen, FieldExpression)
				v.validateCELExpression(action.When.Expression, path)
			}
		}
	}
}

func (v *TaskConfigValidator) validateCELExpression(expr string, path string) {
	if expr == "" || v.celEnv == nil {
		return
	}

	expr = strings.TrimSpace(expr)

	_, issues := v.celEnv.Parse(expr)
	if issues != nil && issues.Err() != nil {
		v.errors.Add(path, fmt.Sprintf("CEL parse error: %v", issues.Err()))
	}
}

func (v *TaskConfigValidator) validateBuildExpressions(m map[string]interface{}, path string) {
	for key, value := range m {
		currentPath := fmt.Sprintf("%s.%s", path, key)
		switch val := value.(type) {
		case string:
			if key == FieldExpression {
				v.validateCELExpression(val, currentPath)
			}
		case map[string]interface{}:
			v.validateBuildExpressions(val, currentPath)
		case []interface{}:
			for i, item := range val {
				itemPath := fmt.Sprintf("%s[%d]", currentPath, i)
				if m, ok := item.(map[string]interface{}); ok {
					v.validateBuildExpressions(m, itemPath)
				}
			}
		}
	}
}

func (v *TaskConfigValidator) validateK8sManifests() {
	for i, resource := range v.config.Resources {
		if resource.Manifest == nil {
			continue
		}

		path := fmt.Sprintf("%s[%d].%s", FieldResources, i, FieldManifest)

		// All manifests are rendered as Go templates at execution time.
		// K8s structural validation is deferred to execution time since
		// template parameters are not available at config load time.
		// Only validate manifest ref is not empty.
		if m, ok := resource.Manifest.(map[string]interface{}); ok {
			if ref, hasRef := m[FieldRef].(string); hasRef {
				if ref == "" {
					v.errors.Add(path+"."+FieldRef, "manifest ref cannot be empty")
				}
			}
		}
	}
}

func (v *TaskConfigValidator) validateLifecycleConfig() {
	validPropagationPolicies := map[string]bool{
		"Background": true,
		"Foreground": true,
		"Orphan":     true,
	}
	for i, resource := range v.config.Resources {
		if resource.Lifecycle == nil {
			continue
		}

		// NOTE: these are independent if/else-if chains (no continue) so that a validation
		// failure in one lifecycle block (e.g. missing discovery for create) never skips
		// validating the sibling block (e.g. delete) for the same resource.
		if resource.Lifecycle.Create != nil {
			create := resource.Lifecycle.Create
			basePath := fmt.Sprintf("%s[%d].%s.%s", FieldResources, i, FieldLifecycle, FieldLifecycleCreate)

			// discovery is required — without it executeResource cannot determine whether
			// the resource already exists, and will always evaluate the when condition.
			switch {
			case resource.Discovery == nil:
				v.errors.Add(
					basePath,
					"lifecycle.create requires a discovery config to check if the resource already exists",
				)
			case create.When == nil || create.When.Expression == "":
				v.errors.Add(
					basePath+"."+FieldLifecycleWhen+"."+FieldExpression,
					"lifecycle.create.when.expression is required when lifecycle.create is configured",
				)
			default:
				path := basePath + "." + FieldLifecycleWhen + "." + FieldExpression
				v.validateCELExpression(create.When.Expression, path)
			}
		}

		if resource.Lifecycle.Delete != nil {
			del := resource.Lifecycle.Delete
			basePath := fmt.Sprintf("%s[%d].%s.%s", FieldResources, i, FieldLifecycle, FieldLifecycleDelete)

			// discovery is required — without it executeResourceDelete cannot locate
			// the resource and will silently declare it "already deleted" without calling DeleteResource.
			if resource.Discovery == nil {
				v.errors.Add(
					basePath,
					"lifecycle.delete requires a discovery config to locate the resource for deletion",
				)
			} else {
				// Validate propagationPolicy: must be a known K8s value if set
				if del.PropagationPolicy != "" && !validPropagationPolicies[del.PropagationPolicy] {
					v.errors.Add(
						basePath+"."+FieldLifecyclePropagationPolicy,
						fmt.Sprintf("invalid propagationPolicy %q: must be one of Background, Foreground, Orphan",
							del.PropagationPolicy),
					)
				}

				// Validate when: required — without it the resource is never deleted
				if del.When == nil || del.When.Expression == "" {
					v.errors.Add(
						basePath+"."+FieldLifecycleWhen+"."+FieldExpression,
						"lifecycle.delete.when.expression is required: without it the resource will never be deleted",
					)
				} else {
					// Validate when.expression: must be valid CEL
					path := basePath + "." + FieldLifecycleWhen + "." + FieldExpression
					v.validateCELExpression(del.When.Expression, path)
				}
			}
		}
	}
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

func isSliceOrArray(value interface{}) bool {
	if value == nil {
		return false
	}
	kind := reflect.TypeOf(value).Kind()
	return kind == reflect.Slice || kind == reflect.Array
}

// ValidateAdapterVersion validates that the config's adapter version is compatible
// with the expected adapter version. Only major and minor versions are compared;
// patch version differences are allowed (patch releases are bug fixes only).
// For example, config "1.2.0" is compatible with adapter "1.2.3".
func ValidateAdapterVersion(
	ctx context.Context, log logger.Logger, config *AdapterConfig, expectedVersion string,
) error {
	if expectedVersion == "" {
		return nil
	}

	configVersion := config.Adapter.Version
	if configVersion == "" {
		return nil
	}

	configSemver, err := semver.NewVersion(configVersion)
	if err != nil {
		ctx = logger.WithLogField(ctx, "version", configVersion)
		ctx = logger.WithErrorField(ctx, err)
		log.Warn(ctx, "Skipping adapter version validation: config version is not valid semver")
		return nil
	}

	expectedSemver, err := semver.NewVersion(expectedVersion)
	if err != nil {
		ctx = logger.WithLogField(ctx, "version", expectedVersion)
		ctx = logger.WithErrorField(ctx, err)
		log.Warn(ctx, "Skipping adapter version validation: binary version is not valid semver")
		return nil
	}

	// Skip validation for dev builds (0.0.0-*) where major, minor, and patch are all zero
	if expectedSemver.Major() == 0 && expectedSemver.Minor() == 0 && expectedSemver.Patch() == 0 {
		return nil
	}

	if configSemver.Major() != expectedSemver.Major() || configSemver.Minor() != expectedSemver.Minor() {
		return fmt.Errorf("adapter version mismatch: config %q (major.minor=%d.%d) != adapter %q (major.minor=%d.%d)",
			configVersion, configSemver.Major(), configSemver.Minor(),
			expectedVersion, expectedSemver.Major(), expectedSemver.Minor())
	}

	return nil
}
