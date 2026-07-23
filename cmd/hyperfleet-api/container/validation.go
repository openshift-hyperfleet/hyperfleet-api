package container

import (
	"context"
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validators"
)

func (c *Container) SchemaValidator() (*validators.SchemaValidator, error) {
	if c.schemaValidator == nil {
		schemaPath := c.cfg.Server.OpenAPISchemaPath
		schemaValidator, err := validators.NewSchemaValidator(schemaPath)
		if err != nil {
			return nil, fmt.Errorf("unable to create schema validator: %w", err)
		}
		c.schemaValidator = schemaValidator
		logger.With(context.Background(), logger.FieldSchemaPath, schemaPath).Info("Schema validation enabled")
	}
	return c.schemaValidator, nil
}
