package adapters

import "context"

// StatusDataReader returns another adapter's published data for a cluster.
// A nil map means that adapter has not reported yet.
type StatusDataReader func(
	ctx context.Context, cluster ClusterView, adapter string,
) (map[string]interface{}, error)
