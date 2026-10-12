package appmanifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ErrMalformed means the input is neither JSON nor YAML.
var ErrMalformed = errors.New("manifest is not valid JSON or YAML")

// Problem is one validation failure at a JSON pointer ("" is the document root).
type Problem struct {
	Pointer, Message string
}

// ValidationError lists everything wrong with a well-formed but invalid manifest.
type ValidationError struct{ Problems []Problem }

// Error implements error.
func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Problems))
	for _, p := range e.Problems {
		parts = append(parts, p.Pointer+": "+p.Message)
	}
	return "invalid manifest: " + strings.Join(parts, "; ")
}

// SupportedBotEvents are the bot_events LocalSlack emits.
func SupportedBotEvents() []string {
	return []string{"app_mention", "message.channels", "message.groups", "message.im", "message.mpim", "reaction_added"}
}

// Parsed is a validated manifest.
type Parsed struct {
	Manifest Manifest
	// Raw is the whole manifest as JSON, stored verbatim (unknown fields included).
	Raw json.RawMessage
	// Warnings are non-fatal: bot_events LocalSlack does not emit.
	Warnings []string
}

// Parse reads a JSON or YAML manifest and validates the fields LocalSlack honors.
// Errors are ErrMalformed or *ValidationError.
func Parse(data []byte) (Parsed, error) {
	raw, err := toJSON(data)
	if err != nil {
		return Parsed{}, err
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Parsed{}, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	root, ok := doc.(map[string]any)
	if !ok {
		return Parsed{}, &ValidationError{Problems: []Problem{{Message: "manifest must be an object"}}}
	}
	if problems := validate(root); len(problems) > 0 {
		return Parsed{}, &ValidationError{Problems: problems}
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return Parsed{}, fmt.Errorf("decode validated manifest: %w", err)
	}
	if m.Features.BotUser.DisplayName == "" {
		m.Features.BotUser.DisplayName = m.DisplayInformation.Name
	}
	return Parsed{Manifest: m, Raw: raw, Warnings: warnings(m.Settings.EventSubscriptions.BotEvents)}, nil
}

func toJSON(data []byte) (json.RawMessage, error) {
	if json.Valid(data) {
		var buf bytes.Buffer
		if err := json.Compact(&buf, data); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
		}
		return buf.Bytes(), nil
	}
	var doc any
	if err := yaml.Unmarshal(data, &doc); err != nil || doc == nil {
		return nil, ErrMalformed
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	return out, nil
}

func validate(root map[string]any) []Problem {
	var v validator
	if name, _ := get(root, "display_information", "name").(string); name == "" {
		v.add("/display_information/name", "required")
	}
	v.optString(root, "features", "bot_user", "display_name")
	if scopes := get(root, "oauth_config", "scopes", "bot"); scopes != nil {
		v.stringList(scopes, "/oauth_config/scopes/bot", "must be an array of strings")
	}
	if sm := get(root, "settings", "socket_mode_enabled"); sm != nil {
		if _, ok := sm.(bool); !ok {
			v.add("/settings/socket_mode_enabled", "must be a boolean")
		}
	}
	v.requestURL(get(root, "settings", "event_subscriptions", "request_url"))
	if ev := get(root, "settings", "event_subscriptions", "bot_events"); ev != nil {
		v.stringList(ev, "/settings/event_subscriptions/bot_events", "must be an array of strings")
	}
	return v.problems
}

type validator struct{ problems []Problem }

func (v *validator) add(ptr, msg string) { v.problems = append(v.problems, Problem{ptr, msg}) }

func (v *validator) optString(root map[string]any, path ...string) {
	if x := get(root, path...); x != nil {
		if _, ok := x.(string); !ok {
			v.add("/"+strings.Join(path, "/"), "must be a string")
		}
	}
}

func (v *validator) stringList(x any, ptr, typeMsg string) {
	list, ok := x.([]any)
	if !ok {
		v.add(ptr, typeMsg)
		return
	}
	for i, e := range list {
		s, ok := e.(string)
		switch {
		case !ok:
			v.add(ptr+"/"+strconv.Itoa(i), "must be a string")
		case s == "":
			v.add(ptr+"/"+strconv.Itoa(i), "must not be empty")
		}
	}
}

func (v *validator) requestURL(x any) {
	const ptr = "/settings/event_subscriptions/request_url"
	if x == nil {
		return
	}
	s, ok := x.(string)
	if !ok {
		v.add(ptr, "must be a string")
		return
	}
	if s == "" {
		return
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		v.add(ptr, "must be an http(s) URL")
	}
}

// get walks nested objects; nil if any step is missing or not an object.
func get(m map[string]any, path ...string) any {
	var cur any = m
	for _, k := range path {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = obj[k]
	}
	return cur
}

func warnings(events []string) []string {
	supported := make(map[string]bool)
	for _, e := range SupportedBotEvents() {
		supported[e] = true
	}
	var out []string
	for i, e := range events {
		if !supported[e] {
			out = append(out, fmt.Sprintf("/settings/event_subscriptions/bot_events/%d: event %q is not emitted by LocalSlack", i, e))
		}
	}
	return out
}
