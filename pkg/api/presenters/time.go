package presenters

import (
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
)

func PresentTime(t time.Time) *time.Time {
	if t.IsZero() {
		return util.ToPtr(time.Time{})
	}
	return util.ToPtr(t.Round(time.Microsecond))
}
