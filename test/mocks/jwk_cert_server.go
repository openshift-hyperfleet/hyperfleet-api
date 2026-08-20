package mocks

import (
	"crypto"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/mendsley/gojwk"
)

const (
	certEndpoint = "/auth/realms/rhd/protocol/openid-connect/certs"
)

func NewJWKCertServerMock(
	pubKey crypto.PublicKey,
	jwkKID string,
	jwkAlg string,
) (url string, teardown func() error) {
	certHandler := http.NewServeMux()
	certHandler.HandleFunc(certEndpoint,
		func(w http.ResponseWriter, r *http.Request) {
			pubjwk, err := gojwk.PublicKey(pubKey)
			if err != nil {
				http.Error(w, fmt.Sprintf("generate public jwk: %v", err), http.StatusInternalServerError)
				return
			}
			pubjwk.Kid = jwkKID
			pubjwk.Alg = jwkAlg
			jwkBytes, err := gojwk.Marshal(pubjwk)
			if err != nil {
				http.Error(w, fmt.Sprintf("marshal public jwk: %v", err), http.StatusInternalServerError)
				return
			}
			fmt.Fprintf(w, `{"keys":[%s]}`, string(jwkBytes))
		},
	)

	server := httptest.NewServer(certHandler)
	return fmt.Sprintf("%s%s", server.URL, certEndpoint), serverClose(server)
}

func serverClose(server *httptest.Server) func() error {
	return func() error {
		server.Close()
		return nil
	}
}
