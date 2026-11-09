package adapters

import (
	"fmt"
	"slices"
	"strings"
)

const (
	// EtcdManagementUnmanaged is the only management type the PoC emits.
	EtcdManagementUnmanaged = "Unmanaged"
	// EtcdSeamMaxEndpointLength is the verified UnmanagedEtcdSpec limit.
	EtcdSeamMaxEndpointLength = 255
)

// EtcdSeamPayload is the etcd block of a HostedCluster spec. The primary shape
// carries every member endpoint; the fallback carries one fabric-DNS endpoint.
//
//nolint:govet // fieldalignment: JSON contract reads naturally in this order
type EtcdSeamPayload struct {
	ManagementType string      `json:"managementType"`
	Endpoints      []string    `json:"endpoints,omitempty"`
	Endpoint       string      `json:"endpoint,omitempty"`
	TLS            EtcdSeamTLS `json:"tls"`
}

// EtcdSeamTLS is the TLS block of the etcd seam.
type EtcdSeamTLS struct {
	ClientSecret EtcdSeamSecretRef `json:"clientSecret"`
}

// EtcdSeamSecretRef names the Secret in the HostedCluster namespace.
type EtcdSeamSecretRef struct {
	Name string `json:"name"`
}

// EtcdSeam is one rendered handoff. Payload is safe for status; Secret carries
// key material and goes straight to the transport.
//
//nolint:govet // fieldalignment: payload-first reads better than padding
type EtcdSeam struct {
	Payload EtcdSeamPayload
	Secret  EtcdSecret
}

// RenderEtcdSeam renders the primary multi-endpoint shape from published
// status data: every member slot URL the adapter already reports.
func RenderEtcdSeam(
	status EtcdStatusData, pki *EtcdPKI, hcNamespace string,
) (EtcdSeam, error) {
	seam, err := newEtcdSeam(pki, hcNamespace)
	if err != nil {
		return EtcdSeam{}, err
	}
	seam.Payload.Endpoints = append([]string(nil), status.Endpoints...)
	return seam, seam.Validate()
}

// RenderEtcdSeamFallback renders the single-endpoint shape used if upstream
// declines the multi-endpoint ask. The endpoint is a slot-stable fabric DNS
// name; the adapter re-points it when a member slot is replaced. Fabric DNS
// manipulation is later work, so this only renders the name.
func RenderEtcdSeamFallback(
	cluster string, pki *EtcdPKI, hcNamespace string,
) (EtcdSeam, error) {
	seam, err := newEtcdSeam(pki, hcNamespace)
	if err != nil {
		return EtcdSeam{}, err
	}
	seam.Payload.Endpoint = EtcdFabricEndpoint(cluster)
	return seam, seam.Validate()
}

// EtcdFabricEndpoint is the fallback's slot-stable fabric DNS endpoint.
func EtcdFabricEndpoint(cluster string) string {
	return fmt.Sprintf("https://etcd-%s:%d", cluster, etcdClientPort)
}

func newEtcdSeam(pki *EtcdPKI, hcNamespace string) (EtcdSeam, error) {
	if pki == nil {
		return EtcdSeam{}, fmt.Errorf("etcd PKI is required")
	}
	secret := pki.ClientSecret(hcNamespace)
	return EtcdSeam{
		Payload: EtcdSeamPayload{
			ManagementType: EtcdManagementUnmanaged,
			TLS: EtcdSeamTLS{
				ClientSecret: EtcdSeamSecretRef{Name: secret.Name},
			},
		},
		Secret: secret,
	}, nil
}

// Validate checks the payload format and the three-key Secret manifest.
func (s EtcdSeam) Validate() error {
	if err := s.Payload.Validate(); err != nil {
		return err
	}
	if s.Secret.Name == "" || s.Secret.Namespace == "" {
		return fmt.Errorf("etcd client secret name and namespace are required")
	}
	return ValidateEtcdClientSecret(s.Secret)
}

// Validate mirrors the verified UnmanagedEtcdSpec CEL rules.
func (p EtcdSeamPayload) Validate() error {
	if p.ManagementType != EtcdManagementUnmanaged {
		return fmt.Errorf(
			"etcd seam managementType must be %q, got %q",
			EtcdManagementUnmanaged, p.ManagementType,
		)
	}
	endpoints := p.endpointList()
	if len(endpoints) == 0 {
		return fmt.Errorf("etcd seam has no endpoints")
	}
	for _, endpoint := range endpoints {
		if !strings.HasPrefix(endpoint, "https://") {
			return fmt.Errorf("etcd seam endpoint %q must start with https://", endpoint)
		}
		if len(endpoint) > EtcdSeamMaxEndpointLength {
			return fmt.Errorf(
				"etcd seam endpoint %q exceeds %d characters",
				endpoint, EtcdSeamMaxEndpointLength,
			)
		}
	}
	if p.TLS.ClientSecret.Name == "" {
		return fmt.Errorf("etcd seam client secret name is required")
	}
	return nil
}

func (p EtcdSeamPayload) endpointList() []string {
	if len(p.Endpoints) > 0 {
		return p.Endpoints
	}
	if p.Endpoint != "" {
		return []string{p.Endpoint}
	}
	return nil
}

// ValidateEtcdSeamStable detects a membership change between the published
// and re-rendered seam. The endpoint list is immutable after creation, so a
// mismatch is a contract violation; the replacement flow owns resolution.
func ValidateEtcdSeamStable(previous, current EtcdSeamPayload) error {
	if !slices.Equal(previous.endpointList(), current.endpointList()) {
		return fmt.Errorf(
			"etcd seam endpoints changed: %v -> %v",
			previous.endpointList(), current.endpointList(),
		)
	}
	return nil
}

// ValidateEtcdClientSecret enforces exactly the three verified seam keys.
func ValidateEtcdClientSecret(secret EtcdSecret) error {
	if len(secret.Data) != 3 {
		return fmt.Errorf(
			"etcd client secret must have exactly 3 keys, got %d", len(secret.Data),
		)
	}
	for _, key := range []string{EtcdClientCAKey, EtcdClientCertKey, EtcdClientKeyKey} {
		if len(secret.Data[key]) == 0 {
			return fmt.Errorf("etcd client secret is missing non-empty key %q", key)
		}
	}
	return nil
}
