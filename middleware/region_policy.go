package middleware

import (
	"context"
	"starhub/model"
)

type deploymentRegionKey struct{}

func deploymentRegion(ctx context.Context) (*model.RegionProfile, error) {
	if ctx != nil {
		if profile, ok := ctx.Value(deploymentRegionKey{}).(*model.RegionProfile); ok {
			return profile, nil
		}
	}
	return model.ActiveRegionProfile()
}
