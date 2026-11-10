package adapters

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestEtcdStateRoundTripsMembershipAndPKI(t *testing.T) {
	RegisterTestingT(t)
	dir := t.TempDir()
	membership := NewMembership("my-cluster", []string{"mc-a", "mc-b", "mc-c"}, EtcdOptions{
		NodeIPs: map[string]string{"mc-a": "10.89.0.10", "mc-b": "10.89.0.11", "mc-c": "10.89.0.12"},
	})
	pki, err := MintEtcdPKI("my-cluster", membership.Slots, EtcdPKIOptions{})
	Expect(err).ToNot(HaveOccurred())

	Expect(saveEtcdState(dir, stateFrom(membership, pki))).To(Succeed())
	loaded, err := loadEtcdState(dir, "my-cluster")
	Expect(err).ToNot(HaveOccurred())
	Expect(loaded.Membership.Slots[1].ClientURL).To(Equal("https://10.89.0.11:31379"))
	Expect(loaded.Membership.Slots[1].TargetMC).To(Equal("mc-b"))
	Expect(loaded.pki().CA.CertPEM).To(Equal(pki.CA.CertPEM))
	Expect(loaded.pki().Client.KeyPEM).To(Equal(pki.Client.KeyPEM))
}

func TestLoadEtcdStateIgnoresAMissingDirectory(t *testing.T) {
	RegisterTestingT(t)

	loaded, err := loadEtcdState(t.TempDir(), "missing")

	Expect(err).ToNot(HaveOccurred())
	Expect(loaded).To(BeNil())
}
