package api

import "fmt"

const (
	MaxLabelKeyLen   = 255
	MaxLabelValueLen = 255
)

type ResourceLabel struct {
	ResourceID string `json:"-" gorm:"primaryKey;size:255;not null"`
	Key        string `json:"key" gorm:"primaryKey;size:255;not null"`
	Value      string `json:"value" gorm:"size:255;not null"`
}

func (ResourceLabel) TableName() string {
	return "resource_labels"
}

func ValidateLabel(key, value string) error {
	if key == "" {
		return fmt.Errorf("label key cannot be empty")
	}
	if len(key) > MaxLabelKeyLen {
		return fmt.Errorf("label key %q exceeds maximum length of %d", key, MaxLabelKeyLen)
	}
	if len(value) > MaxLabelValueLen {
		return fmt.Errorf("label value for key %q exceeds maximum length of %d", key, MaxLabelValueLen)
	}
	return nil
}
