package tools

import (
	"strings"
)

func ConstructSortColumns(allowedFields map[string]string, inputSort []string) []string {
	if allowedFields == nil || len(allowedFields) == 0 {
		return nil
	}
	if inputSort == nil || len(inputSort) == 0 {
		return nil
	}

	result := make([]string, 0, len(inputSort))
	var isDesc bool
	var expr string
	var ok bool

	for _, inputV := range inputSort {
		isDesc = strings.HasPrefix(inputV, "-")
		inputV = strings.TrimLeft(inputV, "-")

		if expr, ok = allowedFields[inputV]; ok {
			if expr != "" {
				if isDesc {
					result = append(result, expr+" desc")
				} else {
					result = append(result, expr)
				}
			}
		}
	}

	if len(result) == 0 {
		return nil
	}

	return result
}
