package presenters

import (
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
)

func PresentReference(id, obj interface{}) openapi.ObjectReference {
	refID, ok := makeReferenceID(id)

	if !ok {
		return openapi.ObjectReference{}
	}

	return openapi.ObjectReference{
		Id:   util.PtrString(refID),
		Kind: ObjectKind(obj),
		Href: ObjectPath(refID, obj),
	}
}

func makeReferenceID(id interface{}) (string, bool) {
	var refID string

	if i, ok := id.(string); ok {
		refID = i
	}

	if i, ok := id.(*string); ok {
		if i != nil {
			refID = *i
		}
	}

	return refID, refID != ""
}
