// Package maestroclient tests
//
// Note: Tests for manifest.ValidateGeneration, manifest.ValidateGenerationFromUnstructured,
// and manifest.ValidateManifestWorkGeneration are in internal/generation/generation_test.go.
// This file contains tests specific to maestroclient functionality.
package maestroclient

import (
	"context"
	"fmt"
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/constants"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	workv1 "open-cluster-management.io/api/work/v1"
)

func TestIsConsumerNotFoundError(t *testing.T) {
	tests := []struct {
		err      error
		name     string
		expected bool
	}{
		{
			name:     "nil error returns false",
			err:      nil,
			expected: false,
		},
		{
			name:     "unrelated error returns false",
			err:      fmt.Errorf("some other database error"),
			expected: false,
		},
		{
			name: "FK constraint error is detected",
			err: fmt.Errorf(
				`pq: insert or update on table "resources" ` +
					`violates foreign key constraint "fk_resources_consumers"`),
			expected: true,
		},
		{
			name:     "FK constraint wrapped in outer message is detected",
			err:      fmt.Errorf(`maestro error: %w`, fmt.Errorf(`fk_resources_consumers violation`)),
			expected: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isConsumerNotFoundError(tt.err); got != tt.expected {
				t.Errorf("isConsumerNotFoundError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetGenerationFromManifestWork(t *testing.T) {
	tests := []struct {
		work     *workv1.ManifestWork
		name     string
		expected int64
	}{
		{
			name:     "nil work returns 0",
			work:     nil,
			expected: 0,
		},
		{
			name: "work with generation annotation",
			work: &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.AnnotationGeneration: "42",
					},
				},
			},
			expected: 42,
		},
		{
			name: "work without annotations",
			work: &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{},
			},
			expected: 0,
		},
		{
			name: "work with invalid generation value",
			work: &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.AnnotationGeneration: "invalid",
					},
				},
			},
			expected: 0,
		},
		{
			name: "work with empty generation value",
			work: &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.AnnotationGeneration: "",
					},
				},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result int64
			if tt.work == nil {
				result = 0
			} else {
				result = manifest.GetGeneration(tt.work.ObjectMeta)
			}
			if result != tt.expected {
				t.Errorf("expected generation %d, got %d", tt.expected, result)
			}
		})
	}
}

// BuildManifestWorkName generates a consistent ManifestWork name for testing
// Format: <adapter-name>-<resource-name>-<cluster-id>
func BuildManifestWorkName(adapterName, resourceName, clusterID string) string {
	return adapterName + "-" + resourceName + "-" + clusterID
}

func TestBuildManifestWorkName(t *testing.T) {
	tests := []struct {
		name         string
		adapterName  string
		resourceName string
		clusterID    string
		expected     string
	}{
		{
			name:         "basic name construction",
			adapterName:  "my-adapter",
			resourceName: "namespace",
			clusterID:    "cluster-123",
			expected:     "my-adapter-namespace-cluster-123",
		},
		{
			name:         "with special characters",
			adapterName:  "adapter_v1",
			resourceName: "config-map",
			clusterID:    "prod-us-east-1",
			expected:     "adapter_v1-config-map-prod-us-east-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildManifestWorkName(tt.adapterName, tt.resourceName, tt.clusterID)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestGenerationComparison(t *testing.T) {
	tests := []struct {
		name               string
		description        string
		existingGeneration int64
		newGeneration      int64
		shouldUpdate       bool
	}{
		{
			name:               "same generation - no update",
			existingGeneration: 5,
			newGeneration:      5,
			shouldUpdate:       false,
			description:        "When generations match, should skip update",
		},
		{
			name:               "newer generation - update",
			existingGeneration: 5,
			newGeneration:      6,
			shouldUpdate:       true,
			description:        "When new generation is higher, should update",
		},
		{
			name:               "older generation - still update",
			existingGeneration: 6,
			newGeneration:      5,
			shouldUpdate:       true,
			description:        "When new generation is lower, should still update (allow rollback)",
		},
		// Note: "both 0" case is no longer valid since validation requires generation > 0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Logic from ApplyManifestWork:
			// if existingGeneration == generation { return existing }
			shouldSkipUpdate := tt.existingGeneration == tt.newGeneration
			shouldUpdate := !shouldSkipUpdate

			if shouldUpdate != tt.shouldUpdate {
				t.Errorf("%s: expected shouldUpdate=%v, got %v",
					tt.description, tt.shouldUpdate, shouldUpdate)
			}
		})
	}
}

func TestIsTransientGRPCError(t *testing.T) {
	tests := []struct {
		err      error
		name     string
		expected bool
	}{
		{
			name:     "nil returns false",
			err:      nil,
			expected: false,
		},
		{
			name:     "Unavailable returns true",
			err:      status.Error(codes.Unavailable, "connection refused"),
			expected: true,
		},
		{
			name:     "InvalidArgument returns false",
			err:      status.Error(codes.InvalidArgument, "bad request"),
			expected: false,
		},
		{
			name:     "NotFound returns false",
			err:      status.Error(codes.NotFound, "not found"),
			expected: false,
		},
		{
			name:     "PermissionDenied returns false",
			err:      status.Error(codes.PermissionDenied, "denied"),
			expected: false,
		},
		{
			name:     "non-gRPC error returns false",
			err:      fmt.Errorf("plain error"),
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransientGRPCError(tt.err); got != tt.expected {
				t.Errorf("isTransientGRPCError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRetryOnTransientGRPC(t *testing.T) {
	client := &Client{log: logger.NewTestLogger()}
	ctx := context.Background()

	t.Run("succeeds first try", func(t *testing.T) {
		calls := 0
		err := client.retryOnTransientGRPC(ctx, func() error {
			calls++
			return nil
		})
		if err != nil {
			t.Errorf("expected nil, got %v", err)
		}
		if calls != 1 {
			t.Errorf("expected 1 call, got %d", calls)
		}
	})

	t.Run("succeeds on second try", func(t *testing.T) {
		calls := 0
		err := client.retryOnTransientGRPC(ctx, func() error {
			calls++
			if calls == 1 {
				return status.Error(codes.Unavailable, "transient")
			}
			return nil
		})
		if err != nil {
			t.Errorf("expected nil, got %v", err)
		}
		if calls != 2 {
			t.Errorf("expected 2 calls, got %d", calls)
		}
	})

	t.Run("gives up after max attempts", func(t *testing.T) {
		calls := 0
		err := client.retryOnTransientGRPC(ctx, func() error {
			calls++
			return status.Error(codes.Unavailable, "always failing")
		})
		if err == nil {
			t.Error("expected error, got nil")
		}
		if calls != grpcRetryMaxAttempts {
			t.Errorf("expected %d calls, got %d", grpcRetryMaxAttempts, calls)
		}
	})

	t.Run("no retry on non-transient error", func(t *testing.T) {
		calls := 0
		err := client.retryOnTransientGRPC(ctx, func() error {
			calls++
			return status.Error(codes.InvalidArgument, "bad request")
		})
		if err == nil {
			t.Error("expected error, got nil")
		}
		if calls != 1 {
			t.Errorf("expected 1 call, got %d", calls)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()
		calls := 0
		err := client.retryOnTransientGRPC(cancelCtx, func() error {
			calls++
			return status.Error(codes.Unavailable, "transient")
		})
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})
}
