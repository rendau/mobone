package tools

import (
	"reflect"
	"testing"
)

func TestConstructSortColumns(t *testing.T) {
	tests := []struct {
		name           string
		allowedFields  map[string]string
		inputSort      []string
		expectedResult []string
	}{
		{
			name:           "EmptyAllowedFields",
			allowedFields:  nil,
			inputSort:      []string{"-name"},
			expectedResult: nil,
		},
		{
			name:           "EmptyInputSort",
			allowedFields:  map[string]string{"name": "user_name"},
			inputSort:      nil,
			expectedResult: nil,
		},
		{
			name:           "EmptyAllowedFieldsAndInput",
			allowedFields:  nil,
			inputSort:      nil,
			expectedResult: nil,
		},
		{
			name:           "ValidFieldAscending",
			allowedFields:  map[string]string{"name": "user_name"},
			inputSort:      []string{"name"},
			expectedResult: []string{"user_name"},
		},
		{
			name:           "ValidFieldDescending",
			allowedFields:  map[string]string{"name": "user_name"},
			inputSort:      []string{"-name"},
			expectedResult: []string{"user_name desc"},
		},
		{
			name:           "InvalidField",
			allowedFields:  map[string]string{"name": "user_name"},
			inputSort:      []string{"age"},
			expectedResult: nil,
		},
		{
			name:           "MixedFieldsValidAndInvalid",
			allowedFields:  map[string]string{"name": "user_name", "age": "user_age"},
			inputSort:      []string{"-name", "height"},
			expectedResult: []string{"user_name desc"},
		},
		{
			name:           "MultipleValidFields",
			allowedFields:  map[string]string{"name": "user_name", "age": "user_age"},
			inputSort:      []string{"name", "-age"},
			expectedResult: []string{"user_name", "user_age desc"},
		},
		{
			name:           "ValidFieldWithEmptyMapping",
			allowedFields:  map[string]string{"name": "user_name", "status": ""},
			inputSort:      []string{"status", "name"},
			expectedResult: []string{"user_name"},
		},
		{
			name:           "NoMatch",
			allowedFields:  map[string]string{"name": "user_name"},
			inputSort:      []string{"-unknown"},
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConstructSortColumns(tt.allowedFields, tt.inputSort)
			if !reflect.DeepEqual(result, tt.expectedResult) {
				t.Errorf("expected %v, got %v", tt.expectedResult, result)
			}
		})
	}
}
