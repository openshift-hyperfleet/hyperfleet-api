package services

import (
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"

	"github.com/onsi/gomega/types"
	"github.com/yaacov/tree-search-language/v6/pkg/tsl"

	. "github.com/onsi/gomega"
)

func TestSQLTranslation(t *testing.T) {
	RegisterTestingT(t)
	cfg := db.WalkConfig{TableName: "resources"}

	errorTests := []map[string]interface{}{
		{
			"search": "= = =",
			"error":  errors.CodeBadRequest + ": Failed to parse search query: = = =",
		},
	}
	for _, test := range errorTests {
		search := test["search"].(string)
		errorMsg := test["error"].(string)

		tree, parseErr := tsl.ParseTSL(search)
		if parseErr != nil {
			Expect(errors.BadRequest("Failed to parse search query: %s", search).Error()).To(Equal(errorMsg))
			continue
		}

		_, _, serviceErr := db.TSLToSQL(tree, cfg)
		Expect(serviceErr).To(HaveOccurred())
		Expect(serviceErr.Type).To(Equal(errors.ErrorTypeBadRequest))
		Expect(serviceErr.Error()).To(Equal(errorMsg))
	}

	sqlTests := []map[string]interface{}{
		{
			"search": "created_by in ['ooo.openshift']",
			"sql":    "resources.created_by IN (?)",
			"values": ConsistOf("ooo.openshift"),
		},
		{
			"search": "spec.region = 'us-east-1'",
			"sql":    "spec->>'region' = ?",
			"values": ConsistOf("us-east-1"),
		},
		{
			"search": "spec.release.version = '2'",
			"sql":    "spec->'release'->>'version' = ?",
			"values": ConsistOf("2"),
		},
		{
			"search": "spec.release.notes.url = 'https://example.com'",
			"sql":    "spec->'release'->'notes'->>'url' = ?",
			"values": ConsistOf("https://example.com"),
		},
		{
			"search": "spec.replicas > 9",
			"sql":    "CAST(spec->>'replicas' AS numeric) > ?",
			"values": ConsistOf(float64(9)),
		},
		{
			"search": "spec.release.version > 9",
			"sql":    "CAST(spec->'release'->>'version' AS numeric) > ?",
			"values": ConsistOf(float64(9)),
		},
		{
			"search": "spec.release.config.replicas > 9",
			"sql":    "CAST(spec->'release'->'config'->>'replicas' AS numeric) > ?",
			"values": ConsistOf(float64(9)),
		},
		{
			"search": "id = 'cls-123'",
			"sql":    "resources.id = ?",
			"values": ConsistOf("cls-123"),
		},
	}
	for _, test := range sqlTests {
		search := test["search"].(string)
		sqlReal := test["sql"].(string)
		valuesReal := test["values"].(types.GomegaMatcher)

		tslTree, err := tsl.ParseTSL(search)
		Expect(err).ToNot(HaveOccurred())
		sql, values, serviceErr := db.TSLToSQL(tslTree, cfg)
		Expect(serviceErr).To(BeNil())
		Expect(sql).To(Equal(sqlReal))
		Expect(values).To(valuesReal)
	}

	jsonbRelatedTableTests := []map[string]interface{}{
		{
			"search":      "spec.release.version = '2'",
			"sqlContains": "spec->'release'->>'version'",
			"values":      ConsistOf("2"),
		},
	}
	for _, test := range jsonbRelatedTableTests {
		search := test["search"].(string)
		sqlContains := test["sqlContains"].(string)
		valuesReal := test["values"].(types.GomegaMatcher)

		tslTree, err := tsl.ParseTSL(search)
		Expect(err).ToNot(HaveOccurred())
		sql, values, serviceErr := db.TSLToSQL(tslTree, cfg)
		Expect(serviceErr).To(BeNil(), "spec field should not be misclassified as related table: %s", search)
		Expect(sql).To(ContainSubstring(sqlContains))
		Expect(values).To(valuesReal)
	}
}
