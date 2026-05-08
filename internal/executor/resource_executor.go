package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/configloader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/criteria"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/maestroclient"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/transportclient"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/metrics"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ResourceExecutor creates and updates Kubernetes resources
type ResourceExecutor struct {
	client  transportclient.TransportClient
	log     logger.Logger
	metrics *metrics.Recorder
}

// newResourceExecutor creates a new resource executor
// NOTE: Caller (NewExecutor) is responsible for config validation
func newResourceExecutor(config *ExecutorConfig) *ResourceExecutor {
	return &ResourceExecutor{
		client:  config.TransportClient,
		log:     config.Logger,
		metrics: config.MetricsRecorder,
	}
}

// ExecuteAll creates/updates all resources in sequence
// Returns results for each resource and updates the execution context
func (re *ResourceExecutor) ExecuteAll(
	ctx context.Context,
	resources []configloader.Resource,
	execCtx *ExecutionContext,
) ([]ResourceResult, error) {
	if execCtx.Resources == nil {
		execCtx.Resources = make(map[string]interface{})
	}

	// Pre-discover all resources before evaluating any lifecycle.delete.when expression.
	// This ensures that when conditions can reference sibling resources regardless of their
	// position in the list. For example, a namespace's when can check
	// !resources.?jobServiceAccount.hasValue() even if jobServiceAccount comes later.
	// NotFound results are non-fatal and leave the resource absent from context.
	// Any other error (RBAC, network, API server) is propagated to avoid incorrect
	// "resource absent" conclusions that could trigger unwanted deletions.
	// Skip the pass entirely when no resource has lifecycle.delete configured — avoids
	// unnecessary discovery API calls for adapters that don't use this feature.
	if hasLifecycleDelete(resources) {
		if err := re.preDiscoverAll(ctx, resources, execCtx); err != nil {
			return nil, err
		}
	}

	results := make([]ResourceResult, 0, len(resources))
	var deleteErrs []error

	for _, resource := range resources {
		result, err := re.executeResource(ctx, resource, execCtx)
		results = append(results, result)

		if err != nil {
			// Delete operations: continue processing remaining resources so that
			// all deletions are attempted even when one fails (JIRA HYPERFLEET-849:
			// "continue with the rest of resources deletion").
			// Apply operations: fail fast (existing behavior).
			if result.Operation == manifest.OperationDelete {
				deleteErrs = append(deleteErrs, err)
				continue
			}
			return results, errors.Join(append(deleteErrs, err)...)
		}
	}

	return results, errors.Join(deleteErrs...)
}

