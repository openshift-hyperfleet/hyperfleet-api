package container

import (
	"context"
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/auth"
)

func (c *Container) JWTHandler() (*auth.JWTHandler, error) {
	if c.jwtHandler == nil {
		jwtHandler, err := auth.NewJWTHandler(
			context.Background(),
			auth.JWTHandlerConfig{
				Issuers: c.cfg.Server.JWT.Configs,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("unable to create JWT handler: %w", err)
		}
		c.jwtHandler = jwtHandler
	}
	return c.jwtHandler, nil
}
