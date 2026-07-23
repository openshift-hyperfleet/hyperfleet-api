package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

type testAPIServerConfig struct {
	bindAddress  string
	tlsCertFile  string
	tlsKeyFile   string
	readTimeout  time.Duration
	writeTimeout time.Duration
	tlsEnabled   bool
}

func (c testAPIServerConfig) BindAddress() string {
	return c.bindAddress
}

func (c testAPIServerConfig) ReadTimeout() time.Duration {
	return c.readTimeout
}

func (c testAPIServerConfig) WriteTimeout() time.Duration {
	return c.writeTimeout
}

func (c testAPIServerConfig) TLSEnabled() bool {
	return c.tlsEnabled
}

func (c testAPIServerConfig) TLSCertFile() string {
	return c.tlsCertFile
}

func (c testAPIServerConfig) TLSKeyFile() string {
	return c.tlsKeyFile
}

func TestAPIServerServeWithoutTLS(t *testing.T) {
	RegisterTestingT(t)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())

	s := NewAPIServer(
		testAPIServerConfig{bindAddress: listener.Addr().String()},
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Serve(listener)
	}()

	var resp *http.Response
	Eventually(func() error {
		var err error
		resp, err = http.Get("http://" + listener.Addr().String())
		return err
	}, "2s", "25ms").Should(Succeed())
	t.Cleanup(func() { _ = resp.Body.Close() })
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

	Expect(s.Stop()).To(Succeed())
	<-done
}

func TestAPIServerServeWithTLS(t *testing.T) {
	RegisterTestingT(t)

	certFile, keyFile := writeSelfSignedCert(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())

	s := NewAPIServer(
		testAPIServerConfig{
			bindAddress: listener.Addr().String(),
			tlsEnabled:  true,
			tlsCertFile: certFile,
			tlsKeyFile:  keyFile,
		},
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}),
	)

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Serve(listener)
	}()

	certPEM, err := os.ReadFile(certFile)
	Expect(err).NotTo(HaveOccurred())
	certPool := x509.NewCertPool()
	Expect(certPool.AppendCertsFromPEM(certPEM)).To(BeTrue())

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: certPool, MinVersion: tls.VersionTLS13},
		},
	}

	var resp *http.Response
	Eventually(func() error {
		var err error
		resp, err = client.Get("https://" + listener.Addr().String())
		return err
	}, "2s", "25ms").Should(Succeed())
	t.Cleanup(func() { _ = resp.Body.Close() })
	Expect(resp.StatusCode).To(Equal(http.StatusAccepted))

	Expect(s.Stop()).To(Succeed())
	<-done
}

func writeSelfSignedCert(t *testing.T) (string, string) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate private key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	dir := t.TempDir()
	certFile := filepath.Join(dir, "server.crt")
	keyFile := filepath.Join(dir, "server.key")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	return certFile, keyFile
}
