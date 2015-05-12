package util

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"reflect"
	"strconv"
	"time"
)

//ConvertToQueryValues converts the struct to url.Values
func ConvertToQueryValues(ifc interface{}) url.Values {
	values := url.Values{}
	SetQueryValues(ifc, &values)
	return values
}

//SetQueryValues sets the struct to existing url.Values following ECS encoding rules
func SetQueryValues(ifc interface{}, values *url.Values) {
	setQueryValues(ifc, values, "")
}

func setQueryValues(i interface{}, values *url.Values, prefix string) {
	elem := reflect.ValueOf(i)
	if elem.Kind() == reflect.Ptr {
		elem = elem.Elem()
	}
	elemType := elem.Type()
	for i := 0; i < elem.NumField(); i++ {
		fieldName := elemType.Field(i).Name
		field := elem.Field(i)
		// TODO Use Tag for validation
		// tag := typ.Field(i).Tag.Get("tagname")
		kind := field.Kind()
		if (kind == reflect.Ptr || kind == reflect.Array || kind == reflect.Slice || kind == reflect.Map || kind == reflect.Chan) && field.IsNil() {
			continue
		}
		if kind == reflect.Ptr {
			field = field.Elem()
		}
		var value string
		switch field.Interface().(type) {
		case int, int8, int16, int32, int64:
			i := field.Int()
			if i != 0 {
				value = strconv.FormatInt(i, 10)
			}
		case uint, uint8, uint16, uint32, uint64:
			i := field.Uint()
			if i != 0 {
				value = strconv.FormatUint(i, 10)
			}
		case float32:
			value = strconv.FormatFloat(field.Float(), 'f', 4, 32)
		case float64:
			value = strconv.FormatFloat(field.Float(), 'f', 4, 64)
		case []byte:
			value = string(field.Bytes())
		case bool:
			value = strconv.FormatBool(field.Bool())
		case string:
			value = field.String()
		case []string:
			l := field.Len()
			if l > 0 {
				strArray := make([]string, l)
				for i := 0; i < l; i++ {
					strArray[i] = field.Index(i).String()
				}
				bytes, err := json.Marshal(strArray)
				if err == nil {
					value = string(bytes)
				} else {
					log.Printf("Failed to convert JSON: %v", err)
				}
			}
		case time.Time:
			t := field.Interface().(time.Time)
			value = GetISO8601TimeStamp(t)

		default:
			if kind == reflect.Slice { //Array of structs
				l := field.Len()
				for j := 0; j < l; j++ {
					prefixName := fmt.Sprintf("%s.%d.", fieldName, (j + 1))
					ifc := field.Index(j).Interface()
					log.Printf("%s : %v", prefixName, ifc)
					if ifc != nil {
						setQueryValues(ifc, values, prefixName)
					}
				}
			} else {
				ifc := field.Interface()
				if ifc != nil {
					SetQueryValues(ifc, values)
					continue
				}
			}
		}
		if value != "" {
			name := elemType.Field(i).Tag.Get("ArgName")
			if name == "" {
				name = fieldName
			}
			if prefix != "" {
				name = prefix + name
			}
			values.Set(name, value)
		}
	}
}
