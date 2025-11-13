# AdapterStatusCreateRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Adapter** | **string** | Adapter identifier | 
**ObservedGeneration** | **int32** | Which cluster generation this status reflects | 
**Conditions** | [**[]Condition**](Condition.md) |  | 
**Data** | Pointer to **map[string]interface{}** | Adapter-specific data | [optional] 
**Metadata** | Pointer to **map[string]interface{}** | Job execution metadata | [optional] 

## Methods

### NewAdapterStatusCreateRequest

`func NewAdapterStatusCreateRequest(adapter string, observedGeneration int32, conditions []Condition, ) *AdapterStatusCreateRequest`

NewAdapterStatusCreateRequest instantiates a new AdapterStatusCreateRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewAdapterStatusCreateRequestWithDefaults

`func NewAdapterStatusCreateRequestWithDefaults() *AdapterStatusCreateRequest`

NewAdapterStatusCreateRequestWithDefaults instantiates a new AdapterStatusCreateRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetAdapter

`func (o *AdapterStatusCreateRequest) GetAdapter() string`

GetAdapter returns the Adapter field if non-nil, zero value otherwise.

### GetAdapterOk

`func (o *AdapterStatusCreateRequest) GetAdapterOk() (*string, bool)`

GetAdapterOk returns a tuple with the Adapter field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAdapter

`func (o *AdapterStatusCreateRequest) SetAdapter(v string)`

SetAdapter sets Adapter field to given value.


### GetObservedGeneration

`func (o *AdapterStatusCreateRequest) GetObservedGeneration() int32`

GetObservedGeneration returns the ObservedGeneration field if non-nil, zero value otherwise.

### GetObservedGenerationOk

`func (o *AdapterStatusCreateRequest) GetObservedGenerationOk() (*int32, bool)`

GetObservedGenerationOk returns a tuple with the ObservedGeneration field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetObservedGeneration

`func (o *AdapterStatusCreateRequest) SetObservedGeneration(v int32)`

SetObservedGeneration sets ObservedGeneration field to given value.


### GetConditions

`func (o *AdapterStatusCreateRequest) GetConditions() []Condition`

GetConditions returns the Conditions field if non-nil, zero value otherwise.

### GetConditionsOk

`func (o *AdapterStatusCreateRequest) GetConditionsOk() (*[]Condition, bool)`

GetConditionsOk returns a tuple with the Conditions field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetConditions

`func (o *AdapterStatusCreateRequest) SetConditions(v []Condition)`

SetConditions sets Conditions field to given value.


### GetData

`func (o *AdapterStatusCreateRequest) GetData() map[string]interface{}`

GetData returns the Data field if non-nil, zero value otherwise.

### GetDataOk

`func (o *AdapterStatusCreateRequest) GetDataOk() (*map[string]interface{}, bool)`

GetDataOk returns a tuple with the Data field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetData

`func (o *AdapterStatusCreateRequest) SetData(v map[string]interface{})`

SetData sets Data field to given value.

### HasData

`func (o *AdapterStatusCreateRequest) HasData() bool`

HasData returns a boolean if a field has been set.

### GetMetadata

`func (o *AdapterStatusCreateRequest) GetMetadata() map[string]interface{}`

GetMetadata returns the Metadata field if non-nil, zero value otherwise.

### GetMetadataOk

`func (o *AdapterStatusCreateRequest) GetMetadataOk() (*map[string]interface{}, bool)`

GetMetadataOk returns a tuple with the Metadata field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMetadata

`func (o *AdapterStatusCreateRequest) SetMetadata(v map[string]interface{})`

SetMetadata sets Metadata field to given value.

### HasMetadata

`func (o *AdapterStatusCreateRequest) HasMetadata() bool`

HasMetadata returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


