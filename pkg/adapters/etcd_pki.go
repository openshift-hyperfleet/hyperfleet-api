package adapters

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"
)

const (
	EtcdCACertKey     = "ca.crt"
	EtcdServerCertKey = "server.crt"
	EtcdServerKeyKey  = "server.key"
	EtcdPeerCertKey   = "peer.crt"
	EtcdPeerKeyKey    = "peer.key"

	EtcdClientCAKey   = "etcd-client-ca.crt"
	EtcdClientCertKey = "etcd-client.crt"
	EtcdClientKeyKey  = "etcd-client.key"

	EtcdClientTLSName = "etcd-client-tls"
)

const (
	defaultCertValidity = 365 * 24 * time.Hour
	caValidityMultiple  = 10
)

// CertMaterial is one PEM encoded certificate and its private key.
type CertMaterial struct {
	CertPEM []byte
	KeyPEM  []byte
}

// EtcdSecret is the Secret-shaped handoff object. Key material only travels
// in Data, never in logs or status.
//
//nolint:govet // fieldalignment: field order mirrors the Secret shape
type EtcdSecret struct {
	Name      string
	Namespace string
	Data      map[string][]byte
}

// EtcdPKI is all certificate material for one etcd cluster.
type EtcdPKI struct {
	CA          CertMaterial
	ServerCerts map[string]CertMaterial
	PeerCerts   map[string]CertMaterial
	Client      CertMaterial
}

// EtcdPKIOptions controls cert validity and makes tests deterministic.
//
//nolint:govet // fieldalignment: options read naturally in this order
type EtcdPKIOptions struct {
	Validity time.Duration
	Now      func() time.Time
}

func (o EtcdPKIOptions) withDefaults() EtcdPKIOptions {
	if o.Validity <= 0 {
		o.Validity = defaultCertValidity
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return o
}

// MintEtcdPKI creates a CA, a server and peer cert per member, and the client
// cert used by the control plane.
func MintEtcdPKI(cluster string, members []MemberSlot, opts EtcdPKIOptions) (*EtcdPKI, error) {
	opts = opts.withDefaults()
	now := opts.Now()

	caMaterial, caCert, caKey, err := mintCA(now, opts.Validity)
	if err != nil {
		return nil, err
	}

	pki := &EtcdPKI{
		CA:          caMaterial,
		ServerCerts: make(map[string]CertMaterial, len(members)),
		PeerCerts:   make(map[string]CertMaterial, len(members)),
	}
	for _, member := range members {
		server, mintErr := signMemberCert(caCert, caKey, member, now, opts.Validity,
			member.MemberName, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth})
		if mintErr != nil {
			return nil, mintErr
		}
		peer, mintErr := signMemberCert(caCert, caKey, member, now, opts.Validity,
			member.MemberName+"-peer", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth})
		if mintErr != nil {
			return nil, mintErr
		}
		pki.ServerCerts[member.MemberName] = server
		pki.PeerCerts[member.MemberName] = peer
	}

	client, err := signCert(caCert, caKey, certTemplate{
		commonName: "etcd-client-" + cluster,
		notBefore:  now,
		notAfter:   now.Add(opts.Validity),
		usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err != nil {
		return nil, err
	}
	pki.Client = client
	return pki, nil
}

// MemberSecret renders the cert bundle mounted by one member pod.
func (p *EtcdPKI) MemberSecret(member MemberSlot) EtcdSecret {
	server := p.ServerCerts[member.MemberName]
	peer := p.PeerCerts[member.MemberName]
	return EtcdSecret{
		Name:      member.MemberName + "-tls",
		Namespace: member.Namespace,
		Data: map[string][]byte{
			EtcdCACertKey:     p.CA.CertPEM,
			EtcdServerCertKey: server.CertPEM,
			EtcdServerKeyKey:  server.KeyPEM,
			EtcdPeerCertKey:   peer.CertPEM,
			EtcdPeerKeyKey:    peer.KeyPEM,
		},
	}
}

// ClientSecret renders the exact three-key secret the HyperShift seam expects.
func (p *EtcdPKI) ClientSecret(namespace string) EtcdSecret {
	return EtcdSecret{
		Name:      EtcdClientTLSName,
		Namespace: namespace,
		Data: map[string][]byte{
			EtcdClientCAKey:   p.CA.CertPEM,
			EtcdClientCertKey: p.Client.CertPEM,
			EtcdClientKeyKey:  p.Client.KeyPEM,
		},
	}
}

func mintCA(now time.Time, validity time.Duration) (CertMaterial, *x509.Certificate, *ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return CertMaterial{}, nil, nil, fmt.Errorf("generate etcd CA key: %w", err)
	}
	serial, err := newSerial()
	if err != nil {
		return CertMaterial{}, nil, nil, err
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "etcd-ca"},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.Add(validity * caValidityMultiple),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign |
			x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return CertMaterial{}, nil, nil, fmt.Errorf("create etcd CA: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return CertMaterial{}, nil, nil, fmt.Errorf("parse etcd CA: %w", err)
	}
	keyPEM, err := encodeKey(key)
	if err != nil {
		return CertMaterial{}, nil, nil, err
	}
	return CertMaterial{CertPEM: encodeCert(der), KeyPEM: keyPEM}, cert, key, nil
}

type certTemplate struct {
	commonName string
	notBefore  time.Time
	notAfter   time.Time
	usages     []x509.ExtKeyUsage
	dnsNames   []string
	ips        []net.IP
}

func signMemberCert(
	ca *x509.Certificate, caKey *ecdsa.PrivateKey, member MemberSlot,
	now time.Time, validity time.Duration, commonName string, usages []x509.ExtKeyUsage,
) (CertMaterial, error) {
	return signCert(ca, caKey, certTemplate{
		commonName: commonName,
		notBefore:  now,
		notAfter:   now.Add(validity),
		usages:     usages,
		dnsNames:   memberDNSNames(member),
		ips:        []net.IP{net.ParseIP("127.0.0.1")},
	})
}

func signCert(ca *x509.Certificate, caKey *ecdsa.PrivateKey, tmpl certTemplate) (CertMaterial, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return CertMaterial{}, fmt.Errorf("generate etcd cert key: %w", err)
	}
	serial, err := newSerial()
	if err != nil {
		return CertMaterial{}, err
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: tmpl.commonName},
		NotBefore:             tmpl.notBefore.Add(-5 * time.Minute),
		NotAfter:              tmpl.notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           tmpl.usages,
		DNSNames:              tmpl.dnsNames,
		IPAddresses:           tmpl.ips,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		return CertMaterial{}, fmt.Errorf("create etcd cert: %w", err)
	}
	keyPEM, err := encodeKey(key)
	if err != nil {
		return CertMaterial{}, err
	}
	return CertMaterial{CertPEM: encodeCert(der), KeyPEM: keyPEM}, nil
}

func memberDNSNames(member MemberSlot) []string {
	return []string{
		member.MemberName,
		member.MemberName + "." + member.Namespace,
		member.MemberName + "." + member.Namespace + ".svc",
		member.MemberName + "." + member.Namespace + ".svc.cluster.local",
		"localhost",
	}
}

func newSerial() (*big.Int, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate etcd cert serial: %w", err)
	}
	return serial, nil
}

func encodeCert(der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func encodeKey(key *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal etcd private key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}
