// Command etcd-client-standin is the kill-test client: it writes sequential
// keys to every member endpoint and reports its own failover behavior.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

func main() {
	var endpoints string
	var caFile, certFile, keyFile, prefix string
	var interval time.Duration
	flag.StringVar(&endpoints, "endpoints", "",
		"comma-separated etcd client endpoints")
	flag.StringVar(&caFile, "ca", "", "CA certificate file")
	flag.StringVar(&certFile, "cert", "", "client certificate file")
	flag.StringVar(&keyFile, "key", "", "client key file")
	flag.StringVar(&prefix, "prefix", "standin", "key prefix")
	flag.DurationVar(&interval, "interval", 500*time.Millisecond, "write interval")
	flag.Parse()

	if endpoints == "" || caFile == "" || certFile == "" || keyFile == "" {
		fmt.Fprintln(os.Stderr, "endpoints, ca, cert, and key are required")
		os.Exit(2)
	}

	tlsConfig, err := loadTLS(caFile, certFile, keyFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load TLS: %v\n", err)
		os.Exit(1)
	}
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   strings.Split(endpoints, ","),
		DialTimeout: 5 * time.Second,
		TLS:         tlsConfig,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "create client: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = client.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("START %d %s\n", time.Now().UnixNano(), endpoints)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	sequence := 0
	for {
		select {
		case <-ctx.Done():
			fmt.Println("STOP")
			return
		case <-ticker.C:
			sequence++
			write(ctx, client, prefix, sequence)
		}
	}
}

func write(ctx context.Context, client *clientv3.Client, prefix string, sequence int) {
	key := fmt.Sprintf("%s-%08d", prefix, sequence)
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	now := time.Now().UnixNano()
	if _, err := client.Put(writeCtx, key, fmt.Sprintf("%d", now)); err != nil {
		fmt.Printf("ERR %d %s %v\n", now, key, err)
		return
	}
	fmt.Printf("OK %d %s\n", now, key)
}

func loadTLS(caFile, certFile, keyFile string) (*tls.Config, error) {
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("parse CA certificate")
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      pool,
		Certificates: []tls.Certificate{cert},
	}, nil
}