// executeResource creates or updates a single resource via the transport client.
// For k8s transport: renders manifest template → marshals to JSON → calls ApplyResource(bytes)
// For maestro transport: renders manifestWork template → marshals to JSON → calls ApplyResource(bytes)
func (re *ResourceExecutor) executeResource(
	ctx context.Context,
	resource configloader.Resource,
	execCtx *ExecutionContext,
) (ResourceResult, error) {
	result := ResourceResult{
		Name:   resource.Name,
		Status: StatusSuccess,
	}

	transportClient := re.client
	if transportClient == nil {
		result.Status = StatusFailed
		result.Error = fmt.Errorf("transport client not configured for %s", resource.GetTransportClient())
		return result, NewExecutorError(PhaseResources, resource.Name, "transport client not configured", result.Error)
	}

	// Step 1: Build transport context (nil for k8s, *maestroclient.TransportContext for maestro).
	// Done first so it is available for both the lifecycle delete path and the apply path.
	var transportTarget transportclient.TransportContext
	if resource.IsMaestroTransport() && resource.Transport.Maestro != nil {
		targetCluster, tplErr := utils.RenderTemplate(resource.Transport.Maestro.TargetCluster, execCtx.Params)
		if tplErr != nil {
			result.Status = StatusFailed
			result.Error = tplErr
			return result, NewExecutorError(PhaseResources, resource.Name, "failed to render targetCluster template", tplErr)
		}
		transportTarget = &maestroclient.TransportContext{
			ConsumerName: targetCluster,
		}
	}

	// Step 2: Check lifecycle.delete — if the when-expression evaluates to true, delete the resource
	// instead of applying it. This enables dependency-ordered deletion driven by CEL expressions.
	if resource.Lifecycle != nil && resource.Lifecycle.Delete != nil {
		shouldDelete, delErr := re.evaluateLifecycleDeleteWhen(ctx, resource, execCtx)
		if delErr != nil {
			result.Status = StatusFailed
			result.Operation = manifest.OperationDelete
			result.Error = delErr
			re.recordResourceError(execCtx, resource, delErr)
			return result, NewExecutorError(PhaseResources, resource.Name, "failed to evaluate lifecycle.delete.when", delErr)
		}
		if shouldDelete {
			return re.executeResourceDelete(ctx, resource, execCtx, transportTarget)
		}
		// when-expression is false → fall through to normal apply flow
		re.log.Debugf(ctx, "Resource[%s] lifecycle.delete.when evaluated to false, applying normally", resource.Name)
	}

	// Step 3: Render the manifest/manifestWork to bytes
	re.log.Debugf(ctx, "Rendering manifest template for resource %s", resource.Name)
	renderedBytes, err := re.renderToBytes(resource, execCtx)
	if err != nil {
		result.Status = StatusFailed
		result.Error = err
		return result, NewExecutorError(PhaseResources, resource.Name, "failed to render manifest", err)
	}

	// Step 4: Extract resource identity from rendered manifest for result reporting
	var obj unstructured.Unstructured
	if unmarshalErr := json.Unmarshal(renderedBytes, &obj.Object); unmarshalErr == nil {
		result.Kind = obj.GetKind()
		result.Namespace = obj.GetNamespace()
		result.ResourceName = obj.GetName()
	}

	// Step 5: Prepare apply options
	var applyOpts *transportclient.ApplyOptions
	if resource.RecreateOnChange {
		applyOpts = &transportclient.ApplyOptions{RecreateOnChange: true}
	}

	// Step 6: Call transport client ApplyResource with rendered bytes
	applyResult, err := transportClient.ApplyResource(ctx, renderedBytes, applyOpts, transportTarget)
	if err != nil {
		result.Status = StatusFailed
		result.Error = err
		execCtx.Adapter.ExecutionError = &ExecutionError{
			Phase:   string(PhaseResources),
			Step:    resource.Name,
			Message: err.Error(),
		}
		errCtx := logger.WithK8sResult(ctx, "FAILED")
		errCtx = logger.WithErrorField(errCtx, err)
		re.log.Errorf(errCtx, "Resource[%s] processed: FAILED", resource.Name)
		return result, NewExecutorError(PhaseResources, resource.Name, "failed to apply resource", err)
	}

	// Step 7: Extract result
	result.Operation = applyResult.Operation
	result.OperationReason = applyResult.Reason

	successCtx := logger.WithK8sResult(ctx, "SUCCESS")
	re.log.Infof(successCtx, "Resource[%s] processed: operation=%s reason=%s",
		resource.Name, result.Operation, result.OperationReason)

	// Step 7: Post-apply discovery — find the applied resource and store in execCtx for CEL evaluation
	if resource.Discovery != nil {
		discovered, discoverErr := re.discoverResource(ctx, resource, execCtx, transportTarget)
		if discoverErr != nil {
			result.Status = StatusFailed
			result.Error = discoverErr
			execCtx.Adapter.ExecutionError = &ExecutionError{
				Phase:   string(PhaseResources),
				Step:    resource.Name,
				Message: discoverErr.Error(),
			}
			errCtx := logger.WithK8sResult(ctx, "FAILED")
			errCtx = logger.WithErrorField(errCtx, discoverErr)
			re.log.Errorf(errCtx, "Resource[%s] discovery after apply failed: %v", resource.Name, discoverErr)
			return result, NewExecutorError(
				PhaseResources, resource.Name, "failed to discover resource after apply", discoverErr)
		}
		if discovered != nil {
			// Always store the discovered top-level resource by resource name.
			// Nested discoveries are added as independent entries keyed by nested name.
			execCtx.Resources[resource.Name] = discovered
			re.log.Debugf(ctx, "Resource[%s] discovered and stored in context", resource.Name)

			// Step 8: Nested discoveries — find sub-resources within the discovered parent (e.g., ManifestWork)
			if len(resource.NestedDiscoveries) > 0 {
				nestedResults := re.discoverNestedResources(ctx, resource, execCtx, discovered)
				for nestedName, nestedObj := range nestedResults {
					if nestedName == resource.Name {
						re.log.Warnf(ctx,
							"Nested discovery %q has the same name as parent resource; skipping to avoid overwriting parent",
							nestedName)
						continue
					}
					if nestedObj == nil {
						continue
					}
					if _, exists := execCtx.Resources[nestedName]; exists {
						collisionErr := fmt.Errorf(
							"nested discovery key collision: %q already exists in context",
							nestedName,
						)
						result.Status = StatusFailed
						result.Error = collisionErr
						execCtx.Adapter.ExecutionError = &ExecutionError{
							Phase:   string(PhaseResources),
							Step:    resource.Name,
							Message: collisionErr.Error(),
						}
						return result, NewExecutorError(
							PhaseResources, resource.Name,
							"duplicate resource context key",
							collisionErr,
						)
					}
					execCtx.Resources[nestedName] = nestedObj
				}
				re.log.Debugf(ctx, "Resource[%s] discovered with %d nested resources added to context",
					resource.Name, len(nestedResults))
			}
		}
	}

	return result, nil
}

