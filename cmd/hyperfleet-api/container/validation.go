package container

import (
	"context"
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validators"
)

func (c *Container) SchemaValidator() *validators.SchemaValidator {
	if c.schemaValidator == nil {
		schemaPath := c.cfg.Server.OpenAPISchemaPath
		schemaValidator, err := validators.NewSchemaValidator(schemaPath)
		if err != nil {
			panic(fmt.Sprintf("create schema validator: %v", err))
		}
		c.schemaValidator = schemaValidator
		logger.With(context.Background(), logger.FieldSchemaPath, schemaPath).Info("Schema validation enabled")
	}
	return c.schemaValidator
}
