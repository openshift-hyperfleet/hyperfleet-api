# APIResource

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Labels** | Pointer to **map[string]string** | labels for the API resource as pairs of name:value strings | [optional] 
**Id** | Pointer to **string** | Resource identifier | [optional] 
**Kind** | Pointer to **string** | Resource kind | [optional] 
**Href** | Pointer to **string** | Resource URI | [optional] 

## Methods

### NewAPIResource

`func NewAPIResource() *APIResource`

NewAPIResource instantiates a new APIResource object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewAPIResourceWithDefaults

`func NewAPIResourceWithDefaults() *APIResource`

NewAPIResourceWithDefaults instantiates a new APIResource object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetLabels

`func (o *APIResource) GetLabels() map[string]string`

GetLabels returns the Labels field if non-nil, zero value otherwise.

### GetLabelsOk

`func (o *APIResource) GetLabelsOk() (*map[string]string, bool)`

GetLabelsOk returns a tuple with the Labels field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetLabels

`func (o *APIResource) SetLabels(v map[string]string)`

SetLabels sets Labels field to given value.

### HasLabels

`func (o *APIResource) HasLabels() bool`

HasLabels returns a boolean if a field has been set.

### GetId

`func (o *APIResource) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *APIResource) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *APIResource) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *APIResource) HasId() bool`

HasId returns a boolean if a field has been set.

### GetKind

`func (o *APIResource) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *APIResource) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *APIResource) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *APIResource) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetHref

`func (o *APIResource) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *APIResource) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *APIResource) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *APIResource) HasHref() bool`

HasHref returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