// renderToBytes renders the resource's manifest template to JSON bytes.
// The manifest holds either a K8s resource or a ManifestWork depending on transport type.
// All manifests are rendered as Go templates: map manifests are serialized to YAML first,
// then rendered and parsed like string manifests.
func (re *ResourceExecutor) renderToBytes(
	resource configloader.Resource,
	execCtx *ExecutionContext,
) ([]byte, error) {
	if resource.Manifest == nil {
		return nil, fmt.Errorf("no manifest specified for resource %s", resource.Name)
	}

	manifestStr, err := manifest.ToYAMLString(resource.Manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to convert manifest to string: %w", err)
	}

	return manifest.RenderStringManifest(manifestStr, execCtx.Params)
}

// discoverResource discovers the applied resource using the discovery config.
// For k8s transport: discovers the K8s resource by name or label selector.
// For maestro transport: discovers the ManifestWork by name or label selector.
// The discovered resource is stored in execCtx.Resources for post-action CEL evaluation.
func (re *ResourceExecutor) discoverResource(
	ctx context.Context,
	resource configloader.Resource,
	execCtx *ExecutionContext,
	transportTarget transportclient.TransportContext,
) (*unstructured.Unstructured, error) {
	discovery := resource.Discovery
	if discovery == nil {
		return nil, nil
	}

	// Render discovery namespace template
	namespace, err := utils.RenderTemplate(discovery.Namespace, execCtx.Params)
	if err != nil {
		return nil, fmt.Errorf("failed to render namespace template: %w", err)
	}

	// Discover by name
	if discovery.ByName != "" {
		name, err := utils.RenderTemplate(discovery.ByName, execCtx.Params)
		if err != nil {
			return nil, fmt.Errorf("failed to render byName template: %w", err)
		}

		// For maestro: use ManifestWork GVK
		// For k8s: parse the rendered manifest to get GVK
		gvk := re.resolveGVK(resource)

		return re.client.GetResource(ctx, gvk, namespace, name, transportTarget)
	}

	// Discover by label selector
	if discovery.BySelectors != nil && len(discovery.BySelectors.LabelSelector) > 0 {
		renderedLabels := make(map[string]string)
		for k, v := range discovery.BySelectors.LabelSelector {
			renderedK, err := utils.RenderTemplate(k, execCtx.Params)
			if err != nil {
				return nil, fmt.Errorf("failed to render label key template: %w", err)
			}
			renderedV, err := utils.RenderTemplate(v, execCtx.Params)
			if err != nil {
				return nil, fmt.Errorf("failed to render label value template: %w", err)
			}
			renderedLabels[renderedK] = renderedV
		}

		labelSelector := manifest.BuildLabelSelector(renderedLabels)
		discoveryConfig := &manifest.DiscoveryConfig{
			Namespace:     namespace,
			LabelSelector: labelSelector,
		}

		gvk := re.resolveGVK(resource)

		list, err := re.client.DiscoverResources(ctx, gvk, discoveryConfig, transportTarget)
		if err != nil {
			return nil, err
		}

		if len(list.Items) == 0 {
			return nil, apierrors.NewNotFound(schema.GroupResource{Group: gvk.Group, Resource: gvk.Kind}, "")
		}

		return manifest.GetLatestGenerationFromList(list), nil
	}

	return nil, fmt.Errorf("discovery config must specify byName or bySelectors")
}

