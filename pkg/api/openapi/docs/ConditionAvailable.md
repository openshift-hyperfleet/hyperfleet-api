# ConditionAvailable

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Type** | **string** |  | 
**Adapter** | **string** | Adapter name | 
**Status** | **string** | Condition status | 
**Reason** | Pointer to **string** | Machine-readable reason code | [optional] 
**Message** | Pointer to **string** | Human-readable message | [optional] 
**ObservedGeneration** | **int32** |  | 
**CreatedAt** | **time.Time** | Time when the condition was created - In an adapter reporting conditions &#x60;updated_at&#x3D;&#x3D;created_at&#x60; - In the API, &#x60;created_at&#x60; doesn&#39;t change with new updates from adapters | 
**UpdatedAt** | **time.Time** | Time when the condition was updated | 

## Methods

### NewConditionAvailable

`func NewConditionAvailable(type_ string, adapter string, status string, observedGeneration int32, createdAt time.Time, updatedAt time.Time, ) *ConditionAvailable`

NewConditionAvailable instantiates a new ConditionAvailable object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewConditionAvailableWithDefaults

`func NewConditionAvailableWithDefaults() *ConditionAvailable`

NewConditionAvailableWithDefaults instantiates a new ConditionAvailable object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetType

`func (o *ConditionAvailable) GetType() string`

GetType returns the Type field if non-nil, zero value otherwise.

### GetTypeOk

`func (o *ConditionAvailable) GetTypeOk() (*string, bool)`

GetTypeOk returns a tuple with the Type field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetType

`func (o *ConditionAvailable) SetType(v string)`

SetType sets Type field to given value.


### GetAdapter

`func (o *ConditionAvailable) GetAdapter() string`

GetAdapter returns the Adapter field if non-nil, zero value otherwise.

### GetAdapterOk

`func (o *ConditionAvailable) GetAdapterOk() (*string, bool)`

GetAdapterOk returns a tuple with the Adapter field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAdapter

`func (o *ConditionAvailable) SetAdapter(v string)`

SetAdapter sets Adapter field to given value.


### GetStatus

`func (o *ConditionAvailable) GetStatus() string`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *ConditionAvailable) GetStatusOk() (*string, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *ConditionAvailable) SetStatus(v string)`

SetStatus sets Status field to given value.


### GetReason

`func (o *ConditionAvailable) GetReason() string`

GetReason returns the Reason field if non-nil, zero value otherwise.

### GetReasonOk

`func (o *ConditionAvailable) GetReasonOk() (*string, bool)`

GetReasonOk returns a tuple with the Reason field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetReason

`func (o *ConditionAvailable) SetReason(v string)`

SetReason sets Reason field to given value.

### HasReason

`func (o *ConditionAvailable) HasReason() bool`

HasReason returns a boolean if a field has been set.

### GetMessage

`func (o *ConditionAvailable) GetMessage() string`

GetMessage returns the Message field if non-nil, zero value otherwise.

### GetMessageOk

`func (o *ConditionAvailable) GetMessageOk() (*string, bool)`

GetMessageOk returns a tuple with the Message field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMessage

`func (o *ConditionAvailable) SetMessage(v string)`

SetMessage sets Message field to given value.

### HasMessage

`func (o *ConditionAvailable) HasMessage() bool`

HasMessage returns a boolean if a field has been set.

### GetObservedGeneration

`func (o *ConditionAvailable) GetObservedGeneration() int32`

GetObservedGeneration returns the ObservedGeneration field if non-nil, zero value otherwise.

### GetObservedGenerationOk

`func (o *ConditionAvailable) GetObservedGenerationOk() (*int32, bool)`

GetObservedGenerationOk returns a tuple with the ObservedGeneration field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetObservedGeneration

`func (o *ConditionAvailable) SetObservedGeneration(v int32)`

SetObservedGeneration sets ObservedGeneration field to given value.


### GetCreatedAt

`func (o *ConditionAvailable) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *ConditionAvailable) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *ConditionAvailable) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.


### GetUpdatedAt

`func (o *ConditionAvailable) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *ConditionAvailable) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *ConditionAvailable) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


