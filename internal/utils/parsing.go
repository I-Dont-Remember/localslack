package utils

import (
	"encoding/json"
	"fmt"
		"net/http"


	"localslack/internal/core"
)

type Params map[string][]string

func (p Params) Get(k string) string {
	if v := p[k]; len(v) > 0 {
		return v[0]
	}
	return ""
}
func (p Params) GetAll(k string) []string { return p[k] }

func (p Params) Require(k string) (string, *core.SlackError) {
	v := p.Get(k)
	if v == "" {
		return "", &core.SlackError{
			OK:      false,
			Error:   "missing_param",
			Message: fmt.Sprintf("missing required parameter: %s", k),
		}
	}
	return v, nil
}

type Request struct {
	Params Params
	Raw    map[string]json.RawMessage
}

func (r Request) JSONInto(key string, dst any) bool {
	b, ok := r.Raw[key]
	if !ok || len(b) == 0 {
		return false
	}
	return json.Unmarshal(b, dst) == nil
}

// Used to make important utilities available to all handlers without having complicated function signatures
type Deps interface {
    // Store() store.WorkspaceStore
    // Events() events.Dispatcher
    // Rate(), Chaos(), Stats() if needed
}

func ParseRequest(r *http.Request) (Request, error) {
	return Request{}, nil
}