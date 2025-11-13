# NodePoolCreateRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Labels** | Pointer to **map[string]string** | labels for the API resource as pairs of name:value strings | [optional] 
**Id** | Pointer to **string** | Resource identifier | [optional] 
**Kind** | Pointer to **string** | Resource kind | [optional] 
**Href** | Pointer to **string** | Resource URI | [optional] 
**Name** | **string** | NodePool name (unique in a cluster) | 
**Spec** | **map[string]interface{}** | Cluster specification CLM doesn&#39;t know how to unmarshall the spec, it only stores and forwards to adapters to do their job But CLM will validate the schema before accepting the request | 

## Methods

### NewNodePoolCreateRequest

`func NewNodePoolCreateRequest(name string, spec map[string]interface{}, ) *NodePoolCreateRequest`

NewNodePoolCreateRequest instantiates a new NodePoolCreateRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewNodePoolCreateRequestWithDefaults

`func NewNodePoolCreateRequestWithDefaults() *NodePoolCreateRequest`

NewNodePoolCreateRequestWithDefaults instantiates a new NodePoolCreateRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetLabels

`func (o *NodePoolCreateRequest) GetLabels() map[string]string`

GetLabels returns the Labels field if non-nil, zero value otherwise.

### GetLabelsOk

`func (o *NodePoolCreateRequest) GetLabelsOk() (*map[string]string, bool)`

GetLabelsOk returns a tuple with the Labels field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetLabels

`func (o *NodePoolCreateRequest) SetLabels(v map[string]string)`

SetLabels sets Labels field to given value.

### HasLabels

`func (o *NodePoolCreateRequest) HasLabels() bool`

HasLabels returns a boolean if a field has been set.

### GetId

`func (o *NodePoolCreateRequest) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *NodePoolCreateRequest) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *NodePoolCreateRequest) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *NodePoolCreateRequest) HasId() bool`

HasId returns a boolean if a field has been set.

### GetKind

`func (o *NodePoolCreateRequest) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *NodePoolCreateRequest) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *NodePoolCreateRequest) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *NodePoolCreateRequest) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetHref

`func (o *NodePoolCreateRequest) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *NodePoolCreateRequest) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *NodePoolCreateRequest) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *NodePoolCreateRequest) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetName

`func (o *NodePoolCreateRequest) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *NodePoolCreateRequest) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *NodePoolCreateRequest) SetName(v string)`

SetName sets Name field to given value.


### GetSpec

`func (o *NodePoolCreateRequest) GetSpec() map[string]interface{}`

GetSpec returns the Spec field if non-nil, zero value otherwise.

### GetSpecOk

`func (o *NodePoolCreateRequest) GetSpecOk() (*map[string]interface{}, bool)`

GetSpecOk returns a tuple with the Spec field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSpec

`func (o *NodePoolCreateRequest) SetSpec(v map[string]interface{})`

SetSpec sets Spec field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


