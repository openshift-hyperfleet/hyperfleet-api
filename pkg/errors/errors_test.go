package errors

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestErrorFormatting(t *testing.T) {
	RegisterTestingT(t)
	err := New(CodeInternalGeneral, "test %s, %d", "errors", 1)
	Expect(err.Reason).To(Equal("test errors, 1"))
}

func TestErrorFind(t *testing.T) {
	RegisterTestingT(t)
	exists, err := Find(CodeNotFoundGeneric)
	Expect(exists).To(Equal(true))
	Expect(err.Type).To(Equal(ErrorTypeNotFound))
	Expect(err.RFC9457Code).To(Equal(CodeNotFoundGeneric))

	// Test with invalid code
	exists, err = Find("INVALID-CODE-999")
	Expect(exists).To(Equal(false))
	Expect(err).To(BeNil())
}
