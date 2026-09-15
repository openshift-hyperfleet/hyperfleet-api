package container

import (
	"context"
	"fmt"
	"log/slog"

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
		slog.InfoContext(context.Background(), "Schema validation enabled", logger.FieldSchemaPath, schemaPath)
	}
	return c.schemaValidator
}
