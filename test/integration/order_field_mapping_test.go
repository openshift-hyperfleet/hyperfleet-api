package integration

import (
	"fmt"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
	"github.com/openshift-hyperfleet/hyperfleet-api/test/factories"
)

// TestOrderFieldMapping verifies that order parameters correctly map to database columns
// and produce properly sorted results.
func TestOrderFieldMapping(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create 5 clusters for ordering tests
	for range 5 {
		_, err := factories.NewClusterWithStatus(&h.Factories, h.DBFactory, h.NewID(), true, true)
		Expect(err).NotTo(HaveOccurred())
	}

	t.Run("OrderName", func(t *testing.T) {
		RegisterTestingT(t)

		orderAsc := openapi.QueryParamsOrder("name asc")
		params := &openapi.GetClustersParams{
			Order: &orderAsc,
		}
		resp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))

		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusOK))
		list := resp.JSON200
		Expect(list).NotTo(BeNil())

		// Verify ascending order - each name should be >= previous
		items := list.Items
		if len(items) >= 2 {
			for i := 1; i < len(items); i++ {
				prevName := items[i-1].Name
				currName := items[i].Name
				Expect(currName >= prevName).To(BeTrue(),
					fmt.Sprintf("Names should be in ascending order: %s >= %s", currName, prevName))
			}
		}

		// Order by desc
		orderDesc := openapi.QueryParamsOrder("name desc")
		params = &openapi.GetClustersParams{
			Order: &orderDesc,
		}
		resp, err = client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusOK))
		list = resp.JSON200
		Expect(list).NotTo(BeNil())
		items = list.Items
		if len(items) >= 2 {
			for i := 1; i < len(items); i++ {
				prevName := items[i-1].Name
				currName := items[i].Name
				Expect(currName <= prevName).To(BeTrue(),
					fmt.Sprintf("Names should be in descending order: %s <= %s", currName, prevName))
			}
		}
	})

	// order defaults to created_time desc
	t.Run("OrderDefault", func(t *testing.T) {
		RegisterTestingT(t)

		// Default ordering is created_time desc
		params := &openapi.GetClustersParams{}
		resp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))

		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusOK))
		list := resp.JSON200
		Expect(list).NotTo(BeNil())

		// Verify results are ordered by created_time descending (newest first)
		items := list.Items
		if len(items) >= 2 {
			// Each subsequent item should have created_time <= previous
			for i := 1; i < len(items); i++ {
				prevTime := items[i-1].CreatedTime
				currTime := items[i].CreatedTime
				Expect(currTime.Before(prevTime) || currTime.Equal(prevTime)).To(BeTrue(),
					fmt.Sprintf("created_time should be descending: %v should be <= %v",
						currTime, prevTime))
			}
		}
	})
	t.Run("MultipleOrderFields", func(t *testing.T) {
		RegisterTestingT(t)

		// Order by kind asc, then name desc
		order := openapi.QueryParamsOrder("kind asc,name desc")
		params := &openapi.GetClustersParams{
			Order: &order,
		}

		resp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))

		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusOK))
		list := resp.JSON200
		Expect(list).NotTo(BeNil())

		// Verify multi-level ordering
		items := list.Items
		if len(items) >= 2 {
			for i := 1; i < len(items); i++ {
				prevKind := items[i-1].Kind
				currKind := items[i].Kind
				prevName := items[i-1].Name
				currName := items[i].Name

				// Primary key: kind should be in ascending order (equal)
				Expect(currKind == prevKind).To(BeTrue(), fmt.Sprintf("Kinds should be equal: %s == %s", currKind, prevKind))

				// Secondary key: within same kind, names should be ascending
				Expect(currName <= prevName).To(BeTrue(),
					fmt.Sprintf("Within same kind, names should be descending: %s <= %s",
						currName, prevName))
			}
		}
	})
}

// TestOrderFieldValidation verifies that invalid order parameters return proper errors
func TestOrderFieldValidation(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	tests := []struct {
		name          string
		order         string
		expectedError string
	}{
		{
			name:          "InvalidFieldName",
			order:         "nonexistent_field asc",
			expectedError: "not allowed for ordering",
		},
		{
			name:          "InvalidDirection",
			order:         "name ascending",
			expectedError: "invalid order format",
		},
		{
			name:          "SQLInjectionAttempt1",
			order:         "name; DROP TABLE clusters",
			expectedError: "invalid order format",
		},
		{
			name:          "SQLInjectionAttempt2",
			order:         "name; (SELECT/**/tablename::int/**/FROM/**/pg_tables/**/LIMIT/**/1/**/OFFSET/**/N)",
			expectedError: "invalid order format",
		},
		{
			name:          "SQLInjectionAttempt3",
			order:         "pg_sleep(10)",
			expectedError: "invalid order format",
		},
		{
			name:          "SQLInjectionAttempt4",
			order:         "(SELECT/**/current_user::int)",
			expectedError: "invalid order format",
		},
		{
			name:          "TooManyParts",
			order:         "name asc extra",
			expectedError: "invalid order format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			order := openapi.QueryParamsOrder(tt.order)
			params := &openapi.GetClustersParams{
				Order: &order,
			}
			resp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))

			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest))
			Expect(string(resp.Body)).To(ContainSubstring(tt.expectedError))
		})
	}
}

// TestOrderAllowedFields verifies that all whitelisted fields work correctly
func TestOrderAllowedFields(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a test cluster
	_, err := factories.NewClusterWithStatus(&h.Factories, h.DBFactory, h.NewID(), true, true)
	Expect(err).NotTo(HaveOccurred())

	allowedFields := []string{
		"id",
		"name",
		"created_time",
		"updated_time",
		"kind",
		"created_by",
		"updated_by",
		"generation",
		"href",
		"deleted_time",
		"deleted_by",
	}

	for _, field := range allowedFields {
		t.Run(fmt.Sprintf("Order_%s", field), func(t *testing.T) {
			RegisterTestingT(t)

			order := openapi.QueryParamsOrder(fmt.Sprintf("%s asc", field))
			params := &openapi.GetClustersParams{
				Order: &order,
			}
			resp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))

			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusOK),
				fmt.Sprintf("Field %s should be allowed for ordering", field))
			Expect(resp.JSON200).NotTo(BeNil())
		})
	}
}

// TestOrderEmptyStrings verifies that empty order values are handled gracefully
func TestOrderEmptyStrings(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a test cluster
	_, err := factories.NewClusterWithStatus(&h.Factories, h.DBFactory, h.NewID(), true, true)
	Expect(err).NotTo(HaveOccurred())

	t.Run("EmptyOrder", func(t *testing.T) {
		RegisterTestingT(t)

		// Empty string should fall back to default (created_time desc)
		order := openapi.QueryParamsOrder("")
		params := &openapi.GetClustersParams{
			Order: &order,
		}
		resp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))

		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusOK))
		Expect(resp.JSON200).NotTo(BeNil())
	})

}
