package fasthttp

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

// Codec unmarshals request bodies for a given content type.
type Codec interface {
	// ContentType returns the MIME type this codec handles (e.g. "application/json").
	// Prefix matching is used so "application/json" matches "application/json; charset=utf-8".
	ContentType() string

	// Unmarshal decodes data into v.
	Unmarshal(data []byte, v any) error
}

// ContentTypeAwareCodec is an optional extension that gives the codec access
// to the full Content-Type header value, including parameters (e.g. boundary
// for multipart/form-data).
type ContentTypeAwareCodec interface {
	Codec
	UnmarshalWithContentType(data []byte, contentType string, v any) error
}

var (
	codecs     map[string]Codec
	codecOrder []string // longest prefix first
)

// RegisterCodec registers a codec. Built-in codecs are registered in init().
func RegisterCodec(c Codec) {
	if codecs == nil {
		codecs = make(map[string]Codec)
	}
	ct := c.ContentType()
	if _, exists := codecs[ct]; !exists {
		codecOrder = append(codecOrder, ct)
	}
	codecs[ct] = c
}

func matchCodec(contentType string) Codec {
	if codecs == nil || contentType == "" {
		return nil
	}
	// longest prefix wins
	for _, ct := range codecOrder {
		if strings.HasPrefix(contentType, ct) {
			return codecs[ct]
		}
	}
	return nil
}

// DecodeForm parses a URL-encoded form string (or query string) into a map.
// Supports bracket notation for nesting (user[name]=John) and arrays (items[]=a).
func DecodeForm(data []byte, v any) error {
	var fc formCodec
	return fc.Unmarshal(data, v)
}

// ── JSON ─────────────────────────────────────────────────────────────────────

type jsonCodec struct{}

func (jsonCodec) ContentType() string { return "application/json" }

func (jsonCodec) Unmarshal(data []byte, v any) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, v)
}

// ── XML ──────────────────────────────────────────────────────────────────────

type xmlCodec struct{}

func (xmlCodec) ContentType() string { return "application/xml" }

func (xmlCodec) Unmarshal(data []byte, v any) error {
	if len(data) == 0 {
		return nil
	}
	return xml.Unmarshal(data, v)
}

// ── Text / plain ─────────────────────────────────────────────────────────────

type textCodec struct{}

func (textCodec) ContentType() string { return "text/plain" }

func (c textCodec) Unmarshal(data []byte, v any) error {
	if len(data) == 0 {
		return nil
	}
	switch dst := v.(type) {
	case *string:
		*dst = string(data)
		return nil
	case *[]byte:
		*dst = data
		return nil
	case *any:
		*dst = string(data)
		return nil
	}
	return fmt.Errorf("text/plain: unsupported target type %T (expect *string, *[]byte, or *any)", v)
}

// ── Form (application/x-www-form-urlencoded) ─────────────────────────────────

type formCodec struct{}

func (formCodec) ContentType() string { return "application/x-www-form-urlencoded" }

func (f formCodec) Unmarshal(data []byte, v any) error {
	if len(data) == 0 {
		return nil
	}
	parsed, err := decodeForm(string(data))
	if err != nil {
		return err
	}
	switch dst := v.(type) {
	case *map[string]any:
		if *dst == nil {
			*dst = parsed
		} else {
			for k, vv := range parsed {
				(*dst)[k] = vv
			}
		}
		return nil
	case *any:
		*dst = parsed
		return nil
	}
	if err := decodeFormToStruct(parsed, v); err != nil {
		return err
	}
	return nil
}

func decodeFormToStruct(form map[string]any, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("form: target must be a non-nil pointer, got %T", v)
	}
	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		return fmt.Errorf("form: target must be a pointer to struct, got %T", v)
	}
	return populateStruct(elem, form)
}

