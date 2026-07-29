package filters

import (
	"fmt"

	"github.com/itchyny/gojq"
)

func ApplyJQFilter(payload any, jqFilter string) (any, error) {
	query, err := gojq.Parse(jqFilter)
	if err != nil {
		return nil, fmt.Errorf("invalid jq_filter: %v", err)
	}

	iter := query.Run(payload)
	var results []any
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			return nil, fmt.Errorf("invalid jq_filter: %v", err)
		}
		results = append(results, v)
	}

	if len(results) == 1 {
		return results[0], nil
	}
	return results, nil
}
