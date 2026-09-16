package services

import (
	stderrors "errors"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	serviceerrors "github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

type reconciliationOutcome struct {
	kind     string
	isDelete bool
}

func serviceErrorFromTransaction(err error) *serviceerrors.ServiceError {
	if err == nil {
		return nil
	}
	if svcErr, ok := stderrors.AsType[*serviceerrors.ServiceError](err); ok {
		return svcErr
	}
	if db.IsDBConnectionError(err) {
		return serviceerrors.ServiceUnavailable("Database connection unavailable")
	}
	return serviceerrors.GeneralError("")
}
