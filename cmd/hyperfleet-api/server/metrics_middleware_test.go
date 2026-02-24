/*
Copyright (c) 2019 Red Hat, Inc.

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

package server

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/gorilla/mux"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

const (
	testRequestsTotalMetric  = "hyperfleet_api_requests_total"
	testDurationMetric       = "hyperfleet_api_request_duration_seconds"
	testBuildInfoMetric      = "hyperfleet_api_build_info"
	testClustersPath         = "/api/hyperfleet/v1/clusters"
	testClustersPathWithID   = "/api/hyperfleet/v1/clusters/{id}"
	testClustersPathWithSub  = "/api/hyperfleet/v1/clusters/-"
	testClustersURLWithID    = "/api/hyperfleet/v1/clusters/abc123"
	testMethodGet            = "GET"
)

func TestMetricsSubsystem(t *testing.T) {
	RegisterTestingT(t)
	Expect(metricsSubsystem).To(Equal("hyperfleet_api"))
}

func TestMetricsNames(t *testing.T) {
	RegisterTestingT(t)
	Expect(MetricsNames).To(ContainElement("requests_total"))
	Expect(MetricsNames).To(ContainElement("request_duration_seconds"))
}

func TestMetricsLabels(t *testing.T) {
	RegisterTestingT(t)
	Expect(MetricsLabels).To(ContainElement("component"))
	Expect(MetricsLabels).To(ContainElement("version"))
	Expect(MetricsLabels).To(ContainElement("method"))
	Expect(MetricsLabels).To(ContainElement("path"))
	Expect(MetricsLabels).To(ContainElement("code"))
	Expect(MetricsLabels).To(HaveLen(5))
}

func TestBuildInfoMetricIsRegistered(t *testing.T) {
	RegisterTestingT(t)

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == testBuildInfoMetric {
			found = true
			Expect(mf.GetType()).To(Equal(dto.MetricType_GAUGE))
			Expect(mf.GetMetric()).To(HaveLen(1))

			metric := mf.GetMetric()[0]
			labels := make(map[string]string)
			for _, lp := range metric.GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}

			Expect(labels["component"]).To(Equal("api"))
			Expect(labels["version"]).To(Equal(api.Version))
			Expect(labels["commit"]).To(Equal(api.Commit))
			Expect(labels["go_version"]).To(Equal(runtime.Version()))
			Expect(metric.GetGauge().GetValue()).To(Equal(1.0))
			break
		}
	}
	Expect(found).To(BeTrue(), testBuildInfoMetric + " metric should be registered")
}

func TestMetricsMiddleware_IncrementsRequestCount(t *testing.T) {
	RegisterTestingT(t)
	ResetMetricCollectors()

	router := mux.NewRouter()
	router.Use(MetricsMiddleware)
	router.HandleFunc(testClustersPath, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, testClustersPath, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == testRequestsTotalMetric {
			found = true
			Expect(mf.GetType()).To(Equal(dto.MetricType_COUNTER))

			for _, metric := range mf.GetMetric() {
				labels := make(map[string]string)
				for _, lp := range metric.GetLabel() {
					labels[lp.GetName()] = lp.GetValue()
				}
				if labels["method"] == testMethodGet && labels["path"] == testClustersPath {
					Expect(labels["component"]).To(Equal("api"))
					Expect(labels["version"]).To(Equal(api.Version))
					Expect(labels["code"]).To(Equal("200"))
					Expect(metric.GetCounter().GetValue()).To(Equal(1.0))
				}
			}
			break
		}
	}
	Expect(found).To(BeTrue(), testRequestsTotalMetric + " metric should exist")
}

func TestMetricsMiddleware_RecordsDuration(t *testing.T) {
	RegisterTestingT(t)
	ResetMetricCollectors()

	router := mux.NewRouter()
	router.Use(MetricsMiddleware)
	router.HandleFunc(testClustersPath, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, testClustersPath, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == testDurationMetric {
			found = true
			Expect(mf.GetType()).To(Equal(dto.MetricType_HISTOGRAM))

			for _, metric := range mf.GetMetric() {
				labels := make(map[string]string)
				for _, lp := range metric.GetLabel() {
					labels[lp.GetName()] = lp.GetValue()
				}
				if labels["method"] == testMethodGet && labels["path"] == testClustersPath {
					Expect(metric.GetHistogram().GetSampleCount()).To(BeEquivalentTo(1))
					Expect(metric.GetHistogram().GetSampleSum()).To(BeNumerically(">", 0))
				}
			}
			break
		}
	}
	Expect(found).To(BeTrue(), testDurationMetric + " metric should exist")
}

func TestMetricsMiddleware_PathVariableSubstitution(t *testing.T) {
	RegisterTestingT(t)
	ResetMetricCollectors()

	router := mux.NewRouter()
	router.Use(MetricsMiddleware)
	router.HandleFunc(testClustersPathWithID, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, testClustersURLWithID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	for _, mf := range metricFamilies {
		if mf.GetName() == testRequestsTotalMetric {
			for _, metric := range mf.GetMetric() {
				labels := make(map[string]string)
				for _, lp := range metric.GetLabel() {
					labels[lp.GetName()] = lp.GetValue()
				}
				if labels["method"] == testMethodGet {
					Expect(labels["path"]).To(Equal(testClustersPathWithSub))
				}
			}
			break
		}
	}
}

func TestMetricsMiddleware_CapturesStatusCode(t *testing.T) {
	RegisterTestingT(t)
	ResetMetricCollectors()

	router := mux.NewRouter()
	router.Use(MetricsMiddleware)
	router.HandleFunc(testClustersPath, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, testClustersPath, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	for _, mf := range metricFamilies {
		if mf.GetName() == testRequestsTotalMetric {
			for _, metric := range mf.GetMetric() {
				labels := make(map[string]string)
				for _, lp := range metric.GetLabel() {
					labels[lp.GetName()] = lp.GetValue()
				}
				if labels["method"] == testMethodGet && labels["path"] == testClustersPath {
					Expect(labels["code"]).To(Equal("404"))
				}
			}
			break
		}
	}
}

func TestMetricsMiddleware_DefaultStatusCodeOnWrite(t *testing.T) {
	RegisterTestingT(t)
	ResetMetricCollectors()

	router := mux.NewRouter()
	router.Use(MetricsMiddleware)
	router.HandleFunc(testClustersPath, func(w http.ResponseWriter, r *http.Request) {
		// Write without calling WriteHeader - should default to 200
		_, _ = w.Write([]byte("ok"))
	}).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, testClustersPath, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	for _, mf := range metricFamilies {
		if mf.GetName() == testRequestsTotalMetric {
			for _, metric := range mf.GetMetric() {
				labels := make(map[string]string)
				for _, lp := range metric.GetLabel() {
					labels[lp.GetName()] = lp.GetValue()
				}
				if labels["method"] == testMethodGet && labels["path"] == testClustersPath {
					Expect(labels["code"]).To(Equal("200"))
				}
			}
			break
		}
	}
}

func TestHistogramBuckets(t *testing.T) {
	RegisterTestingT(t)

	expectedBuckets := []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	for _, mf := range metricFamilies {
		if mf.GetName() == testDurationMetric {
			for _, metric := range mf.GetMetric() {
				histogram := metric.GetHistogram()
				buckets := histogram.GetBucket()
				// +Inf bucket is implicit, so we check explicit buckets
				Expect(buckets).To(HaveLen(len(expectedBuckets)))
				for i, b := range buckets {
					Expect(b.GetUpperBound()).To(Equal(expectedBuckets[i]))
				}
			}
			break
		}
	}
}