func populateStruct(rv reflect.Value, form map[string]any) error {
	t := rv.Type()
	for i := 0; i < t.NumField(); i++ {
		ft := t.Field(i)
		if !ft.IsExported() {
			continue
		}
		fv := rv.Field(i)
		if !fv.CanSet() {
			continue
		}
		name := ft.Tag.Get("form")
		if name == "" {
			name = strings.ToLower(ft.Name)
		}
		raw, ok := form[name]
		if !ok {
			continue
		}
		if err := setField(fv, raw); err != nil {
			return fmt.Errorf("form: field %q: %v", ft.Name, err)
		}
	}
	return nil
}

func setField(fv reflect.Value, raw any) error {
	if fv.Kind() == reflect.Ptr {
		if fv.IsNil() {
			fv.Set(reflect.New(fv.Type().Elem()))
		}
		fv = fv.Elem()
	}
	switch fv.Kind() {
	case reflect.String:
		s, ok := raw.(string)
		if !ok {
			return fmt.Errorf("expected string, got %T", raw)
		}
		fv.SetString(s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		s, ok := raw.(string)
		if !ok {
			return fmt.Errorf("expected string, got %T", raw)
		}
		n, err := strconv.ParseInt(s, 10, fv.Type().Bits())
		if err != nil {
			return err
		}
		fv.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		s, ok := raw.(string)
		if !ok {
			return fmt.Errorf("expected string, got %T", raw)
		}
		n, err := strconv.ParseUint(s, 10, fv.Type().Bits())
		if err != nil {
			return err
		}
		fv.SetUint(n)
	case reflect.Float32, reflect.Float64:
		s, ok := raw.(string)
		if !ok {
			return fmt.Errorf("expected string, got %T", raw)
		}
		n, err := strconv.ParseFloat(s, fv.Type().Bits())
		if err != nil {
			return err
		}
		fv.SetFloat(n)
	case reflect.Bool:
		s, ok := raw.(string)
		if !ok {
			return fmt.Errorf("expected string, got %T", raw)
		}
		b, err := strconv.ParseBool(s)
		if err != nil {
			return err
		}
		fv.SetBool(b)
	case reflect.Struct:
		m, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("expected map for struct field, got %T", raw)
		}
		return populateStruct(fv, m)
	case reflect.Slice:
		return setSliceField(fv, raw)
	case reflect.Map:
		m, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("expected map for map field, got %T", raw)
		}
		if fv.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("map key must be string")
		}
		if fv.IsNil() {
			fv.Set(reflect.MakeMap(fv.Type()))
		}
		elemType := fv.Type().Elem()
		for k, v := range m {
			elem := reflect.New(elemType).Elem()
			if err := setField(elem, v); err != nil {
				return fmt.Errorf("map[%q]: %v", k, err)
			}
			fv.SetMapIndex(reflect.ValueOf(k), elem)
		}
	default:
		return fmt.Errorf("unsupported field type %s", fv.Type())
	}
	return nil
}

func setSliceField(fv reflect.Value, raw any) error {
	var vals []string
	switch v := raw.(type) {
	case string:
		vals = []string{v}
	case []string:
		vals = v
	case []any:
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return fmt.Errorf("expected string in slice, got %T", item)
			}
			vals = append(vals, s)
		}
	default:
		return fmt.Errorf("expected string or []string for slice field, got %T", raw)
	}
	elemType := fv.Type().Elem()
	for _, s := range vals {
		elem := reflect.New(elemType).Elem()
		if err := setField(elem, s); err != nil {
			return err
		}
		fv.Set(reflect.Append(fv, elem))
	}
	return nil
}

// ── Multipart (multipart/form-data) ──────────────────────────────────────────

type multipartCodec struct{}

func (multipartCodec) ContentType() string { return "multipart/form-data" }

func (c multipartCodec) Unmarshal(data []byte, v any) error {
	return fmt.Errorf("multipart: content-type boundary required; use BodyParser instead")
}

