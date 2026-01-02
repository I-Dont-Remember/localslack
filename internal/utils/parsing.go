package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"localslack/constants"
)

type Params map[string][]string

func (p Params) Get(k string) string {
	if v := p[k]; len(v) > 0 {
		return v[0]
	}
	return ""
}

func (p Params) GetOrDefault(k string, def string) string {
	if v := p[k]; len(v) > 0 {
		return v[0]
	}
	return def
}

func (p Params) GetAll(k string) []string { return p[k] }

func (p Params) Require(k string) (string, *SlackError) {
	v := p.Get(k)
	if v == "" {
		return "", &SlackError{
			OK:      false,
			Error:   "missing_param",
			Message: fmt.Sprintf("missing required parameter: %s", k),
		}
	}
	return v, nil
}

type ApiRequest struct {
	Params Params
	Raw    map[string]json.RawMessage
}

func (r ApiRequest) JSONInto(key string, dst any) bool {
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

func ParseRequest(r *http.Request) (ApiRequest, error) {
	// Abstracts input in either form encoded or json content types
	req := ApiRequest{
		Params: make(Params),
		Raw:    make(map[string]json.RawMessage),
	}
	contentType := r.Header.Get("Content-Type")

	switch {
	case strings.HasPrefix(contentType, constants.CONTENT_TYPE_FORM):
		if err := r.ParseForm(); err != nil {
			return ApiRequest{}, err
		}
		for k, vals := range r.PostForm {
			req.Params[k] = append([]string{}, vals...)
		}
	case strings.HasPrefix(contentType, constants.CONTENT_TYPE_JSON):
		var payload map[string]any
		bodyBytes, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			log.Printf("Error reading request body: %s\n", readErr)
			return ApiRequest{}, readErr
		}

		// If you need to allow empty body as "{}" semantics:
		if len(bytes.TrimSpace(bodyBytes)) == 0 {
			bodyBytes = []byte("{}")
		}

		if err := json.Unmarshal(bodyBytes, &payload); err != nil {
			log.Printf("Error parsing JSON body: %v", err)
			return ApiRequest{}, err
		}

		for k, v := range payload {
			switch t := v.(type) {
			case string:
				req.Params[k] = []string{t}
			case float64:
				req.Params[k] = []string{strconv.FormatFloat(t, 'f', -1, 64)}
			case bool:
				req.Params[k] = []string{strconv.FormatBool(t)}
			default:
				// TODO: this is ChatGPT's idea - not sure if i'll stick with this exact approach
				// complex values -> Raw (opt-in parsing by handlers)
				b, err := json.Marshal(t)
				if err != nil {
					log.Printf("Error marshaling JSON body: %s\n", err)
					return ApiRequest{}, err
				}
				req.Raw[k] = json.RawMessage(b)
			}
		}

	default:
		return ApiRequest{}, fmt.Errorf("unsupported content type: %s", contentType)
	}
	return req, nil
}
