package mcache

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Codec handles serialization and deserialization of cache values.
type Codec interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

// RawCodec passes []byte and string through without transformation.
type RawCodec struct{}

// Marshal returns v as-is when v is []byte or string.
func (RawCodec) Marshal(v any) ([]byte, error) {
	switch val := v.(type) {
	case []byte:
		return val, nil
	case string:
		return []byte(val), nil
	default:
		return nil, fmt.Errorf("raw codec: unsupported type %T (expected []byte or string)", v)
	}
}

// Unmarshal copies data into *[]byte or *string.
func (RawCodec) Unmarshal(data []byte, v any) error {
	switch ptr := v.(type) {
	case *[]byte:
		*ptr = data
		return nil
	case *string:
		*ptr = string(data)
		return nil
	default:
		return fmt.Errorf("raw codec: unsupported target type %T (expected *[]byte or *string)", v)
	}
}

// JSONCodec uses encoding/json for all values.
type JSONCodec struct{}

// Marshal serializes v to JSON.
func (JSONCodec) Marshal(v any) ([]byte, error) {
	if v == nil {
		return nil, ErrValueNil
	}
	return json.Marshal(v)
}

// Unmarshal parses JSON data into v.
func (JSONCodec) Unmarshal(data []byte, v any) error {
	if v == nil {
		return errors.New("unmarshal target cannot be nil")
	}
	return json.Unmarshal(data, v)
}