func (c multipartCodec) UnmarshalWithContentType(data []byte, ct string, v any) error {
	if len(data) == 0 {
		return nil
	}
	_, params, err := mime.ParseMediaType(ct)
	if err != nil {
		return fmt.Errorf("multipart: %v", err)
	}
	boundary := params["boundary"]
	if boundary == "" {
		return fmt.Errorf("multipart: no boundary in content type")
	}

	reader := multipart.NewReader(strings.NewReader(string(data)), boundary)
	form := make(map[string]any)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("multipart: %v", err)
		}
		name := part.FormName()
		if name == "" {
			part.Close()
			continue
		}
		valueBytes, err := io.ReadAll(part)
		part.Close()
		if err != nil {
			return fmt.Errorf("multipart: %v", err)
		}
		valueStr := string(valueBytes)
		if existing, ok := form[name]; ok {
			switch sl := existing.(type) {
			case []string:
				form[name] = append(sl, valueStr)
			default:
				form[name] = []string{existing.(string), valueStr}
			}
		} else {
			form[name] = valueStr
		}
	}

	switch dst := v.(type) {
	case *map[string]any:
		if *dst == nil {
			*dst = form
		} else {
			for k, vv := range form {
				(*dst)[k] = vv
			}
		}
		return nil
	case *any:
		*dst = form
		return nil
	}
	return fmt.Errorf("multipart: unsupported target type %T (expect *map[string]any or *any)", v)
}

// ── Form decoder (shared by formCodec and QueryParser) ────────────────────────

func decodeForm(raw string) (map[string]any, error) {
	vals := make(map[string]any)
	for _, pair := range strings.Split(raw, "&") {
		if pair == "" {
			continue
		}
		eq := strings.IndexByte(pair, '=')
		var key, val string
		if eq >= 0 {
			key = pair[:eq]
			val = pair[eq+1:]
		} else {
			key = pair
		}
		key, _ = url.PathUnescape(key)
		val, _ = url.QueryUnescape(val)

		if strings.ContainsAny(key, "[]") {
			insertNested(vals, parseBracketPath(key), val)
		} else {
			insertFlat(vals, key, val)
		}
	}
	return collapseArrays(vals), nil
}

func parseBracketPath(key string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(key); i++ {
		switch key[i] {
		case '[':
			if i > start {
				parts = append(parts, key[start:i])
			}
			start = i + 1
		case ']':
			if i > start {
				parts = append(parts, key[start:i])
			} else {
				parts = append(parts, "")
			}
			start = i + 1
		}
	}
	if start < len(key) {
		parts = append(parts, key[start:])
	}
	return parts
}

func insertNested(dest map[string]any, path []string, val string) {
	m := dest
	for i := 0; i < len(path)-1; i++ {
		k := path[i]
		if k == "" {
			k = strconv.Itoa(len(m))
		}
		next, ok := m[k]
		if !ok {
			n := make(map[string]any)
			m[k] = n
			m = n
		} else {
			m = next.(map[string]any)
		}
	}
	last := path[len(path)-1]
	if last == "" {
		last = strconv.Itoa(len(m))
	}
	m[last] = val
}

func insertFlat(dest map[string]any, key, val string) {
	if existing, ok := dest[key]; ok {
		switch sl := existing.(type) {
		case []string:
			dest[key] = append(sl, val)
		default:
			dest[key] = []string{existing.(string), val}
		}
	} else {
		dest[key] = val
	}
}

func collapseArrays(dest map[string]any) map[string]any {
	for k, v := range dest {
		if mv, ok := v.(map[string]any); ok {
			dest[k] = tryArray(collapseArrays(mv))
		}
	}
	return dest
}

func tryArray(m map[string]any) any {
	if len(m) == 0 {
		return m
	}
	var arr []any
	for i := 0; ; i++ {
		s := strconv.Itoa(i)
		v, ok := m[s]
		if !ok {
			break
		}
		arr = append(arr, v)
	}
	if len(arr) > 0 && len(arr) == len(m) {
		return arr
	}
	return m
}

func init() {
	RegisterCodec(jsonCodec{})
	RegisterCodec(xmlCodec{})
	RegisterCodec(formCodec{})
	RegisterCodec(multipartCodec{})
	RegisterCodec(textCodec{})
}
