package utils

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const maxSafeIntegerFloat64 = 1<<53 - 1

func Uint64ToUint(v uint64) (uint, error) {
	if v > uint64(^uint(0)) {
		return 0, fmt.Errorf("uint overflow")
	}
	return uint(v), nil
}

func Int64ToUint(v int64) (uint, error) {
	if v < 0 {
		return 0, fmt.Errorf("negative uint value")
	}
	return Uint64ToUint(uint64(v))
}

func Float64ToUint(v float64) (uint, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || math.Trunc(v) != v || v > maxSafeIntegerFloat64 {
		return 0, fmt.Errorf("invalid uint value")
	}
	if v > float64(^uint(0)) {
		return 0, fmt.Errorf("uint overflow")
	}
	return uint(v), nil
}

func ParseUint(value string) (uint, error) {
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0, err
	}
	return Uint64ToUint(parsed)
}

func ParsePositiveUint(value string) (uint, error) {
	parsed, err := ParseUint(value)
	if err != nil {
		return 0, err
	}
	if parsed == 0 {
		return 0, fmt.Errorf("zero uint value")
	}
	return parsed, nil
}

func UintFromAny(value interface{}) (uint, error) {
	switch typed := value.(type) {
	case uint:
		return typed, nil
	case uint64:
		return Uint64ToUint(typed)
	case uint32:
		return uint(typed), nil
	case uint16:
		return uint(typed), nil
	case uint8:
		return uint(typed), nil
	case int:
		return Int64ToUint(int64(typed))
	case int64:
		return Int64ToUint(typed)
	case int32:
		return Int64ToUint(int64(typed))
	case int16:
		return Int64ToUint(int64(typed))
	case int8:
		return Int64ToUint(int64(typed))
	case float64:
		return Float64ToUint(typed)
	case string:
		return ParseUint(typed)
	default:
		return 0, fmt.Errorf("unsupported uint type %T", value)
	}
}

func PositiveUintFromAny(value interface{}) (uint, error) {
	parsed, err := UintFromAny(value)
	if err != nil {
		return 0, err
	}
	if parsed == 0 {
		return 0, fmt.Errorf("zero uint value")
	}
	return parsed, nil
}
