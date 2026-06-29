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
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

const (
	testReconciliationStartedTotalMetric = "hyperfleet_api_reconciliation_started_total"
)

func TestReconciliationStartedTotalIsRegistered(t *testing.T) {
	RegisterTestingT(t)
	ResetReconciliationMetrics()

	RecordReconciliationStarted(testResourceCluster, false)

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == testReconciliationStartedTotalMetric {
			found = true
			Expect(mf.GetType()).To(Equal(dto.MetricType_COUNTER))
			break
		}
	}
	Expect(found).To(BeTrue(), testReconciliationStartedTotalMetric+" metric should be registered")
}

func TestRecordReconciliationStarted_IncrementsCounter(t *testing.T) {
	RegisterTestingT(t)
	ResetReconciliationMetrics()

	RecordReconciliationStarted(testResourceCluster, false)
	RecordReconciliationStarted(testResourceCluster, false)
	RecordReconciliationStarted(testResourceNodepool, true)

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	var clusterCount, nodepoolDeleteCount float64
	for _, mf := range metricFamilies {
		if mf.GetName() == testReconciliationStartedTotalMetric {
			for _, metric := range mf.GetMetric() {
				labels := labelsToMap(metric)
				if labels["resource_type"] == testResourceCluster && labels["is_delete"] == "false" {
					clusterCount = metric.GetCounter().GetValue()
				}
				if labels["resource_type"] == testResourceNodepool && labels["is_delete"] == "true" {
					nodepoolDeleteCount = metric.GetCounter().GetValue()
				}
			}
			break
		}
	}
	Expect(clusterCount).To(Equal(2.0))
	Expect(nodepoolDeleteCount).To(Equal(1.0))
}

func TestRecordReconciliationStarted_Labels(t *testing.T) {
	RegisterTestingT(t)
	ResetReconciliationMetrics()

	RecordReconciliationStarted(testResourceCluster, false)

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == testReconciliationStartedTotalMetric {
			for _, metric := range mf.GetMetric() {
				labels := labelsToMap(metric)
				if labels["resource_type"] == testResourceCluster {
					found = true
					Expect(labels["is_delete"]).To(Equal("false"))
					Expect(labels["component"]).To(Equal("api"))
					Expect(labels["version"]).To(Equal(api.Version))
				}
			}
			break
		}
	}
	Expect(found).To(BeTrue(), "reconciliation started metric with expected labels should exist")
}

func TestRecordReconciliationStarted_IsDeleteLabelValues(t *testing.T) {
	RegisterTestingT(t)
	ResetReconciliationMetrics()

	RecordReconciliationStarted(testResourceCluster, true)
	RecordReconciliationStarted(testResourceCluster, false)

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	isDeleteValues := map[string]bool{}
	for _, mf := range metricFamilies {
		if mf.GetName() == testReconciliationStartedTotalMetric {
			for _, metric := range mf.GetMetric() {
				labels := labelsToMap(metric)
				if labels["resource_type"] == testResourceCluster {
					isDeleteValues[labels["is_delete"]] = true
				}
			}
			break
		}
	}
	Expect(isDeleteValues).To(HaveKey("true"))
	Expect(isDeleteValues).To(HaveKey("false"))
}

func TestResetReconciliationMetrics_ClearsAllMetrics(t *testing.T) {
	RegisterTestingT(t)

	RecordReconciliationStarted(testResourceCluster, false)

	ResetReconciliationMetrics()

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	for _, mf := range metricFamilies {
		if mf.GetName() == testReconciliationStartedTotalMetric {
			Expect(mf.GetMetric()).To(BeEmpty(), "reconciliation started total should be empty after reset")
		}
	}
}

func TestReconciliationCollector_Describe(t *testing.T) {
	RegisterTestingT(t)

	collector := NewReconciliationCollector(nil, 10*time.Minute)
	ch := make(chan *prometheus.Desc, 10)
	collector.Describe(ch)
	close(ch)

	var descs []*prometheus.Desc
	for desc := range ch {
		descs = append(descs, desc)
	}

	Expect(descs).To(HaveLen(3))
}

func TestReconciliationCollector_CollectWithNilDB(t *testing.T) {
	RegisterTestingT(t)

	collector := NewReconciliationCollector(nil, 10*time.Minute)
	ch := make(chan prometheus.Metric, 10)
	collector.Collect(ch)
	close(ch)

	var collectedMetrics []prometheus.Metric
	for m := range ch {
		collectedMetrics = append(collectedMetrics, m)
	}

	Expect(collectedMetrics).To(BeEmpty())
}

func TestReconciliationCollector_DescriptorNames(t *testing.T) {
	RegisterTestingT(t)

	collector := NewReconciliationCollector(nil, 10*time.Minute)

	Expect(collector.pendingDesc.String()).To(ContainSubstring("resource_pending_reconciliation"))
	Expect(collector.stuckDesc.String()).To(ContainSubstring("resource_pending_reconciliation_stuck"))
	Expect(collector.durationDesc.String()).To(ContainSubstring("resource_pending_reconciliation_stuck_duration_seconds"))
}

func TestReconciliationCollector_DescriptorLabels(t *testing.T) {
	RegisterTestingT(t)

	collector := NewReconciliationCollector(nil, 10*time.Minute)

	for _, desc := range []*prometheus.Desc{collector.pendingDesc, collector.stuckDesc, collector.durationDesc} {
		descStr := desc.String()
		Expect(descStr).To(ContainSubstring("resource_type"))
		Expect(descStr).To(ContainSubstring("is_delete"))
		Expect(descStr).To(ContainSubstring("component"))
		Expect(descStr).To(ContainSubstring("version"))
	}
}
