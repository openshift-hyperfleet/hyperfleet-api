# ClusterStatus

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Phase** | **string** | Current cluster phase (native database column). Updated when adapters report status. Note: status.phase provides aggregated view from all adapter conditions. | 
**LastTransitionTime** | **time.Time** | When cluster last transitioned (updated by adapters, used by Sentinel for backoff) Updated when adapters report status if the phase changes | 
**ObservedGeneration** | **int32** | Last generation processed by adapters Updated when adapters report status. This will be the lowest value of each adapter&#39;s observed_generation values The phase value is based on this generation | 
**UpdatedAt** | **time.Time** | Time of the last complete report from adapters Updated when adapters report status. Oldest &#x60;updated_at&#x60; from the adapter conditions | 
**Adapters** | [**[]ConditionAvailable**](ConditionAvailable.md) |  | 

## Methods

### NewClusterStatus

`func NewClusterStatus(phase string, lastTransitionTime time.Time, observedGeneration int32, updatedAt time.Time, adapters []ConditionAvailable, ) *ClusterStatus`

NewClusterStatus instantiates a new ClusterStatus object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewClusterStatusWithDefaults

`func NewClusterStatusWithDefaults() *ClusterStatus`

NewClusterStatusWithDefaults instantiates a new ClusterStatus object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetPhase

`func (o *ClusterStatus) GetPhase() string`

GetPhase returns the Phase field if non-nil, zero value otherwise.

### GetPhaseOk

`func (o *ClusterStatus) GetPhaseOk() (*string, bool)`

GetPhaseOk returns a tuple with the Phase field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPhase

`func (o *ClusterStatus) SetPhase(v string)`

SetPhase sets Phase field to given value.


### GetLastTransitionTime

`func (o *ClusterStatus) GetLastTransitionTime() time.Time`

GetLastTransitionTime returns the LastTransitionTime field if non-nil, zero value otherwise.

### GetLastTransitionTimeOk

`func (o *ClusterStatus) GetLastTransitionTimeOk() (*time.Time, bool)`

GetLastTransitionTimeOk returns a tuple with the LastTransitionTime field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetLastTransitionTime

`func (o *ClusterStatus) SetLastTransitionTime(v time.Time)`

SetLastTransitionTime sets LastTransitionTime field to given value.


### GetObservedGeneration

`func (o *ClusterStatus) GetObservedGeneration() int32`

GetObservedGeneration returns the ObservedGeneration field if non-nil, zero value otherwise.

### GetObservedGenerationOk

`func (o *ClusterStatus) GetObservedGenerationOk() (*int32, bool)`

GetObservedGenerationOk returns a tuple with the ObservedGeneration field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetObservedGeneration

`func (o *ClusterStatus) SetObservedGeneration(v int32)`

SetObservedGeneration sets ObservedGeneration field to given value.


### GetUpdatedAt

`func (o *ClusterStatus) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *ClusterStatus) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *ClusterStatus) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.


### GetAdapters

`func (o *ClusterStatus) GetAdapters() []ConditionAvailable`

GetAdapters returns the Adapters field if non-nil, zero value otherwise.

### GetAdaptersOk

`func (o *ClusterStatus) GetAdaptersOk() (*[]ConditionAvailable, bool)`

GetAdaptersOk returns a tuple with the Adapters field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAdapters

`func (o *ClusterStatus) SetAdapters(v []ConditionAvailable)`

SetAdapters sets Adapters field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


