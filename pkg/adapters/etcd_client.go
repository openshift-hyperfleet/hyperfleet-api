package adapters

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strconv"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

const etcdClientOpTimeout = 10 * time.Second

// ClientV3 is the live MemberClient backed by etcd's own client.
type ClientV3 struct {
	client *clientv3.Client
}

var _ MemberClient = (*ClientV3)(nil)

// NewClientV3 connects to every member endpoint with the fleet client cert.
// The client balances and fails over across endpoints on its own.
func NewClientV3(endpoints []string, pki *EtcdPKI) (*ClientV3, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pki.CA.CertPEM) {
		return nil, fmt.Errorf("parse etcd CA certificate")
	}
	pair, err := tls.X509KeyPair(pki.Client.CertPEM, pki.Client.KeyPEM)
	if err != nil {
		return nil, fmt.Errorf("load etcd client certificate: %w", err)
	}
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: etcdClientOpTimeout,
		TLS: &tls.Config{
			MinVersion:   tls.VersionTLS12,
			RootCAs:      pool,
			Certificates: []tls.Certificate{pair},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create etcd client: %w", err)
	}
	return &ClientV3{client: client}, nil
}

// Close releases the client's connections.
func (c *ClientV3) Close() error {
	return c.client.Close()
}

// List returns etcd's own member list.
func (c *ClientV3) List(ctx context.Context) ([]EtcdMember, error) {
	ctx, cancel := context.WithTimeout(ctx, etcdClientOpTimeout)
	defer cancel()
	response, err := c.client.MemberList(ctx)
	if err != nil {
		return nil, fmt.Errorf("etcd member list: %w", err)
	}
	members := make([]EtcdMember, 0, len(response.Members))
	for _, member := range response.Members {
		peerURL := ""
		if len(member.PeerURLs) > 0 {
			peerURL = member.PeerURLs[0]
		}
		members = append(members, EtcdMember{
			ID:      strconv.FormatUint(member.ID, 16),
			Name:    member.Name,
			PeerURL: peerURL,
			Learner: member.IsLearner,
		})
	}
	return members, nil
}

// Remove removes a member by its hex ID.
func (c *ClientV3) Remove(ctx context.Context, id string) error {
	memberID, err := parseMemberID(id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, etcdClientOpTimeout)
	defer cancel()
	_, err = c.client.MemberRemove(ctx, memberID)
	return wrapEtcdOp(err, "remove member %s", id)
}

// Add adds a voting member. etcd learns its name when it joins.
func (c *ClientV3) Add(ctx context.Context, _ string, peerURL string) error {
	ctx, cancel := context.WithTimeout(ctx, etcdClientOpTimeout)
	defer cancel()
	_, err := c.client.MemberAdd(ctx, []string{peerURL})
	return wrapEtcdOp(err, "add member at %s", peerURL)
}

// AddAsLearner adds a non-voting member.
func (c *ClientV3) AddAsLearner(ctx context.Context, _ string, peerURL string) error {
	ctx, cancel := context.WithTimeout(ctx, etcdClientOpTimeout)
	defer cancel()
	_, err := c.client.MemberAddAsLearner(ctx, []string{peerURL})
	return wrapEtcdOp(err, "add learner at %s", peerURL)
}

// Promote promotes a learner to a voting member.
func (c *ClientV3) Promote(ctx context.Context, id string) error {
	memberID, err := parseMemberID(id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, etcdClientOpTimeout)
	defer cancel()
	_, err = c.client.MemberPromote(ctx, memberID)
	return wrapEtcdOp(err, "promote member %s", id)
}

func parseMemberID(id string) (uint64, error) {
	memberID, err := strconv.ParseUint(id, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("parse etcd member ID %q: %w", id, err)
	}
	return memberID, nil
}

func wrapEtcdOp(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(format+": %w", append(args, err)...)
}
