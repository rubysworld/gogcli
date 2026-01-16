package cmd

import (
	"fmt"
	"reflect"
	"strings"

	"google.golang.org/api/sheets/v4"
)

func applyForceSendFields(format *sheets.CellFormat, fieldMask string) error {
	if format == nil {
		return fmt.Errorf("format is required")
	}

	for _, raw := range splitFieldMask(fieldMask) {
		normalized := normalizeFormatField(raw)
		if normalized == "" {
			continue
		}
		if err := forceSendJSONField(format, normalized); err != nil {
			return fmt.Errorf("invalid format field %q: %w", strings.TrimSpace(raw), err)
		}
	}
	return nil
}

func splitFieldMask(mask string) []string {
	if strings.TrimSpace(mask) == "" {
		return nil
	}
	parts := strings.Split(mask, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func normalizeFormatField(field string) string {
	field = strings.TrimSpace(field)
	if field == "" {
		return ""
	}
	if field == "userEnteredFormat" {
		return ""
	}
	if strings.HasPrefix(field, "userEnteredFormat.") {
		return strings.TrimPrefix(field, "userEnteredFormat.")
	}
	return ""
}

func forceSendJSONField(root any, jsonPath string) error {
	current := reflect.ValueOf(root)
	if current.Kind() != reflect.Pointer || current.IsNil() {
		return fmt.Errorf("format must be a non-nil pointer")
	}

	parts := strings.Split(jsonPath, ".")
	for i, part := range parts {
		if current.Kind() == reflect.Pointer {
			if current.IsNil() {
				if current.Type().Elem().Kind() != reflect.Struct {
					return fmt.Errorf("field %q is not a struct", part)
				}
				current.Set(reflect.New(current.Type().Elem()))
			}
			current = current.Elem()
		}
		if current.Kind() != reflect.Struct {
			return fmt.Errorf("field %q is not a struct", part)
		}

		fieldValue, fieldName, ok := findJSONField(current, part)
		if !ok {
			return fmt.Errorf("unknown field %q", part)
		}

		if i == len(parts)-1 {
			if fieldValue.Kind() == reflect.Pointer && fieldValue.IsNil() && fieldValue.Type().Elem().Kind() == reflect.Struct {
				fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
			}
			return addForceSendField(current, fieldName)
		}

		switch fieldValue.Kind() {
		case reflect.Pointer:
			if fieldValue.IsNil() {
				if fieldValue.Type().Elem().Kind() != reflect.Struct {
					return fmt.Errorf("field %q is not a struct", part)
				}
				fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
			}
			current = fieldValue
		case reflect.Struct:
			if !fieldValue.CanAddr() {
				return fmt.Errorf("field %q is not addressable", part)
			}
			current = fieldValue.Addr()
		default:
			return fmt.Errorf("field %q is not a struct", part)
		}
	}

	return nil
}

func findJSONField(v reflect.Value, jsonName string) (reflect.Value, string, bool) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "" {
			continue
		}
		if name == jsonName {
			return v.Field(i), field.Name, true
		}
	}
	return reflect.Value{}, "", false
}

func addForceSendField(v reflect.Value, fieldName string) error {
	fs := v.FieldByName("ForceSendFields")
	if !fs.IsValid() {
		return fmt.Errorf("missing ForceSendFields")
	}
	if fs.Kind() != reflect.Slice || fs.Type().Elem().Kind() != reflect.String {
		return fmt.Errorf("invalid ForceSendFields")
	}
	for i := 0; i < fs.Len(); i++ {
		if fs.Index(i).String() == fieldName {
			return nil
		}
	}
	fs.Set(reflect.Append(fs, reflect.ValueOf(fieldName)))
	return nil
}
