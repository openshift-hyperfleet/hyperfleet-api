package services

import (
	"context"
	"encoding/json"
	e "errors"
	"reflect"
	"strings"

	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

func jsonEqual(a, b []byte) bool {
	var va, vb any
	if err := json.Unmarshal(a, &va); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &vb); err != nil {
		return false
	}
	return reflect.DeepEqual(va, vb)
}

// Field names suspected to contain personally identifiable information
var piiFields = []string{
	"username",
	"first_name",
	"last_name",
	"email",
	"address",
}

func handleGetError(resourceType, field string, value interface{}, err error) *errors.ServiceError {
	// Sanitize errors of any personally identifiable information
	for _, f := range piiFields {
		if field == f {
			value = "<redacted>"
			break
		}
	}
	if e.Is(err, gorm.ErrRecordNotFound) {
		return errors.NotFound("%s with %s='%v' not found", resourceType, field, value)
	}
	if db.IsDBConnectionError(err) {
		return errors.ServiceUnavailable("Database connection unavailable")
	}
	return errors.GeneralError("Unable to find %s with %s='%v': %s", resourceType, field, value, err)
}

func handleCreateError(resourceType string, err error) *errors.ServiceError {
	if db.IsDBConnectionError(err) {
		return errors.ServiceUnavailable("Database connection unavailable")
	}
	if strings.Contains(err.Error(), "violates unique constraint") {
		return errors.Conflict("This %s already exists", resourceType)
	}
	return errors.GeneralError("Unable to create %s: %s", resourceType, err.Error())
}

func handleUpdateError(resourceType string, err error) *errors.ServiceError {
	if db.IsDBConnectionError(err) {
		return errors.ServiceUnavailable("Database connection unavailable")
	}
	if strings.Contains(err.Error(), "violates unique constraint") {
		return errors.Conflict("Changes to %s conflict with existing records", resourceType)
	}
	return errors.GeneralError("Unable to update %s: %s", resourceType, err.Error())
}

func handleSoftDeleteError(resourceType string, err error) *errors.ServiceError {
	if e.Is(err, gorm.ErrRecordNotFound) {
		return errors.NotFound("%s not found", resourceType)
	}
	if db.IsDBConnectionError(err) {
		return errors.ServiceUnavailable("Database connection unavailable")
	}
	return errors.GeneralError("Unable to soft-delete %s: %s", resourceType, err.Error())
}

func handleDeleteError(resourceType string, err error) *errors.ServiceError {
	if db.IsDBConnectionError(err) {
		return errors.ServiceUnavailable("Database connection unavailable")
	}
	return errors.GeneralError("Unable to delete %s: %s", resourceType, err.Error())
}

type adapterSummary struct {
	Conditions map[string]string `json:"conditions"`
	Adapter    string            `json:"adapter"`
}

func buildAdapterSummaries(ctx context.Context, statuses api.AdapterStatusList) []adapterSummary {
	summaries := make([]adapterSummary, 0, len(statuses))
	for _, st := range statuses {
		conds := make(map[string]string)
		var parsed []api.AdapterCondition
		if err := json.Unmarshal(st.Conditions, &parsed); err != nil {
			logger.With(ctx, "adapter", st.Adapter).WithError(err).Warn("Failed to parse adapter conditions for summary")
		} else {
			for _, c := range parsed {
				conds[c.Type] = string(c.Status)
			}
		}
		summaries = append(summaries, adapterSummary{Adapter: st.Adapter, Conditions: conds})
	}
	return summaries
}
