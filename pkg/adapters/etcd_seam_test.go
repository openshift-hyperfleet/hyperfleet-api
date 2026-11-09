package adapters

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/fleet"
)

func testEtcdSeamInputs() (Membership, EtcdStatusData, *EtcdPKI) {
	membership := NewMembership("my-cluster", []string{"mc-a", "mc-b", "mc-c"}, EtcdOptions{})
	pki, err := MintEtcdPKI("my-cluster", membership.Slots, EtcdPKIOptions{})
	Expect(err).ToNot(HaveOccurred())
	return membership, membership.Status(nil), pki
}

func TestRenderEtcdSeamEmitsMultiEndpointPayload(t *testing.T) {
	RegisterTestingT(t)
	_, status, pki := testEtcdSeamInputs()

	seam, err := RenderEtcdSeam(status, pki, "clusters")

	Expect(err).ToNot(HaveOccurred())
	Expect(seam.Payload.ManagementType).To(Equal(EtcdManagementUnmanaged))
	Expect(seam.Payload.Endpoints).To(Equal(status.Endpoints))
	Expect(seam.Payload.Endpoint).To(BeEmpty())
	Expect(seam.Payload.TLS.ClientSecret.Name).To(Equal(EtcdClientTLSName))
	Expect(seam.Secret.Namespace).To(Equal("clusters"))
	Expect(seam.Secret.Data).To(HaveLen(3))
}

func TestEtcdSeamPayloadNeverCarriesKeyMaterial(t *testing.T) {
	RegisterTestingT(t)
	_, status, pki := testEtcdSeamInputs()

	seam, err := RenderEtcdSeam(status, pki, "clusters")
	Expect(err).ToNot(HaveOccurred())

	payload, err := json.Marshal(seam.Payload)
	Expect(err).ToNot(HaveOccurred())
	Expect(string(payload)).ToNot(ContainSubstring(EtcdClientKeyKey))
	Expect(string(payload)).ToNot(ContainSubstring("PRIVATE KEY"))
}

func TestRenderEtcdSeamFallbackUsesSingleFabricEndpoint(t *testing.T) {
	RegisterTestingT(t)
	_, _, pki := testEtcdSeamInputs()

	seam, err := RenderEtcdSeamFallback("my-cluster", pki, "clusters")

	Expect(err).ToNot(HaveOccurred())
	Expect(seam.Payload.ManagementType).To(Equal(EtcdManagementUnmanaged))
	Expect(seam.Payload.Endpoint).To(Equal("https://etcd-my-cluster:2379"))
	Expect(seam.Payload.Endpoints).To(BeEmpty())
	Expect(seam.Payload.TLS.ClientSecret.Name).To(Equal(EtcdClientTLSName))
}

func TestEtcdSeamIsStableForSameMembership(t *testing.T) {
	RegisterTestingT(t)
	_, status, pki := testEtcdSeamInputs()

	first, err := RenderEtcdSeam(status, pki, "clusters")
	Expect(err).ToNot(HaveOccurred())
	second, err := RenderEtcdSeam(status, pki, "clusters")
	Expect(err).ToNot(HaveOccurred())

	Expect(second.Payload).To(Equal(first.Payload))
	Expect(ValidateEtcdSeamStable(first.Payload, second.Payload)).To(Succeed())
}

func TestEtcdSeamDetectsMembershipSlotChange(t *testing.T) {
	RegisterTestingT(t)
	_, status, pki := testEtcdSeamInputs()
	first, err := RenderEtcdSeam(status, pki, "clusters")
	Expect(err).ToNot(HaveOccurred())

	changed := NewMembership("my-cluster", []string{"mc-a", "mc-b", "mc-c"},
		EtcdOptions{Namespace: "etcd-other"})
	changedStatus := changed.Status(nil)
	Expect(changedStatus.Endpoints).ToNot(Equal(status.Endpoints))
	second, err := RenderEtcdSeam(changedStatus, pki, "clusters")
	Expect(err).ToNot(HaveOccurred())

	Expect(ValidateEtcdSeamStable(first.Payload, second.Payload)).
		To(MatchError(ContainSubstring("endpoints changed")))
}

func TestEtcdSeamPayloadValidationRejectsBadFormat(t *testing.T) {
	RegisterTestingT(t)

	validTLS := EtcdSeamTLS{ClientSecret: EtcdSeamSecretRef{Name: EtcdClientTLSName}}
	tooLong := "https://" + strings.Repeat("a", EtcdSeamMaxEndpointLength)

	tests := map[string]EtcdSeamPayload{
		"wrong management type": {
			ManagementType: "Managed",
			Endpoints:      []string{"https://etcd:2379"},
			TLS:            validTLS,
		},
		"no endpoints": {
			ManagementType: EtcdManagementUnmanaged,
			TLS:            validTLS,
		},
		"wrong scheme": {
			ManagementType: EtcdManagementUnmanaged,
			Endpoints:      []string{"http://etcd:2379"},
			TLS:            validTLS,
		},
		"too long": {
			ManagementType: EtcdManagementUnmanaged,
			Endpoints:      []string{tooLong},
			TLS:            validTLS,
		},
		"missing secret name": {
			ManagementType: EtcdManagementUnmanaged,
			Endpoints:      []string{"https://etcd:2379"},
		},
	}

	for name, payload := range tests {
		Expect(payload.Validate()).To(HaveOccurred(), name)
	}
}

func TestValidateEtcdClientSecretRejectsViolations(t *testing.T) {
	RegisterTestingT(t)
	_, _, pki := testEtcdSeamInputs()
	valid := pki.ClientSecret("clusters")

	missing := valid
	missing.Data = map[string][]byte{
		EtcdClientCAKey:   valid.Data[EtcdClientCAKey],
		EtcdClientCertKey: valid.Data[EtcdClientCertKey],
	}

	extra := valid
	extra.Data = map[string][]byte{
		EtcdClientCAKey:   valid.Data[EtcdClientCAKey],
		EtcdClientCertKey: valid.Data[EtcdClientCertKey],
		EtcdClientKeyKey:  valid.Data[EtcdClientKeyKey],
		"extra":           []byte("x"),
	}

	empty := valid
	empty.Data = map[string][]byte{
		EtcdClientCAKey:   valid.Data[EtcdClientCAKey],
		EtcdClientCertKey: valid.Data[EtcdClientCertKey],
		EtcdClientKeyKey:  nil,
	}

	for name, secret := range map[string]EtcdSecret{
		"missing key": missing,
		"extra key":   extra,
		"empty key":   empty,
	} {
		Expect(ValidateEtcdClientSecret(secret)).To(HaveOccurred(), name)
	}
}

func TestEtcdControllerPublishesSeamInStatus(t *testing.T) {
	RegisterTestingT(t)
	mcs := []fleet.MC{{Name: "mc-a"}, {Name: "mc-b"}, {Name: "mc-c"}}
	transport := &recordingTransport{applied: map[string][]runtime.Object{}}
	controller := NewEtcdController(fakeFleet{mcs: mcs}.list, transport, EtcdOptions{})
	cluster := ClusterView{Name: "my-cluster", Generation: 1, Spec: managedEtcdSpec()}

	report, err := controller.Reconcile(context.Background(), cluster)

	Expect(err).ToNot(HaveOccurred())
	seam, ok := report.Data["seam"].(*EtcdSeamPayload)
	Expect(ok).To(BeTrue())
	Expect(seam.Endpoints).To(HaveLen(3))
	Expect(seam.ManagementType).To(Equal(EtcdManagementUnmanaged))
}
