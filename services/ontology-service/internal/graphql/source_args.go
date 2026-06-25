package graphql

import (
	"fmt"
	"strings"
)

func optionalSourceInstanceArgument(value *string, name string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, fmt.Errorf("%s cannot be blank", name)
	}
	return &trimmed, nil
}
