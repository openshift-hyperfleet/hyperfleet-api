package presenters

import (
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

// PresentError converts a ServiceError to RFC 9457 Problem Details format
func PresentError(err *errors.ServiceError, instance string, traceID string) openapi.Error {
	return err.AsProblemDetails(instance, traceID)
}
