package api

import (
	"context"
	"net/http"
	// "net/url"
	// "strings"

	// "localslack/constants"
	"localslack/internal/api/handlers"
	"localslack/internal/utils"
	"localslack/internal/core"

	// "github.com/go-chi/chi/v5"
)

type slackHandler func(ctx context.Context, req utils.Request, d utils.Deps) (any, *core.SlackError)

type MethodSpec struct {
  Verb   	string
  Handler slackHandler
}

var SlackMethodRegistry = map[string]MethodSpec{
	"auth.test": {Verb: "POST", Handler: handlers.HandleAuthCheck},
}

// TODO: do i want all the parsing logic inside parserequest?
func (s *SlackServer) wrap(method string, h slackHandler) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    req, err := utils.ParseRequest(r) // builds utils.Request{Params, Raw}
    if err != nil {
      utils.WriteJSON(w, http.StatusBadRequest, core.NewSlackError("invalid_arguments"))
      return
    }

    out, serr := h(r.Context(), req, s.deps)
    if serr != nil {
      utils.WriteJSON(w, http.StatusOK, serr) // Slack errors are usually 200 + ok:false
      return
    }

    utils.WriteJSON(w, http.StatusOK, out)
  }
}

// func (s *Server) slackMethodHandler(w http.ResponseWriter, r *http.Request) {
// 	method := chi.URLParam(r, "method")

// 	var data map[string]interface{}

// 	handler, ok := SlackMethodRegistry[method]
// 	if !ok {
// 		utils.WriteJSON(w, http.StatusOK, map[string]any{
// 			"ok":    false,
// 			"error": "method_not_supported",
// 		})
// 		return
// 	}

// 	contentType := r.Header.Get("Content-Type")

// 	if strings.HasPrefix(contentType,constants.CONTENT_TYPE_FORM) {
// 		if err := r.ParseForm(); err != nil {
// 			utils.WriteJSON(w, http.StatusBadRequest, map[string]any{
// 				"ok":    false,
// 				"error": "invalid_form",
// 			})
// 			return
// 		}
// 		data = r.PostForm
// 	} else if contentType == constants.CONTENT_TYPE_JSON {
// 		var payload interface{}
// 		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
// 			utils.WriteJSON(w, http.StatusBadRequest, map[string]any{
// 				"ok":    false,
// 				"error": "invalid_json",
// 			})
// 			return
// 		}
// 		data = payload.(map[string]interface{})
// 	} else {
// 		utils.WriteJSON(w, http.StatusBadRequest, map[string]any{
// 			"ok":    false,
// 			"error": "unsupported_content_type",
// 		})
// 	}

// 	// TODO: i gotta convert this Go output back to HTTP responses?
// 	handler(r.Context(), data, s, w)
// }
