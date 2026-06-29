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

package metrics

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

const labelIsDelete = "is_delete"

var reconciliationLabels = []string{labelResourceType, labelIsDelete}

var reconciliationStartedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Subsystem: metricsSubsystem,
		Name:      "reconciliation_started_total",
		Help: "Total number of resources that entered the unreconciled state " +
			"(Reconciled condition transitioned to False).",
		ConstLabels: prometheus.Labels{labelComponent: componentValue, labelVersion: api.Version},
	},
	reconciliationLabels,
)

var reconciliationRegisterOnce sync.Once

func RegisterReconciliationMetrics() {
	reconciliationRegisterOnce.Do(func() {
		prometheus.MustRegister(reconciliationStartedTotal)
	})
}

func init() {
	RegisterReconciliationMetrics()
}

func RecordReconciliationStarted(resourceType string, isDelete bool) {
	reconciliationStartedTotal.With(prometheus.Labels{
		labelResourceType: resourceType,
		labelIsDelete:     fmt.Sprintf("%t", isDelete),
	}).Inc()
}

func ResetReconciliationMetrics() {
	reconciliationStartedTotal.Reset()
}

type ReconciliationCollector struct {
	pendingDesc  *prometheus.Desc
	stuckDesc    *prometheus.Desc
	durationDesc *prometheus.Desc

	db             *sql.DB
	stuckThreshold time.Duration
	queryTimeout   time.Duration
}

func NewReconciliationCollector(db *sql.DB, stuckThreshold time.Duration) *ReconciliationCollector {
	constLabels := prometheus.Labels{labelComponent: componentValue, labelVersion: api.Version}
	variableLabels := []string{labelResourceType, labelIsDelete}

	return &ReconciliationCollector{
		db:             db,
		stuckThreshold: stuckThreshold,
		queryTimeout:   defaultQueryTimeout,
		pendingDesc: prometheus.NewDesc(
			metricsSubsystem+"_resource_pending_reconciliation",
			"Number of resources currently pending reconciliation (Reconciled=False).",
			variableLabels,
			constLabels,
		),
		stuckDesc: prometheus.NewDesc(
			metricsSubsystem+"_resource_pending_reconciliation_stuck",
			"Number of resources pending reconciliation beyond the stuck threshold.",
			variableLabels,
			constLabels,
		),
		durationDesc: prometheus.NewDesc(
			metricsSubsystem+"_resource_pending_reconciliation_stuck_duration_seconds",
			"Maximum duration in seconds that any resource has been stuck pending reconciliation.",
			variableLabels,
			constLabels,
		),
	}
}

func (c *ReconciliationCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.pendingDesc
	ch <- c.stuckDesc
	ch <- c.durationDesc
}

type reconciliationRow struct {
	resourceType string
	isDelete     string
	value        float64
}

//nolint:lll // SQL readability — breaking these lines across Go string boundaries would harm clarity
const (
	// pendingQuery counts resources with Reconciled=False, grouped by resource type and deletion state.
	pendingQuery = `
SELECT resource_type, is_delete, COUNT(*) AS cnt
FROM (
    SELECT 'cluster' AS resource_type,
           CASE WHEN deleted_time IS NOT NULL THEN 'true' ELSE 'false' END AS is_delete
    FROM clusters
    WHERE (jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Reconciled")') ->> 'status') = 'False'
    UNION ALL
    SELECT 'nodepool' AS resource_type,
           CASE WHEN deleted_time IS NOT NULL THEN 'true' ELSE 'false' END AS is_delete
    FROM node_pools
    WHERE (jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Reconciled")') ->> 'status') = 'False'
) sub
GROUP BY resource_type, is_delete`

	// stuckQuery counts resources with Reconciled=False whose last_transition_time is older than the threshold.
	stuckQuery = `
SELECT resource_type, is_delete, COUNT(*) AS cnt
FROM (
    SELECT 'cluster' AS resource_type,
           CASE WHEN deleted_time IS NOT NULL THEN 'true' ELSE 'false' END AS is_delete
    FROM clusters
    WHERE (jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Reconciled")') ->> 'status') = 'False'
      AND (jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Reconciled")') ->> 'last_transition_time')::timestamptz < $1
    UNION ALL
    SELECT 'nodepool' AS resource_type,
           CASE WHEN deleted_time IS NOT NULL THEN 'true' ELSE 'false' END AS is_delete
    FROM node_pools
    WHERE (jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Reconciled")') ->> 'status') = 'False'
      AND (jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Reconciled")') ->> 'last_transition_time')::timestamptz < $1
) sub
GROUP BY resource_type, is_delete`

	// maxDurationQuery returns the maximum stuck duration per resource type and deletion state.
	maxDurationQuery = `
SELECT resource_type, is_delete, MAX(duration_seconds) AS max_duration
FROM (
    SELECT 'cluster' AS resource_type,
           CASE WHEN deleted_time IS NOT NULL THEN 'true' ELSE 'false' END AS is_delete,
           EXTRACT(EPOCH FROM (NOW() - (jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Reconciled")') ->> 'last_transition_time')::timestamptz)) AS duration_seconds
    FROM clusters
    WHERE (jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Reconciled")') ->> 'status') = 'False'
      AND (jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Reconciled")') ->> 'last_transition_time')::timestamptz < $1
    UNION ALL
    SELECT 'nodepool' AS resource_type,
           CASE WHEN deleted_time IS NOT NULL THEN 'true' ELSE 'false' END AS is_delete,
           EXTRACT(EPOCH FROM (NOW() - (jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Reconciled")') ->> 'last_transition_time')::timestamptz)) AS duration_seconds
    FROM node_pools
    WHERE (jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Reconciled")') ->> 'status') = 'False'
      AND (jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Reconciled")') ->> 'last_transition_time')::timestamptz < $1
) sub
GROUP BY resource_type, is_delete`
)

func (c *ReconciliationCollector) Collect(ch chan<- prometheus.Metric) {
	if c == nil || c.db == nil {
		return
	}

	threshold := time.Now().UTC().Add(-c.stuckThreshold)

	c.collectRows(ch, c.pendingDesc, pendingQuery, nil)
	c.collectRows(ch, c.stuckDesc, stuckQuery, &threshold)
	c.collectRows(ch, c.durationDesc, maxDurationQuery, &threshold)
}

func (c *ReconciliationCollector) collectRows(
	ch chan<- prometheus.Metric,
	desc *prometheus.Desc,
	query string,
	threshold *time.Time,
) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()

	var rows *sql.Rows
	var err error
	if threshold != nil {
		rows, err = c.db.QueryContext(ctx, query, *threshold) //nolint:gosec // table names are compile-time constants
	} else {
		rows, err = c.db.QueryContext(ctx, query) //nolint:gosec // table names are compile-time constants
	}
	if err != nil {
		logger.WithError(ctx, err).Error("Failed to query reconciliation metrics")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var r reconciliationRow
		if err := rows.Scan(&r.resourceType, &r.isDelete, &r.value); err != nil {
			logger.WithError(ctx, err).Error("Failed to scan reconciliation metric row")
			continue
		}
		ch <- prometheus.MustNewConstMetric(
			desc,
			prometheus.GaugeValue,
			r.value,
			r.resourceType, r.isDelete,
		)
	}
	if err := rows.Err(); err != nil {
		logger.WithError(ctx, err).Error("Error iterating reconciliation metric rows")
	}
}

func RegisterReconciliationCollector(db *sql.DB, stuckThreshold time.Duration) error {
	collector := NewReconciliationCollector(db, stuckThreshold)
	return prometheus.Register(collector)
}
