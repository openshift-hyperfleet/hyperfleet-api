# AdapterStatus

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Adapter** | **string** | Name of the adapter that generated this status (Validator, DNS...) | 
**ObservedGeneration** | **int32** | Which generation for an entity (Clusters, NodePools) was current at the time of creating this status | 
**Conditions** | [**[]Condition**](Condition.md) | Kubernetes-style conditions tracking adapter state | 
**Data** | Pointer to **map[string]interface{}** | Adapter-specific data (structure varies by adapter type) | [optional] 
**Metadata** | Pointer to [**AdapterStatusMetadata**](AdapterStatusMetadata.md) |  | [optional] 

## Methods

### NewAdapterStatus

`func NewAdapterStatus(adapter string, observedGeneration int32, conditions []Condition, ) *AdapterStatus`

NewAdapterStatus instantiates a new AdapterStatus object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewAdapterStatusWithDefaults

`func NewAdapterStatusWithDefaults() *AdapterStatus`

NewAdapterStatusWithDefaults instantiates a new AdapterStatus object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetAdapter

`func (o *AdapterStatus) GetAdapter() string`

GetAdapter returns the Adapter field if non-nil, zero value otherwise.

### GetAdapterOk

`func (o *AdapterStatus) GetAdapterOk() (*string, bool)`

GetAdapterOk returns a tuple with the Adapter field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAdapter

`func (o *AdapterStatus) SetAdapter(v string)`

SetAdapter sets Adapter field to given value.


### GetObservedGeneration

`func (o *AdapterStatus) GetObservedGeneration() int32`

GetObservedGeneration returns the ObservedGeneration field if non-nil, zero value otherwise.

### GetObservedGenerationOk

`func (o *AdapterStatus) GetObservedGenerationOk() (*int32, bool)`

GetObservedGenerationOk returns a tuple with the ObservedGeneration field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetObservedGeneration

`func (o *AdapterStatus) SetObservedGeneration(v int32)`

SetObservedGeneration sets ObservedGeneration field to given value.


### GetConditions

`func (o *AdapterStatus) GetConditions() []Condition`

GetConditions returns the Conditions field if non-nil, zero value otherwise.

### GetConditionsOk

`func (o *AdapterStatus) GetConditionsOk() (*[]Condition, bool)`

GetConditionsOk returns a tuple with the Conditions field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetConditions

`func (o *AdapterStatus) SetConditions(v []Condition)`

SetConditions sets Conditions field to given value.


### GetData

`func (o *AdapterStatus) GetData() map[string]interface{}`

GetData returns the Data field if non-nil, zero value otherwise.

### GetDataOk

`func (o *AdapterStatus) GetDataOk() (*map[string]interface{}, bool)`

GetDataOk returns a tuple with the Data field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetData

`func (o *AdapterStatus) SetData(v map[string]interface{})`

SetData sets Data field to given value.

### HasData

`func (o *AdapterStatus) HasData() bool`

HasData returns a boolean if a field has been set.

### GetMetadata

`func (o *AdapterStatus) GetMetadata() AdapterStatusMetadata`

GetMetadata returns the Metadata field if non-nil, zero value otherwise.

### GetMetadataOk

`func (o *AdapterStatus) GetMetadataOk() (*AdapterStatusMetadata, bool)`

GetMetadataOk returns a tuple with the Metadata field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMetadata

`func (o *AdapterStatus) SetMetadata(v AdapterStatusMetadata)`

SetMetadata sets Metadata field to given value.

### HasMetadata

`func (o *AdapterStatus) HasMetadata() bool`

HasMetadata returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


