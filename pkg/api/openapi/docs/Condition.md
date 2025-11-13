# Condition

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Adapter** | **string** | Adapter name | 
**Type** | **string** |  | 
**Status** | **string** | Condition status | 
**Reason** | Pointer to **string** | Machine-readable reason code | [optional] 
**Message** | Pointer to **string** | Human-readable message | [optional] 
**ObservedGeneration** | **int32** |  | 
**CreatedAt** | **time.Time** | Time when the condition was created - In an adapter reporting conditions &#x60;updated_at&#x3D;&#x3D;created_at&#x60; - In the API, &#x60;created_at&#x60; doesn&#39;t change with new updates from adapters | 
**UpdatedAt** | **time.Time** | Time when the condition was updated | 
**LastTransitionTime** | Pointer to **time.Time** | When this condition last transitioned (computed by service when status changes) | [optional] 

## Methods

### NewCondition

`func NewCondition(adapter string, type_ string, status string, observedGeneration int32, createdAt time.Time, updatedAt time.Time, ) *Condition`

NewCondition instantiates a new Condition object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewConditionWithDefaults

`func NewConditionWithDefaults() *Condition`

NewConditionWithDefaults instantiates a new Condition object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetAdapter

`func (o *Condition) GetAdapter() string`

GetAdapter returns the Adapter field if non-nil, zero value otherwise.

### GetAdapterOk

`func (o *Condition) GetAdapterOk() (*string, bool)`

GetAdapterOk returns a tuple with the Adapter field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAdapter

`func (o *Condition) SetAdapter(v string)`

SetAdapter sets Adapter field to given value.


### GetType

`func (o *Condition) GetType() string`

GetType returns the Type field if non-nil, zero value otherwise.

### GetTypeOk

`func (o *Condition) GetTypeOk() (*string, bool)`

GetTypeOk returns a tuple with the Type field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetType

`func (o *Condition) SetType(v string)`

SetType sets Type field to given value.


### GetStatus

`func (o *Condition) GetStatus() string`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *Condition) GetStatusOk() (*string, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *Condition) SetStatus(v string)`

SetStatus sets Status field to given value.


### GetReason

`func (o *Condition) GetReason() string`

GetReason returns the Reason field if non-nil, zero value otherwise.

### GetReasonOk

`func (o *Condition) GetReasonOk() (*string, bool)`

GetReasonOk returns a tuple with the Reason field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetReason

`func (o *Condition) SetReason(v string)`

SetReason sets Reason field to given value.

### HasReason

`func (o *Condition) HasReason() bool`

HasReason returns a boolean if a field has been set.

### GetMessage

`func (o *Condition) GetMessage() string`

GetMessage returns the Message field if non-nil, zero value otherwise.

### GetMessageOk

`func (o *Condition) GetMessageOk() (*string, bool)`

GetMessageOk returns a tuple with the Message field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMessage

`func (o *Condition) SetMessage(v string)`

SetMessage sets Message field to given value.

### HasMessage

`func (o *Condition) HasMessage() bool`

HasMessage returns a boolean if a field has been set.

### GetObservedGeneration

`func (o *Condition) GetObservedGeneration() int32`

GetObservedGeneration returns the ObservedGeneration field if non-nil, zero value otherwise.

### GetObservedGenerationOk

`func (o *Condition) GetObservedGenerationOk() (*int32, bool)`

GetObservedGenerationOk returns a tuple with the ObservedGeneration field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetObservedGeneration

`func (o *Condition) SetObservedGeneration(v int32)`

SetObservedGeneration sets ObservedGeneration field to given value.


### GetCreatedAt

`func (o *Condition) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *Condition) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *Condition) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.


### GetUpdatedAt

`func (o *Condition) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *Condition) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *Condition) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.


### GetLastTransitionTime

`func (o *Condition) GetLastTransitionTime() time.Time`

GetLastTransitionTime returns the LastTransitionTime field if non-nil, zero value otherwise.

### GetLastTransitionTimeOk

`func (o *Condition) GetLastTransitionTimeOk() (*time.Time, bool)`

GetLastTransitionTimeOk returns a tuple with the LastTransitionTime field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetLastTransitionTime

`func (o *Condition) SetLastTransitionTime(v time.Time)`

SetLastTransitionTime sets LastTransitionTime field to given value.

### HasLastTransitionTime

`func (o *Condition) HasLastTransitionTime() bool`

HasLastTransitionTime returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


