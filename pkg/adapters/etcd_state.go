package adapters

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// etcdState is the durable part of one cluster's etcd ownership. Keeping the
// membership and PKI together means a restart neither re-mints certs under the
// running members nor forgets where a member was placed.
type etcdState struct {
	Membership Membership              `json:"membership"`
	CA         CertMaterial            `json:"ca"`
	Server     map[string]CertMaterial `json:"server"`
	Peer       map[string]CertMaterial `json:"peer"`
	Client     CertMaterial            `json:"client"`
}

func (s etcdState) pki() *EtcdPKI {
	return &EtcdPKI{
		CA:          s.CA,
		ServerCerts: s.Server,
		PeerCerts:   s.Peer,
		Client:      s.Client,
	}
}

func stateFrom(membership Membership, pki *EtcdPKI) etcdState {
	return etcdState{
		Membership: membership,
		CA:         pki.CA,
		Server:     pki.ServerCerts,
		Peer:       pki.PeerCerts,
		Client:     pki.Client,
	}
}

func loadEtcdState(dir, cluster string) (*etcdState, error) {
	if dir == "" {
		return nil, nil
	}
	path := filepath.Join(dir, cluster+".json")
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read etcd state %q: %w", path, err)
	}
	var state etcdState
	if err := json.Unmarshal(content, &state); err != nil {
		return nil, fmt.Errorf("decode etcd state %q: %w", path, err)
	}
	return &state, nil
}

func removeEtcdState(dir, cluster string) error {
	if dir == "" {
		return nil
	}
	path := filepath.Join(dir, cluster+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove etcd state %q: %w", path, err)
	}
	return nil
}

func saveEtcdState(dir string, state etcdState) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create etcd state directory: %w", err)
	}
	content, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode etcd state: %w", err)
	}
	path := filepath.Join(dir, state.Membership.ClusterName+".json")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("write etcd state %q: %w", path, err)
	}
	return nil
}
