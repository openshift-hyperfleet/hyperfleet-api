/*
Copyright (c) 2026 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// This file implements a Prometheus collector that reports database connection pool
// statistics. It reads sql.DB.Stats() on each Prometheus scrape to report the number
// of open connections and connections in use.

package db_metrics

import (
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"
)

// PoolCollector implements the prometheus.Collector interface to report
// database connection pool statistics.
type PoolCollector struct {
	db *sql.DB
}

// NewPoolCollector creates a new PoolCollector for the given database connection.
func NewPoolCollector(db *sql.DB) *PoolCollector {
	return &PoolCollector{db: db}
}

// Describe sends the descriptors for the connection pool metrics.
func (c *PoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- ConnectionsOpenDesc
	ch <- ConnectionsInUseDesc
}

// Collect reads the current connection pool stats and sends them as metrics.
func (c *PoolCollector) Collect(ch chan<- prometheus.Metric) {
	if c == nil || c.db == nil {
		return
	}
	stats := c.db.Stats()

	ch <- prometheus.MustNewConstMetric(
		ConnectionsOpenDesc,
		prometheus.GaugeValue,
		float64(stats.OpenConnections),
	)
	ch <- prometheus.MustNewConstMetric(
		ConnectionsInUseDesc,
		prometheus.GaugeValue,
		float64(stats.InUse),
	)
}

// RegisterPoolCollector creates and registers a new PoolCollector for the given database.
func RegisterPoolCollector(db *sql.DB) error {
	collector := NewPoolCollector(db)
	return prometheus.Register(collector)
}
