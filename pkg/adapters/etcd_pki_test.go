package adapters

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func parseCert(t *testing.T, material CertMaterial) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(material.CertPEM)
	Expect(block).ToNot(BeNil())
	cert, err := x509.ParseCertificate(block.Bytes)
	Expect(err).ToNot(HaveOccurred())
	return cert
}

func TestMintEtcdPKICreatesVerifiableMemberMaterial(t *testing.T) {
	RegisterTestingT(t)
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	membership := NewMembership("my-cluster", []string{"mc-a", "mc-b", "mc-c"}, EtcdOptions{})

	pki, err := MintEtcdPKI("my-cluster", membership.Slots, EtcdPKIOptions{
		Now: func() time.Time { return now },
	})
	Expect(err).ToNot(HaveOccurred())

	ca := parseCert(t, pki.CA)
	Expect(ca.IsCA).To(BeTrue())
	Expect(ca.KeyUsage & x509.KeyUsageCertSign).ToNot(BeZero())

	for _, slot := range membership.Slots {
		server := parseCert(t, pki.ServerCerts[slot.MemberName])
		Expect(server.CheckSignatureFrom(ca)).To(Succeed())
		Expect(server.DNSNames).To(ContainElement(slot.MemberName))
		Expect(server.ExtKeyUsage).To(ContainElement(x509.ExtKeyUsageServerAuth))

		peer := parseCert(t, pki.PeerCerts[slot.MemberName])
		Expect(peer.CheckSignatureFrom(ca)).To(Succeed())
		Expect(peer.ExtKeyUsage).To(ContainElement(x509.ExtKeyUsageClientAuth))

		for _, material := range []CertMaterial{
			pki.ServerCerts[slot.MemberName],
			pki.PeerCerts[slot.MemberName],
		} {
			_, keyErr := tls.X509KeyPair(material.CertPEM, material.KeyPEM)
			Expect(keyErr).ToNot(HaveOccurred())
		}
	}

	client := parseCert(t, pki.Client)
	Expect(client.CheckSignatureFrom(ca)).To(Succeed())
	Expect(client.ExtKeyUsage).To(ConsistOf(x509.ExtKeyUsageClientAuth))
	Expect(client.NotBefore).To(Equal(now.Add(-5 * time.Minute)))
	Expect(client.NotAfter).To(Equal(now.Add(defaultCertValidity)))
}

func TestEtcdClientSecretHasSeamKeys(t *testing.T) {
	RegisterTestingT(t)
	membership := NewMembership("my-cluster", []string{"mc-a", "mc-b", "mc-c"}, EtcdOptions{})
	pki, err := MintEtcdPKI("my-cluster", membership.Slots, EtcdPKIOptions{})
	Expect(err).ToNot(HaveOccurred())

	secret := pki.ClientSecret("clusters")

	Expect(secret.Name).To(Equal(EtcdClientTLSName))
	Expect(secret.Namespace).To(Equal("clusters"))
	Expect(secret.Data).To(HaveLen(3))
	Expect(secret.Data).To(HaveKey(EtcdClientCAKey))
	Expect(secret.Data).To(HaveKey(EtcdClientCertKey))
	Expect(secret.Data).To(HaveKey(EtcdClientKeyKey))
	Expect(secret.Data[EtcdClientKeyKey]).ToNot(BeEmpty())
}

func TestEtcdMemberSecretCarriesMemberCerts(t *testing.T) {
	RegisterTestingT(t)
	membership := NewMembership("my-cluster", []string{"mc-a", "mc-b", "mc-c"}, EtcdOptions{})
	pki, err := MintEtcdPKI("my-cluster", membership.Slots, EtcdPKIOptions{})
	Expect(err).ToNot(HaveOccurred())

	secret := pki.MemberSecret(membership.Slots[0])

	Expect(secret.Name).To(Equal("my-cluster-etcd-m0-tls"))
	Expect(secret.Namespace).To(Equal(membership.Namespace))
	Expect(secret.Data).To(HaveLen(7))
	for _, key := range []string{
		EtcdCACertKey, EtcdServerCertKey, EtcdServerKeyKey, EtcdPeerCertKey, EtcdPeerKeyKey,
		EtcdClientCertKey, EtcdClientKeyKey,
	} {
		Expect(secret.Data[key]).ToNot(BeEmpty(), key)
	}
}

func TestMintEtcdPKIIncludesFabricNodeAddresses(t *testing.T) {
	RegisterTestingT(t)
	membership := NewMembership("my-cluster", []string{"mc-a", "mc-b", "mc-c"}, EtcdOptions{
		NodeIPs: map[string]string{"mc-a": "10.89.0.10", "mc-b": "10.89.0.11", "mc-c": "10.89.0.12"},
	})

	pki, err := MintEtcdPKI("my-cluster", membership.Slots, EtcdPKIOptions{})
	Expect(err).ToNot(HaveOccurred())

	server := parseCert(t, pki.ServerCerts[membership.Slots[1].MemberName])
	Expect(server.IPAddresses[1].String()).To(Equal("10.89.0.11"))
	peer := parseCert(t, pki.PeerCerts[membership.Slots[1].MemberName])
	Expect(peer.IPAddresses[1].String()).To(Equal("10.89.0.11"))
}

func TestRemintMemberKeepsTheCAAndAddsTheNewAddress(t *testing.T) {
	RegisterTestingT(t)
	membership := NewMembership("my-cluster", []string{"mc-a", "mc-b", "mc-c"}, EtcdOptions{})
	pki, err := MintEtcdPKI("my-cluster", membership.Slots, EtcdPKIOptions{})
	Expect(err).ToNot(HaveOccurred())

	replacement := membership.Slots[0]
	applyNodeAddress(&replacement, "10.89.0.30")
	Expect(pki.RemintMember(replacement)).To(Succeed())

	ca := parseCert(t, pki.CA)
	server := parseCert(t, pki.ServerCerts[replacement.MemberName])
	Expect(server.CheckSignatureFrom(ca)).To(Succeed())
	Expect(server.IPAddresses[1].String()).To(Equal("10.89.0.30"))
}
