package token

import (
	"encoding/json"
	"reflect"
)

// AudienceList is a slice of strings that can be deserialized from either a single string value or a list of strings.
type AudienceList []string

func (s *AudienceList) UnmarshalJSON(data []byte) (err error) {
	var value interface{}

	if err = json.Unmarshal(data, &value); err != nil {
		return err
	}

	switch v := value.(type) {
	case string:
		*s = []string{v}

	case []string:
		*s = v

	case []interface{}:
		var ss []string

		for _, vv := range v {
			vs, ok := vv.(string)
			if !ok {
				return &json.UnsupportedTypeError{
					Type: reflect.TypeOf(vv),
				}
			}

			ss = append(ss, vs)
		}

		*s = ss

	case nil:
		return nil

	default:
		return &json.UnsupportedTypeError{
			Type: reflect.TypeOf(v),
		}
	}

	return
}

func (s AudienceList) MarshalJSON() (b []byte, err error) {
	return json.Marshal([]string(s))
}