// discoverNestedResources discovers sub-resources within a parent resource (e.g., manifests inside a ManifestWork).
// Each nestedDiscovery is matched against the parent's nested manifests using manifest.DiscoverNestedManifest.
func (re *ResourceExecutor) discoverNestedResources(
	ctx context.Context,
	resource configloader.Resource,
	execCtx *ExecutionContext,
	parent *unstructured.Unstructured,
) map[string]*unstructured.Unstructured {
	nestedResults := make(map[string]*unstructured.Unstructured)

	for _, nd := range resource.NestedDiscoveries {
		if nd.Discovery == nil {
			continue
		}

		// Build discovery config with rendered templates
		discoveryConfig, err := re.buildNestedDiscoveryConfig(nd.Discovery, execCtx.Params)
		if err != nil {
			re.log.Warnf(ctx, "Resource[%s] nested discovery[%s] failed to build config: %v",
				resource.Name, nd.Name, err)
			continue
		}

		// Search within the parent resource
		list, err := manifest.DiscoverNestedManifest(parent, discoveryConfig)
		if err != nil {
			re.log.Warnf(ctx, "Resource[%s] nested discovery[%s] failed: %v",
				resource.Name, nd.Name, err)
			continue
		}

		if len(list.Items) == 0 {
			re.log.Debugf(ctx, "Resource[%s] nested discovery[%s] found no matches",
				resource.Name, nd.Name)
			continue
		}

		// Use the latest generation match
		best := manifest.GetLatestGenerationFromList(list)
		if best != nil {
			manifest.EnrichWithResourceStatus(parent, best)
			nestedResults[nd.Name] = best
			re.log.Debugf(ctx, "Resource[%s] nested discovery[%s] found: %s/%s",
				resource.Name, nd.Name, best.GetKind(), best.GetName())
		}
	}

	return nestedResults
}

// buildNestedDiscoveryConfig renders templates in a discovery config and returns a manifest.DiscoveryConfig.
func (re *ResourceExecutor) buildNestedDiscoveryConfig(
	discovery *configloader.DiscoveryConfig,
	params map[string]interface{},
) (*manifest.DiscoveryConfig, error) {
	namespace, err := utils.RenderTemplate(discovery.Namespace, params)
	if err != nil {
		return nil, fmt.Errorf("failed to render namespace template: %w", err)
	}

	if discovery.ByName != "" {
		name, err := utils.RenderTemplate(discovery.ByName, params)
		if err != nil {
			return nil, fmt.Errorf("failed to render byName template: %w", err)
		}
		return &manifest.DiscoveryConfig{
			Namespace: namespace,
			ByName:    name,
		}, nil
	}

	if discovery.BySelectors != nil && len(discovery.BySelectors.LabelSelector) > 0 {
		renderedLabels := make(map[string]string)
		for k, v := range discovery.BySelectors.LabelSelector {
			renderedK, err := utils.RenderTemplate(k, params)
			if err != nil {
				return nil, fmt.Errorf("failed to render label key template: %w", err)
			}
			renderedV, err := utils.RenderTemplate(v, params)
			if err != nil {
				return nil, fmt.Errorf("failed to render label value template: %w", err)
			}
			renderedLabels[renderedK] = renderedV
		}
		return &manifest.DiscoveryConfig{
			Namespace:     namespace,
			LabelSelector: manifest.BuildLabelSelector(renderedLabels),
		}, nil
	}

	return nil, fmt.Errorf("discovery must specify byName or bySelectors")
}

// resolveGVK extracts the GVK from the resource's manifest.
// Works for both K8s resources and ManifestWorks since both have apiVersion and kind.
func (re *ResourceExecutor) resolveGVK(resource configloader.Resource) schema.GroupVersionKind {
	var manifestData map[string]interface{}

	switch m := resource.Manifest.(type) {
	case map[string]interface{}:
		manifestData = m
	case string:
		// String manifests may contain Go template directives ({{ if }}, {{ range }})
		// that make them invalid YAML. Extract apiVersion and kind by scanning lines
		// instead of full YAML parsing.
		return manifest.ExtractGVKFromString(m)
	default:
		return schema.GroupVersionKind{}
	}

	apiVersion, ok1 := manifestData["apiVersion"].(string)
	kind, ok2 := manifestData["kind"].(string)
	if !ok1 || !ok2 {
		return schema.GroupVersionKind{}
	}
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return schema.GroupVersionKind{}
	}
	return gv.WithKind(kind)
}

