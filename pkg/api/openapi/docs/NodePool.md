# NodePool

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**CreatedAt** | **time.Time** |  | 
**UpdatedAt** | **time.Time** |  | 
**CreatedBy** | **string** |  | 
**UpdatedBy** | **string** |  | 
**OwnerReferences** | [**ObjectReference**](ObjectReference.md) |  | 
**Status** | [**NodePoolStatus**](NodePoolStatus.md) |  | 
**Labels** | Pointer to **map[string]string** | labels for the API resource as pairs of name:value strings | [optional] 
**Id** | Pointer to **string** | Resource identifier | [optional] 
**Kind** | Pointer to **string** | Resource kind | [optional] 
**Href** | Pointer to **string** | Resource URI | [optional] 
**Name** | **string** | NodePool name (unique in a cluster) | 
**Spec** | **map[string]interface{}** | Cluster specification CLM doesn&#39;t know how to unmarshall the spec, it only stores and forwards to adapters to do their job But CLM will validate the schema before accepting the request | 

## Methods

### NewNodePool

`func NewNodePool(createdAt time.Time, updatedAt time.Time, createdBy string, updatedBy string, ownerReferences ObjectReference, status NodePoolStatus, name string, spec map[string]interface{}, ) *NodePool`

NewNodePool instantiates a new NodePool object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewNodePoolWithDefaults

`func NewNodePoolWithDefaults() *NodePool`

NewNodePoolWithDefaults instantiates a new NodePool object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetCreatedAt

`func (o *NodePool) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *NodePool) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *NodePool) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.


### GetUpdatedAt

`func (o *NodePool) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *NodePool) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *NodePool) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.


### GetCreatedBy

`func (o *NodePool) GetCreatedBy() string`

GetCreatedBy returns the CreatedBy field if non-nil, zero value otherwise.

### GetCreatedByOk

`func (o *NodePool) GetCreatedByOk() (*string, bool)`

GetCreatedByOk returns a tuple with the CreatedBy field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedBy

`func (o *NodePool) SetCreatedBy(v string)`

SetCreatedBy sets CreatedBy field to given value.


### GetUpdatedBy

`func (o *NodePool) GetUpdatedBy() string`

GetUpdatedBy returns the UpdatedBy field if non-nil, zero value otherwise.

### GetUpdatedByOk

`func (o *NodePool) GetUpdatedByOk() (*string, bool)`

GetUpdatedByOk returns a tuple with the UpdatedBy field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedBy

`func (o *NodePool) SetUpdatedBy(v string)`

SetUpdatedBy sets UpdatedBy field to given value.


### GetOwnerReferences

`func (o *NodePool) GetOwnerReferences() ObjectReference`

GetOwnerReferences returns the OwnerReferences field if non-nil, zero value otherwise.

### GetOwnerReferencesOk

`func (o *NodePool) GetOwnerReferencesOk() (*ObjectReference, bool)`

GetOwnerReferencesOk returns a tuple with the OwnerReferences field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetOwnerReferences

`func (o *NodePool) SetOwnerReferences(v ObjectReference)`

SetOwnerReferences sets OwnerReferences field to given value.


### GetStatus

`func (o *NodePool) GetStatus() NodePoolStatus`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *NodePool) GetStatusOk() (*NodePoolStatus, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *NodePool) SetStatus(v NodePoolStatus)`

SetStatus sets Status field to given value.


### GetLabels

`func (o *NodePool) GetLabels() map[string]string`

GetLabels returns the Labels field if non-nil, zero value otherwise.

### GetLabelsOk

`func (o *NodePool) GetLabelsOk() (*map[string]string, bool)`

GetLabelsOk returns a tuple with the Labels field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetLabels

`func (o *NodePool) SetLabels(v map[string]string)`

SetLabels sets Labels field to given value.

### HasLabels

`func (o *NodePool) HasLabels() bool`

HasLabels returns a boolean if a field has been set.

### GetId

`func (o *NodePool) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *NodePool) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *NodePool) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *NodePool) HasId() bool`

HasId returns a boolean if a field has been set.

### GetKind

`func (o *NodePool) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *NodePool) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *NodePool) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *NodePool) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetHref

`func (o *NodePool) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *NodePool) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *NodePool) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *NodePool) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetName

`func (o *NodePool) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *NodePool) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *NodePool) SetName(v string)`

SetName sets Name field to given value.


### GetSpec

`func (o *NodePool) GetSpec() map[string]interface{}`

GetSpec returns the Spec field if non-nil, zero value otherwise.

### GetSpecOk

`func (o *NodePool) GetSpecOk() (*map[string]interface{}, bool)`

GetSpecOk returns a tuple with the Spec field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSpec

`func (o *NodePool) SetSpec(v map[string]interface{})`

SetSpec sets Spec field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


