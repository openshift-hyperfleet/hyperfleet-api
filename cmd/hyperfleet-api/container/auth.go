package container

import (
	"context"
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/auth"
)

func (c *Container) JWTHandler() *auth.JWTHandler {
	if !c.cfg.Server.JWT.Enabled {
		return nil
	}
	if c.jwtHandler == nil {
		jwtHandler, err := auth.NewJWTHandler(
			context.Background(),
			auth.JWTHandlerConfig{
				Issuers: c.cfg.Server.JWT.Configs,
			},
		)
		if err != nil {
			panic(fmt.Sprintf("create JWT handler: %v", err))
		}
		c.jwtHandler = jwtHandler
	}
	return c.jwtHandler
}