// hasLifecycleDelete reports whether any resource in the list has lifecycle.delete configured.
func hasLifecycleDelete(resources []configloader.Resource) bool {
	for _, r := range resources {
		if r.Lifecycle != nil && r.Lifecycle.Delete != nil {
			return true
		}
	}
	return false
}

// preDiscoverAll discovers all resources and populates execCtx.Resources before the main
// resource loop begins. This makes every resource's current cluster state available to
// lifecycle.delete.when CEL expressions regardless of list order.
//
// NotFound is non-fatal: the resource stays absent from context (resources.?X.hasValue()
// returns false), which is correct for "not yet created" or "already deleted".
//
// Any other error (RBAC denial, network failure, API server error) is returned immediately.
// These transient errors must not be silently treated as "resource absent": doing so could
// cause a lifecycle.delete.when expression to evaluate to true and trigger an unwanted
// deletion, or cause a Finalized=True status to be reported prematurely.
//
// Note: this pre-pass result may be overwritten during the main loop as resources are
// applied/deleted and their post-operation state is re-discovered. The pre-pass only
// guarantees that the initial cluster state is visible to when expressions.
func (re *ResourceExecutor) preDiscoverAll(
	ctx context.Context,
	resources []configloader.Resource,
	execCtx *ExecutionContext,
) error {
	for _, resource := range resources {
		if resource.Discovery == nil {
			continue
		}

		var transportTarget transportclient.TransportContext
		if resource.IsMaestroTransport() && resource.Transport.Maestro != nil {
			targetCluster, err := utils.RenderTemplate(resource.Transport.Maestro.TargetCluster, execCtx.Params)
			if err != nil {
				re.log.Warnf(ctx, "Resource[%s] pre-discovery: failed to render targetCluster: %v",
					resource.Name, err)
				return NewExecutorError(PhaseResources, resource.Name, "failed to render targetCluster", err)
			}
			transportTarget = &maestroclient.TransportContext{ConsumerName: targetCluster}
		}

		discovered, err := re.discoverResource(ctx, resource, execCtx, transportTarget)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// Resource does not exist yet — leave absent from context.
				continue
			}
			// Transient error (RBAC, network, API server): propagate so the reconciliation
			// fails visibly rather than treating the resource as absent.
			re.log.Warnf(ctx, "Resource[%s] pre-discovery failed: %v", resource.Name, err)
			return NewExecutorError(PhaseResources, resource.Name, "pre-discovery failed", err)
		}
		if discovered != nil {
			execCtx.Resources[resource.Name] = discovered
			re.log.Debugf(ctx, "Resource[%s] pre-discovered and stored in context", resource.Name)
		}
	}
	return nil
}

// evaluateLifecycleDeleteWhen evaluates the lifecycle.delete.when CEL expression
// and returns true if the resource should be deleted.
//
// The evaluation uses the same CEL context as post-actions: params + adapter metadata + resources.
// If when.expression is not set, returns false (no deletion).
func (re *ResourceExecutor) evaluateLifecycleDeleteWhen(
	ctx context.Context,
	resource configloader.Resource,
	execCtx *ExecutionContext,
) (bool, error) {
	if resource.Lifecycle.Delete.When == nil {
		return false, nil
	}
	if resource.Lifecycle.Delete.When.Expression == "" {
		return false, fmt.Errorf("resource %q has lifecycle.delete.when configured but expression is empty", resource.Name)
	}
	expression := resource.Lifecycle.Delete.When.Expression

	evalCtx := criteria.NewEvaluationContext()
	evalCtx.SetVariablesFromMap(execCtx.GetCELVariables())

	evaluator, err := criteria.NewEvaluator(ctx, evalCtx, re.log)
	if err != nil {
		return false, fmt.Errorf("failed to create CEL evaluator: %w", err)
	}

	celResult, err := evaluator.EvaluateCEL(expression)
	if err != nil {
		// Surface the error so the executor marks executionStatus=failed and the operator
		// sees Health=False. A common cause is a variable used in the expression that was
		// never captured in the precondition phase (e.g. a typo in a capture name).
		// Failing loudly is safer than silently skipping deletion: the cluster would stay
		// stuck in Finalizing with no visible signal.
		return false, fmt.Errorf("lifecycle.delete.when expression %q failed to evaluate "+
			"(check that all variables are captured in preconditions): %w", expression, err)
	}

	execCtx.AddCELEvaluation(PhaseResources, resource.Name+"/lifecycle.delete.when", expression, celResult.Matched)
	re.log.Debugf(ctx, "Resource[%s] lifecycle.delete.when=%q → matched=%v", resource.Name, expression, celResult.Matched)

	return celResult.Matched, nil
}

