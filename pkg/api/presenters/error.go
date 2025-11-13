package presenters

import (
	"github.com/openshift-hyperfleet/hyperfleet/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/errors"
)

func PresentError(err *errors.ServiceError) openapi.Error {
	return err.AsOpenapiError("")
}
