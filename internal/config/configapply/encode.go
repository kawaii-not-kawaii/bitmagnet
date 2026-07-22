package configapply

import (
	"reflect"
	"time"

	"github.com/iancoleman/strcase"
)

var durationType = reflect.TypeOf(time.Duration(0))

// RedactedPlaceholder is the value the config API substitutes for sensitive
// fields on read (see gqlmodel.RedactedValuePlaceholder, which aliases this).
// SetSection treats it as "keep the existing value".
const RedactedPlaceholder = "***REDACTED***"

// preserveRedacted returns raw with every leaf equal to RedactedPlaceholder
// replaced by the corresponding value from current (the encoded existing
// section). Keys match exactly or via snake_case, mirroring the section
// decoder's MatchName.
func preserveRedacted(raw, current any) any {
	if s, ok := raw.(string); ok && s == RedactedPlaceholder {
		return current
	}

	rawMap, rawOk := raw.(map[string]any)

	currentMap, currentOk := current.(map[string]any)
	if !rawOk || !currentOk {
		return raw
	}

	out := make(map[string]any, len(rawMap))

	for k, v := range rawMap {
		currentValue, ok := currentMap[k]
		if !ok {
			currentValue, ok = currentMap[strcase.ToSnake(k)]
		}

		if ok {
			out[k] = preserveRedacted(v, currentValue)
		} else {
			out[k] = v
		}
	}

	return out
}

func encodeSection(v any) any {
	return encodeValue(reflect.ValueOf(v))
}

func encodeValue(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}

	for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}

		v = v.Elem()
	}

	if v.Type() == durationType {
		return time.Duration(v.Int()).String()
	}

	switch v.Kind() {
	case reflect.Struct:
		encoded := make(map[string]any, v.NumField())

		for i := range v.NumField() {
			field := v.Type().Field(i)
			if field.PkgPath != "" {
				continue
			}

			encoded[strcase.ToSnake(field.Name)] = encodeValue(v.Field(i))
		}

		return encoded
	case reflect.Map:
		if v.IsNil() {
			return nil
		}

		if v.Type().Key().Kind() == reflect.String {
			encoded := make(map[string]any, v.Len())

			iterator := v.MapRange()
			for iterator.Next() {
				encoded[iterator.Key().String()] = encodeValue(iterator.Value())
			}

			return encoded
		}

		encoded := make(map[any]any, v.Len())

		iterator := v.MapRange()
		for iterator.Next() {
			encoded[iterator.Key().Interface()] = encodeValue(iterator.Value())
		}

		return encoded
	case reflect.Array, reflect.Slice:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return nil
		}

		encoded := make([]any, v.Len())

		for i := range v.Len() {
			encoded[i] = encodeValue(v.Index(i))
		}

		return encoded
	default:
		return v.Interface()
	}
}