// executeResourceDelete handles the delete path for a resource with lifecycle.delete configured.
//
// Delete ordering is driven by a post-delete rediscovery after the delete call:
//   - If the resource is not found (pre-delete): store nil in context (already deleted), no-op.
//   - If the resource is found: send delete request, then rediscover to check actual state:
//   - Post-delete NotFound: resource is truly gone (no finalizers, instant K8s delete).
//     Store nil — dependent resources can cascade in the same reconciliation.
//   - Post-delete still present: finalizers are running, or Maestro deletion is async.
//     Store the object — dependent resources wait for the next reconciliation.
//
// This allows same-reconciliation cascading for K8s resources without finalizers, while
// correctly deferring to the next reconciliation for resources with finalizers or async
// Maestro deletions.
func (re *ResourceExecutor) executeResourceDelete(
	ctx context.Context,
	resource configloader.Resource,
	execCtx *ExecutionContext,
	transportTarget transportclient.TransportContext,
) (ResourceResult, error) {
	// Extract resource type (Kubernetes kind) from manifest for metrics labeling.
	// This is done early so it's available for all metric recording paths (success/failure).
	gvk := re.resolveGVK(resource)
	resourceType := gvk.Kind
	if resourceType == "" {
		resourceType = metrics.ResourceTypeUnknown
	}

	// Metrics: track deletion in-progress and duration
	startTime := time.Now()
	re.metrics.IncDeletionInProgress(resourceType)
	defer re.metrics.DecDeletionInProgress(resourceType)

	result := ResourceResult{
		Name:      resource.Name,
		Status:    StatusSuccess,
		Operation: manifest.OperationDelete,
	}

	// Step 1: Discover the existing resource
	discovered, discoverErr := re.discoverResource(ctx, resource, execCtx, transportTarget)

	isNotFound := discoverErr != nil && apierrors.IsNotFound(discoverErr)
	if discoverErr != nil && !isNotFound {
		result.Status = StatusFailed
		result.Error = discoverErr
		re.recordResourceError(execCtx, resource, discoverErr)
		re.metrics.RecordDeletion(resourceType, metrics.DeletionStatusError)
		re.metrics.ObserveDeletionDuration(resourceType, time.Since(startTime))
		return result, NewExecutorError(
			PhaseResources, resource.Name, "failed to discover resource for deletion", discoverErr)
	}

	// Step 2: If not found — resource is already deleted (or never existed)
	if discovered == nil || isNotFound {
		// Store nil — the key is removed from the CEL resources map, so
		// !resources.?X.hasValue() evaluates to true in this reconciliation.
		execCtx.Resources[resource.Name] = nil
		result.OperationReason = "resource already deleted or never existed"
		re.log.Infof(ctx, "Resource[%s] delete: already deleted or never existed", resource.Name)
		re.metrics.RecordDeletion(resourceType, metrics.DeletionStatusSuccess)
		re.metrics.ObserveDeletionDuration(resourceType, time.Since(startTime))
		return result, nil
	}

	// Step 3: Extract identity from discovered resource for result reporting
	gvk = discovered.GroupVersionKind()
	result.Kind = gvk.Kind
	result.Namespace = discovered.GetNamespace()
	result.ResourceName = discovered.GetName()
	result.DiscoveredState = discovered

	// Update resourceType with authoritative kind from discovered resource.
	// If manifest-based GVK resolution initially failed (resourceType was set to "Unknown"),
	// we now have the correct kind from the actual K8s resource.
	if gvk.Kind != "" {
		resourceType = gvk.Kind
	}

	// Step 4: Build delete options
	propagationPolicy := "Background"
	if resource.Lifecycle.Delete.PropagationPolicy != "" {
		propagationPolicy = resource.Lifecycle.Delete.PropagationPolicy
	}
	deleteOpts := &transportclient.DeleteOptions{PropagationPolicy: propagationPolicy}

	// Step 5: Delete via transport client
	if err := re.client.DeleteResource(
		ctx, gvk, result.Namespace, result.ResourceName, deleteOpts, transportTarget,
	); err != nil {
		result.Status = StatusFailed
		result.Error = err
		re.recordResourceError(execCtx, resource, err)
		errCtx := logger.WithK8sResult(ctx, "FAILED")
		errCtx = logger.WithErrorField(errCtx, err)
		re.log.Errorf(errCtx, "Resource[%s] delete: FAILED", resource.Name)
		re.metrics.RecordDeletion(resourceType, metrics.DeletionStatusError)
		re.metrics.ObserveDeletionDuration(resourceType, time.Since(startTime))
		return result, NewExecutorError(PhaseResources, resource.Name, "failed to delete resource", err)
	}

	// Step 6: Re-discover the resource after deletion to determine its actual state.
	// - If NotFound: resource is truly gone (no finalizers, or K8s Background delete was instant).
	//   Store nil so dependent resources can cascade in the same reconciliation.
	// - If still present (e.g., deletionTimestamp set, finalizers running, or Maestro async):
	//   Store the object so dependent resources wait for the next reconciliation.
	postDeleteDiscovered, postDiscoverErr := re.discoverResource(ctx, resource, execCtx, transportTarget)
	postIsNotFound := postDiscoverErr != nil && apierrors.IsNotFound(postDiscoverErr)
	switch {
	case postDiscoverErr != nil && !postIsNotFound:
		// Non-fatal: log the error but don't fail the delete — the delete itself succeeded.
		re.log.Debugf(ctx, "Resource[%s] post-delete discovery error (non-fatal): %v", resource.Name, postDiscoverErr)
		execCtx.Resources[resource.Name] = discovered
	case postDeleteDiscovered == nil || postIsNotFound:
		// Resource is confirmed gone: dependent resources can proceed in this reconciliation.
		execCtx.Resources[resource.Name] = nil
		re.log.Debugf(ctx, "Resource[%s] confirmed deleted (post-delete discovery: not found)", resource.Name)
	default:
		// Resource still present (finalizers or async deletion): dependents wait for next reconciliation.
		execCtx.Resources[resource.Name] = postDeleteDiscovered
		re.log.Debugf(ctx, "Resource[%s] still present after delete (finalizers or async): dependents wait", resource.Name)
	}

	result.OperationReason = "lifecycle.delete.when evaluated to true"

	re.log.Infof(logger.WithK8sResult(ctx, "SUCCESS"),
		"Resource[%s] delete: operation=delete propagationPolicy=%s",
		resource.Name, propagationPolicy)

	re.metrics.RecordDeletion(resourceType, metrics.DeletionStatusSuccess)
	re.metrics.ObserveDeletionDuration(resourceType, time.Since(startTime))

	return result, nil
}

// recordResourceError sets execCtx.Adapter.ExecutionError (first error wins) and populates
// execCtx.Adapter.ResourceErrors with a per-resource entry. Called by executeResourceDelete
// on both discovery failure and delete failure paths.
func (re *ResourceExecutor) recordResourceError(execCtx *ExecutionContext, resource configloader.Resource, err error) {
	execErr := ExecutionError{
		Phase:   string(PhaseResources),
		Step:    resource.Name,
		Message: err.Error(),
	}
	if execCtx.Adapter.ExecutionError == nil {
		execCtx.Adapter.ExecutionError = &execErr
	}
	if execCtx.Adapter.ResourceErrors == nil {
		execCtx.Adapter.ResourceErrors = make(map[string]ExecutionError)
	}
	execCtx.Adapter.ResourceErrors[resource.Name] = execErr
}

// GetResourceAsMap converts an unstructured resource to a map for CEL evaluation
func GetResourceAsMap(resource *unstructured.Unstructured) map[string]interface{} {
	if resource == nil {
		return nil
	}
	return resource.Object
}
