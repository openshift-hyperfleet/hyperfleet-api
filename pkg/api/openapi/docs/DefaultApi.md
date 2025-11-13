# \DefaultAPI

All URIs are relative to *http://localhost:8000*

Method | HTTP request | Description
------------- | ------------- | -------------
[**CreateNodePool**](DefaultAPI.md#CreateNodePool) | **Post** /api/hyperfleet/v1/clusters/{cluster_id}/nodepools | Create nodepool
[**GetClusterById**](DefaultAPI.md#GetClusterById) | **Get** /api/hyperfleet/v1/clusters/{cluster_id} | Get cluster by ID
[**GetClusterStatuses**](DefaultAPI.md#GetClusterStatuses) | **Get** /api/hyperfleet/v1/clusters/{cluster_id}/statuses | List all adapter statuses for cluster
[**GetClusters**](DefaultAPI.md#GetClusters) | **Get** /api/hyperfleet/v1/clusters | List clusters
[**GetCompatibility**](DefaultAPI.md#GetCompatibility) | **Get** /api/hyperfleet/v1/compatibility | 
[**GetNodePoolById**](DefaultAPI.md#GetNodePoolById) | **Get** /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id} | Get nodepool by ID
[**GetNodePools**](DefaultAPI.md#GetNodePools) | **Get** /api/hyperfleet/v1/nodepools | List all nodepools for cluster
[**GetNodePoolsByClusterId**](DefaultAPI.md#GetNodePoolsByClusterId) | **Get** /api/hyperfleet/v1/clusters/{cluster_id}/nodepools | List all nodepools for cluster
[**GetNodePoolsStatuses**](DefaultAPI.md#GetNodePoolsStatuses) | **Get** /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses | List all adapter statuses for nodepools
[**PostCluster**](DefaultAPI.md#PostCluster) | **Post** /api/hyperfleet/v1/clusters | Create cluster
[**PostClusterStatuses**](DefaultAPI.md#PostClusterStatuses) | **Post** /api/hyperfleet/v1/clusters/{cluster_id}/statuses | Create adapter status
[**PostNodePoolStatuses**](DefaultAPI.md#PostNodePoolStatuses) | **Post** /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses | Create adapter status



## CreateNodePool

> NodePoolCreateResponse CreateNodePool(ctx, clusterId).NodePoolCreateRequest(nodePoolCreateRequest).Execute()

Create nodepool



### Example

```go
package main

import (
	"context"
	"fmt"
	"os"
	openapiclient "github.com/GIT_USER_ID/GIT_REPO_ID"
)

func main() {
	clusterId := "clusterId_example" // string | Cluster ID
	nodePoolCreateRequest := *openapiclient.NewNodePoolCreateRequest("Name_example", map[string]interface{}(123)) // NodePoolCreateRequest | 

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.DefaultAPI.CreateNodePool(context.Background(), clusterId).NodePoolCreateRequest(nodePoolCreateRequest).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `DefaultAPI.CreateNodePool``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `CreateNodePool`: NodePoolCreateResponse
	fmt.Fprintf(os.Stdout, "Response from `DefaultAPI.CreateNodePool`: %v\n", resp)
}
```

### Path Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
**clusterId** | **string** | Cluster ID | 

### Other Parameters

Other parameters are passed through a pointer to a apiCreateNodePoolRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------

 **nodePoolCreateRequest** | [**NodePoolCreateRequest**](NodePoolCreateRequest.md) |  | 

### Return type

[**NodePoolCreateResponse**](NodePoolCreateResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: application/json
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## GetClusterById

> Cluster GetClusterById(ctx, clusterId).Search(search).Execute()

Get cluster by ID

### Example

```go
package main

import (
	"context"
	"fmt"
	"os"
	openapiclient "github.com/GIT_USER_ID/GIT_REPO_ID"
)

func main() {
	clusterId := "clusterId_example" // string | 
	search := "search_example" // string |  (optional)

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.DefaultAPI.GetClusterById(context.Background(), clusterId).Search(search).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `DefaultAPI.GetClusterById``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `GetClusterById`: Cluster
	fmt.Fprintf(os.Stdout, "Response from `DefaultAPI.GetClusterById`: %v\n", resp)
}
```

### Path Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
**clusterId** | **string** |  | 

### Other Parameters

Other parameters are passed through a pointer to a apiGetClusterByIdRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------

 **search** | **string** |  | 

### Return type

[**Cluster**](Cluster.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## GetClusterStatuses

> AdapterStatusList GetClusterStatuses(ctx, clusterId).Page(page).PageSize(pageSize).OrderBy(orderBy).Order(order).Search(search).Execute()

List all adapter statuses for cluster



### Example

```go
package main

import (
	"context"
	"fmt"
	"os"
	openapiclient "github.com/GIT_USER_ID/GIT_REPO_ID"
)

func main() {
	clusterId := "clusterId_example" // string | Cluster ID
	page := int32(56) // int32 |  (optional) (default to 1)
	pageSize := int32(56) // int32 |  (optional) (default to 20)
	orderBy := "orderBy_example" // string |  (optional) (default to "created_at")
	order := openapiclient.OrderDirection("asc") // OrderDirection |  (optional)
	search := "search_example" // string |  (optional)

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.DefaultAPI.GetClusterStatuses(context.Background(), clusterId).Page(page).PageSize(pageSize).OrderBy(orderBy).Order(order).Search(search).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `DefaultAPI.GetClusterStatuses``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `GetClusterStatuses`: AdapterStatusList
	fmt.Fprintf(os.Stdout, "Response from `DefaultAPI.GetClusterStatuses`: %v\n", resp)
}
```

### Path Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
**clusterId** | **string** | Cluster ID | 

### Other Parameters

Other parameters are passed through a pointer to a apiGetClusterStatusesRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------

 **page** | **int32** |  | [default to 1]
 **pageSize** | **int32** |  | [default to 20]
 **orderBy** | **string** |  | [default to &quot;created_at&quot;]
 **order** | [**OrderDirection**](OrderDirection.md) |  | 
 **search** | **string** |  | 

### Return type

[**AdapterStatusList**](AdapterStatusList.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## GetClusters

> ClusterList GetClusters(ctx).Page(page).PageSize(pageSize).OrderBy(orderBy).Order(order).Search(search).Execute()

List clusters

### Example

```go
package main

import (
	"context"
	"fmt"
	"os"
	openapiclient "github.com/GIT_USER_ID/GIT_REPO_ID"
)

func main() {
	page := int32(56) // int32 |  (optional) (default to 1)
	pageSize := int32(56) // int32 |  (optional) (default to 20)
	orderBy := "orderBy_example" // string |  (optional) (default to "created_at")
	order := openapiclient.OrderDirection("asc") // OrderDirection |  (optional)
	search := "search_example" // string |  (optional)

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.DefaultAPI.GetClusters(context.Background()).Page(page).PageSize(pageSize).OrderBy(orderBy).Order(order).Search(search).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `DefaultAPI.GetClusters``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `GetClusters`: ClusterList
	fmt.Fprintf(os.Stdout, "Response from `DefaultAPI.GetClusters`: %v\n", resp)
}
```

### Path Parameters



### Other Parameters

Other parameters are passed through a pointer to a apiGetClustersRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **page** | **int32** |  | [default to 1]
 **pageSize** | **int32** |  | [default to 20]
 **orderBy** | **string** |  | [default to &quot;created_at&quot;]
 **order** | [**OrderDirection**](OrderDirection.md) |  | 
 **search** | **string** |  | 

### Return type

[**ClusterList**](ClusterList.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## GetCompatibility

> string GetCompatibility(ctx).Page(page).PageSize(pageSize).OrderBy(orderBy).Order(order).Search(search).Execute()





### Example

```go
package main

import (
	"context"
	"fmt"
	"os"
	openapiclient "github.com/GIT_USER_ID/GIT_REPO_ID"
)

func main() {
	page := int32(56) // int32 |  (optional) (default to 1)
	pageSize := int32(56) // int32 |  (optional) (default to 20)
	orderBy := "orderBy_example" // string |  (optional) (default to "created_at")
	order := openapiclient.OrderDirection("asc") // OrderDirection |  (optional)
	search := "search_example" // string |  (optional)

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.DefaultAPI.GetCompatibility(context.Background()).Page(page).PageSize(pageSize).OrderBy(orderBy).Order(order).Search(search).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `DefaultAPI.GetCompatibility``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `GetCompatibility`: string
	fmt.Fprintf(os.Stdout, "Response from `DefaultAPI.GetCompatibility`: %v\n", resp)
}
```

### Path Parameters



### Other Parameters

Other parameters are passed through a pointer to a apiGetCompatibilityRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **page** | **int32** |  | [default to 1]
 **pageSize** | **int32** |  | [default to 20]
 **orderBy** | **string** |  | [default to &quot;created_at&quot;]
 **order** | [**OrderDirection**](OrderDirection.md) |  | 
 **search** | **string** |  | 

### Return type

**string**

### Authorization

[BearerAuth](../README.md#BearerAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: text/plain

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## GetNodePoolById

> NodePool GetNodePoolById(ctx, clusterId, nodepoolId).Execute()

Get nodepool by ID



### Example

```go
package main

import (
	"context"
	"fmt"
	"os"
	openapiclient "github.com/GIT_USER_ID/GIT_REPO_ID"
)

func main() {
	clusterId := "clusterId_example" // string | Cluster ID
	nodepoolId := "nodepoolId_example" // string | NodePool ID

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.DefaultAPI.GetNodePoolById(context.Background(), clusterId, nodepoolId).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `DefaultAPI.GetNodePoolById``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `GetNodePoolById`: NodePool
	fmt.Fprintf(os.Stdout, "Response from `DefaultAPI.GetNodePoolById`: %v\n", resp)
}
```

### Path Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
**clusterId** | **string** | Cluster ID | 
**nodepoolId** | **string** | NodePool ID | 

### Other Parameters

Other parameters are passed through a pointer to a apiGetNodePoolByIdRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------



### Return type

[**NodePool**](NodePool.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## GetNodePools

> NodePoolList GetNodePools(ctx).Page(page).PageSize(pageSize).OrderBy(orderBy).Order(order).Search(search).Execute()

List all nodepools for cluster



### Example

```go
package main

import (
	"context"
	"fmt"
	"os"
	openapiclient "github.com/GIT_USER_ID/GIT_REPO_ID"
)

func main() {
	page := int32(56) // int32 |  (optional) (default to 1)
	pageSize := int32(56) // int32 |  (optional) (default to 20)
	orderBy := "orderBy_example" // string |  (optional) (default to "created_at")
	order := openapiclient.OrderDirection("asc") // OrderDirection |  (optional)
	search := "search_example" // string |  (optional)

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.DefaultAPI.GetNodePools(context.Background()).Page(page).PageSize(pageSize).OrderBy(orderBy).Order(order).Search(search).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `DefaultAPI.GetNodePools``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `GetNodePools`: NodePoolList
	fmt.Fprintf(os.Stdout, "Response from `DefaultAPI.GetNodePools`: %v\n", resp)
}
```

### Path Parameters



### Other Parameters

Other parameters are passed through a pointer to a apiGetNodePoolsRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **page** | **int32** |  | [default to 1]
 **pageSize** | **int32** |  | [default to 20]
 **orderBy** | **string** |  | [default to &quot;created_at&quot;]
 **order** | [**OrderDirection**](OrderDirection.md) |  | 
 **search** | **string** |  | 

### Return type

[**NodePoolList**](NodePoolList.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## GetNodePoolsByClusterId

> NodePoolList GetNodePoolsByClusterId(ctx, clusterId).Page(page).PageSize(pageSize).OrderBy(orderBy).Order(order).Search(search).Execute()

List all nodepools for cluster



### Example

```go
package main

import (
	"context"
	"fmt"
	"os"
	openapiclient "github.com/GIT_USER_ID/GIT_REPO_ID"
)

func main() {
	clusterId := "clusterId_example" // string | Cluster ID
	page := int32(56) // int32 |  (optional) (default to 1)
	pageSize := int32(56) // int32 |  (optional) (default to 20)
	orderBy := "orderBy_example" // string |  (optional) (default to "created_at")
	order := openapiclient.OrderDirection("asc") // OrderDirection |  (optional)
	search := "search_example" // string |  (optional)

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.DefaultAPI.GetNodePoolsByClusterId(context.Background(), clusterId).Page(page).PageSize(pageSize).OrderBy(orderBy).Order(order).Search(search).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `DefaultAPI.GetNodePoolsByClusterId``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `GetNodePoolsByClusterId`: NodePoolList
	fmt.Fprintf(os.Stdout, "Response from `DefaultAPI.GetNodePoolsByClusterId`: %v\n", resp)
}
```

### Path Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
**clusterId** | **string** | Cluster ID | 

### Other Parameters

Other parameters are passed through a pointer to a apiGetNodePoolsByClusterIdRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------

 **page** | **int32** |  | [default to 1]
 **pageSize** | **int32** |  | [default to 20]
 **orderBy** | **string** |  | [default to &quot;created_at&quot;]
 **order** | [**OrderDirection**](OrderDirection.md) |  | 
 **search** | **string** |  | 

### Return type

[**NodePoolList**](NodePoolList.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## GetNodePoolsStatuses

> AdapterStatusList GetNodePoolsStatuses(ctx, clusterId, nodepoolId).Page(page).PageSize(pageSize).OrderBy(orderBy).Order(order).Search(search).Execute()

List all adapter statuses for nodepools



### Example

```go
package main

import (
	"context"
	"fmt"
	"os"
	openapiclient "github.com/GIT_USER_ID/GIT_REPO_ID"
)

func main() {
	clusterId := "clusterId_example" // string | Cluster ID
	nodepoolId := "nodepoolId_example" // string | 
	page := int32(56) // int32 |  (optional) (default to 1)
	pageSize := int32(56) // int32 |  (optional) (default to 20)
	orderBy := "orderBy_example" // string |  (optional) (default to "created_at")
	order := openapiclient.OrderDirection("asc") // OrderDirection |  (optional)
	search := "search_example" // string |  (optional)

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.DefaultAPI.GetNodePoolsStatuses(context.Background(), clusterId, nodepoolId).Page(page).PageSize(pageSize).OrderBy(orderBy).Order(order).Search(search).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `DefaultAPI.GetNodePoolsStatuses``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `GetNodePoolsStatuses`: AdapterStatusList
	fmt.Fprintf(os.Stdout, "Response from `DefaultAPI.GetNodePoolsStatuses`: %v\n", resp)
}
```

### Path Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
**clusterId** | **string** | Cluster ID | 
**nodepoolId** | **string** |  | 

### Other Parameters

Other parameters are passed through a pointer to a apiGetNodePoolsStatusesRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------


 **page** | **int32** |  | [default to 1]
 **pageSize** | **int32** |  | [default to 20]
 **orderBy** | **string** |  | [default to &quot;created_at&quot;]
 **order** | [**OrderDirection**](OrderDirection.md) |  | 
 **search** | **string** |  | 

### Return type

[**AdapterStatusList**](AdapterStatusList.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## PostCluster

> Cluster PostCluster(ctx).ClusterCreateRequest(clusterCreateRequest).Execute()

Create cluster



### Example

```go
package main

import (
	"context"
	"fmt"
	"os"
	openapiclient "github.com/GIT_USER_ID/GIT_REPO_ID"
)

func main() {
	clusterCreateRequest := *openapiclient.NewClusterCreateRequest("Kind_example", "Name_example", map[string]interface{}(123)) // ClusterCreateRequest | 

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.DefaultAPI.PostCluster(context.Background()).ClusterCreateRequest(clusterCreateRequest).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `DefaultAPI.PostCluster``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `PostCluster`: Cluster
	fmt.Fprintf(os.Stdout, "Response from `DefaultAPI.PostCluster`: %v\n", resp)
}
```

### Path Parameters



### Other Parameters

Other parameters are passed through a pointer to a apiPostClusterRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **clusterCreateRequest** | [**ClusterCreateRequest**](ClusterCreateRequest.md) |  | 

### Return type

[**Cluster**](Cluster.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: application/json
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## PostClusterStatuses

> AdapterStatus PostClusterStatuses(ctx, clusterId).AdapterStatusCreateRequest(adapterStatusCreateRequest).Execute()

Create adapter status



### Example

```go
package main

import (
	"context"
	"fmt"
	"os"
    "time"
	openapiclient "github.com/GIT_USER_ID/GIT_REPO_ID"
)

func main() {
	clusterId := "clusterId_example" // string | Cluster ID
	adapterStatusCreateRequest := *openapiclient.NewAdapterStatusCreateRequest("Adapter_example", int32(123), []openapiclient.Condition{*openapiclient.NewCondition("Adapter_example", "Type_example", "Status_example", int32(123), time.Now(), time.Now())}) // AdapterStatusCreateRequest | 

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.DefaultAPI.PostClusterStatuses(context.Background(), clusterId).AdapterStatusCreateRequest(adapterStatusCreateRequest).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `DefaultAPI.PostClusterStatuses``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `PostClusterStatuses`: AdapterStatus
	fmt.Fprintf(os.Stdout, "Response from `DefaultAPI.PostClusterStatuses`: %v\n", resp)
}
```

### Path Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
**clusterId** | **string** | Cluster ID | 

### Other Parameters

Other parameters are passed through a pointer to a apiPostClusterStatusesRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------

 **adapterStatusCreateRequest** | [**AdapterStatusCreateRequest**](AdapterStatusCreateRequest.md) |  | 

### Return type

[**AdapterStatus**](AdapterStatus.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: application/json
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## PostNodePoolStatuses

> AdapterStatus PostNodePoolStatuses(ctx, clusterId, nodepoolId).AdapterStatusCreateRequest(adapterStatusCreateRequest).Execute()

Create adapter status



### Example

```go
package main

import (
	"context"
	"fmt"
	"os"
    "time"
	openapiclient "github.com/GIT_USER_ID/GIT_REPO_ID"
)

func main() {
	clusterId := "clusterId_example" // string | Cluster ID
	nodepoolId := "nodepoolId_example" // string | 
	adapterStatusCreateRequest := *openapiclient.NewAdapterStatusCreateRequest("Adapter_example", int32(123), []openapiclient.Condition{*openapiclient.NewCondition("Adapter_example", "Type_example", "Status_example", int32(123), time.Now(), time.Now())}) // AdapterStatusCreateRequest | 

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.DefaultAPI.PostNodePoolStatuses(context.Background(), clusterId, nodepoolId).AdapterStatusCreateRequest(adapterStatusCreateRequest).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `DefaultAPI.PostNodePoolStatuses``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `PostNodePoolStatuses`: AdapterStatus
	fmt.Fprintf(os.Stdout, "Response from `DefaultAPI.PostNodePoolStatuses`: %v\n", resp)
}
```

### Path Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
**clusterId** | **string** | Cluster ID | 
**nodepoolId** | **string** |  | 

### Other Parameters

Other parameters are passed through a pointer to a apiPostNodePoolStatusesRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------


 **adapterStatusCreateRequest** | [**AdapterStatusCreateRequest**](AdapterStatusCreateRequest.md) |  | 

### Return type

[**AdapterStatus**](AdapterStatus.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: application/json
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)

