package main

import (
	"encoding/json"
	"strconv"
)

// Some fields need to always be represented in array format.
var alwaysArray = map[string]bool{
	"accounts":         true,
	"keys":             true,
	"owners":           true,
	"signers":          true,
	"filter_tags":      true,
	"select_authors":   true,
	"select_tags":      true,
	"signatures":       true,
	"required_owner":   true,
	"required_active":  true,
	"required_posting": true,
	"required_other":   true,
}

// Converts a map of string slices to a map of interface{}. Pulls out individual elements to be flattened.
func Flatten(m map[string][]string) map[string]interface{} {
	o := make(map[string]interface{})
	for k, v := range m {
		if len(v) == 1 {
			i1, err := strconv.Atoi(v[0])
			if err == nil {
				o[k] = i1
			} else if alwaysArray[k] {
				o[k] = v
			} else {
				o[k] = v[0]
			}
		} else {
			o[k] = v
		}
	}
	return o
}

// JSON is fun sometimes. This function will try to convert a "numberish thing" to an int64.
// JSON can represent numbers as strings, ints, or json.Number.
func MaybeGetInt64(numberish interface{}) (int64, bool) {
	var err error
	number := int64(0)
	jsonNum, ok := numberish.(json.Number)
	if ok {
		number, err = jsonNum.Int64()
		if err != nil {
			return number, false
		}
	} else {
		stringNum, ok := numberish.(string)
		if !ok {
			intNum, ok := numberish.(int64)
			if !ok {
				return number, false
			}
			number = int64(intNum)
		} else {
			number, err = strconv.ParseInt(stringNum, 10, 64)
			if err != nil {
				return number, false
			}
		}
	}
	return number, true
}
