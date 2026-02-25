/*
Copyright (c) 2025 Red Hat, Inc.

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

// This file implements a GORM plugin that instruments database operations with
// Prometheus metrics. It registers callbacks for create, query, update, delete,
// and raw operations to measure query duration and track errors.

package db_metrics

import (
	"errors"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

const (
	callbackPrefix = "hyperfleet_metrics"
	startTimeKey   = "hyperfleet_metrics:start_time"
)

// RegisterPlugin registers the metrics GORM callbacks on the provided
// *gorm.DB. It adds before/after callbacks for create, query, update,
// delete, and raw operations.
func RegisterPlugin(db *gorm.DB) error {
	cb := db.Callback()
	before := callbackPrefix + ":before_"
	after := callbackPrefix + ":after_"

	// Create callbacks
	err := cb.Create().Before("gorm:create").
		Register(before+"create", beforeCallback)
	if err != nil {
		return err
	}
	err = cb.Create().After("gorm:create").
		Register(after+"create", afterCallback("create"))
	if err != nil {
		return err
	}

	// Query callbacks
	err = cb.Query().Before("gorm:query").
		Register(before+"query", beforeCallback)
	if err != nil {
		return err
	}
	err = cb.Query().After("gorm:after_query").
		Register(after+"query", afterCallback("query"))
	if err != nil {
		return err
	}

	// Update callbacks
	err = cb.Update().Before("gorm:update").
		Register(before+"update", beforeCallback)
	if err != nil {
		return err
	}
	err = cb.Update().After("gorm:update").
		Register(after+"update", afterCallback("update"))
	if err != nil {
		return err
	}

	// Delete callbacks
	err = cb.Delete().Before("gorm:delete").
		Register(before+"delete", beforeCallback)
	if err != nil {
		return err
	}
	err = cb.Delete().After("gorm:delete").
		Register(after+"delete", afterCallback("delete"))
	if err != nil {
		return err
	}

	// Raw callbacks
	err = cb.Raw().Before("gorm:raw").
		Register(before+"raw", beforeCallback)
	if err != nil {
		return err
	}
	err = cb.Raw().After("gorm:raw").
		Register(after+"raw", afterCallback("raw"))
	if err != nil {
		return err
	}

	return nil
}

// beforeCallback stores the current time in the GORM statement settings.
func beforeCallback(db *gorm.DB) {
	db.InstanceSet(startTimeKey, time.Now())
}

// afterCallback returns a GORM callback that records query duration and errors.
func afterCallback(operation string) func(*gorm.DB) {
	return func(db *gorm.DB) {
		startVal, ok := db.InstanceGet(startTimeKey)
		if !ok {
			return
		}
		startTime, ok := startVal.(time.Time)
		if !ok {
			return
		}
		elapsed := time.Since(startTime)

		table := extractTableName(db)

		// Record query duration
		QueryDurationMetric.With(prometheus.Labels{
			labelOperation: operation,
			labelTable:     table,
			labelComponent: componentValue,
			labelVersion:   api.Version,
		}).Observe(elapsed.Seconds())

		// Record errors
		if db.Error != nil && !errors.Is(db.Error, gorm.ErrRecordNotFound) {
			errorType := classifyError(db.Error)
			ErrorsMetric.With(prometheus.Labels{
				labelOperation: operation,
				labelErrorType: errorType,
				labelComponent: componentValue,
				labelVersion:   api.Version,
			}).Inc()
		}
	}
}

// extractTableName gets the table name from the GORM statement.
func extractTableName(db *gorm.DB) string {
	if db.Statement != nil && db.Statement.Table != "" {
		return db.Statement.Table
	}
	if db.Statement != nil && db.Statement.Schema != nil {
		return db.Statement.Schema.Table
	}
	return "unknown"
}

// classifyError categorizes a database error into a low-cardinality error_type label.
func classifyError(err error) string {
	if err == nil {
		return ""
	}
	errMsg := strings.ToLower(err.Error())

	switch {
	case strings.Contains(errMsg, "unique") ||
		strings.Contains(errMsg, "duplicate") ||
		strings.Contains(errMsg, "violates") ||
		strings.Contains(errMsg, "constraint"):
		return "constraint_violation"
	case strings.Contains(errMsg, "connection") ||
		strings.Contains(errMsg, "refused") ||
		strings.Contains(errMsg, "reset by peer") ||
		strings.Contains(errMsg, "broken pipe"):
		return "connection_error"
	case strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "deadline exceeded") ||
		strings.Contains(errMsg, "canceling statement"):
		return "timeout"
	default:
		return "other"
	}
}
