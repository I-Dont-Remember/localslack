package scenario

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"go.yaml.in/yaml/v3"
)

// Issue is one problem with a fixture, at a JSON pointer ("" is the document root).
type Issue struct {
	Pointer string `json:"pointer"`
	Message string `json:"message"`
}

// Error lists everything wrong with a well-formed but invalid fixture.
type Error struct{ Issues []Issue }

// Error implements error.
func (e *Error) Error() string {
	parts := make([]string, 0, len(e.Issues))
	for _, i := range e.Issues {
		parts = append(parts, i.Pointer+": "+i.Message)
	}
	return "invalid scenario: " + strings.Join(parts, "; ")
}

// ErrSyntax means the input is not parseable JSON, TOML or YAML (or not an object).
var ErrSyntax = errors.New("scenario is not a valid JSON, TOML or YAML object")

// Format is a fixture file syntax.
type Format string

// Supported formats. JSON is canonical; TOML and YAML are loaders only.
const (
	JSON Format = "json"
	TOML Format = "toml"
	YAML Format = "yaml"
)

// FormatOf picks the format from a file name; "" if the extension is unknown.
func FormatOf(name string) Format {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".json":
		return JSON
	case ".toml":
		return TOML
	case ".yaml", ".yml":
		return YAML
	}
	return ""
}

// LoadFile reads and parses a fixture file, choosing the format by extension.
func LoadFile(path string) (Fixture, error) {
	f := FormatOf(path)
	if f == "" {
		return Fixture{}, fmt.Errorf("scenario file %q: extension must be .json, .toml, .yaml or .yml", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Fixture{}, fmt.Errorf("read scenario: %w", err)
	}
	fx, err := Parse(data, f)
	if err != nil {
		return Fixture{}, fmt.Errorf("scenario file %q: %w", path, err)
	}
	return fx, nil
}

// Parse decodes a fixture. A missing version means 1. Unknown or computed
// fields (reply_count, latest_reply) are reported with JSON pointers as *Error;
// bad syntax is ErrSyntax. Semantic checks are Validate's job.
func Parse(data []byte, f Format) (Fixture, error) {
	raw, err := toJSON(data, f)
	if err != nil {
		return Fixture{}, err
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Fixture{}, fmt.Errorf("%w: %w", ErrSyntax, err)
	}
	if _, ok := doc.(map[string]any); !ok {
		return Fixture{}, ErrSyntax
	}
	if issues := unknownKeys(doc, reflect.TypeFor[Fixture](), ""); len(issues) > 0 {
		return Fixture{}, &Error{Issues: issues}
	}
	var fx Fixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		return Fixture{}, &Error{Issues: []Issue{typeIssue(err)}}
	}
	if fx.Version == 0 {
		fx.Version = Version
	}
	return fx, nil
}

func toJSON(data []byte, f Format) ([]byte, error) {
	var doc any
	switch f {
	case JSON:
		return data, nil
	case TOML:
		if err := toml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrSyntax, err)
		}
	case YAML:
		if err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&doc); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrSyntax, err)
		}
	default:
		return nil, fmt.Errorf("unknown scenario format %q", f)
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSyntax, err)
	}
	return out, nil
}

func typeIssue(err error) Issue {
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		return Issue{Pointer: "/" + strings.ReplaceAll(te.Field, ".", "/"), Message: fmt.Sprintf("want %s, got %s", te.Type, te.Value)}
	}
	return Issue{Message: err.Error()}
}

// computedFields are derived on populate and must not be supplied.
func computedFields() map[string]bool {
	return map[string]bool{"reply_count": true, "latest_reply": true, "replies": true, "reply_users": true}
}

// unknownKeys walks the decoded document against t's json tags.
func unknownKeys(doc any, t reflect.Type, ptr string) []Issue {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch v := doc.(type) {
	case []any:
		if t.Kind() != reflect.Slice {
			return nil
		}
		var out []Issue
		for i, e := range v {
			out = append(out, unknownKeys(e, t.Elem(), fmt.Sprintf("%s/%d", ptr, i))...)
		}
		return out
	case map[string]any:
		if t.Kind() != reflect.Struct {
			return nil
		}
		return structKeys(v, t, ptr)
	}
	return nil
}

func structKeys(v map[string]any, t reflect.Type, ptr string) []Issue {
	fields := map[string]reflect.Type{}
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		fields[name] = t.Field(i).Type
	}
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	computed := computedFields()
	var out []Issue
	for _, k := range keys {
		ft, ok := fields[k]
		switch {
		case computed[k]:
			out = append(out, Issue{ptr + "/" + k, k + " is computed from the thread; remove it"})
		case !ok:
			out = append(out, Issue{ptr + "/" + k, "unknown field " + k})
		default:
			out = append(out, unknownKeys(v[k], ft, ptr+"/"+k)...)
		}
	}
	return out
}
