package api

import (
	"context"
	"log"
	"net/http"

	// "net/url"
	// "strings"

	// "localslack/constants"
	"localslack/internal/api/handlers"
	"localslack/internal/core"
	"localslack/internal/utils"
	// "github.com/go-chi/chi/v5"
)

type slackHandler func(ctx context.Context, req utils.ApiRequest, d utils.Deps) (any, *core.SlackError)

type MethodSpec struct {
	Verb    string
	Handler slackHandler
}

var SlackMethodRegistry = map[string]MethodSpec{
	"auth.test":             {Verb: "POST", Handler: handlers.HandleAuthCheck},
	"conversations.archive": {Verb: "POST", Handler: handlers.HandleConversationsArchive},
	"conversations.info":    {Verb: "POST", Handler: handlers.HandleConversationsInfo},
	"conversations.history": {Verb: "POST", Handler: handlers.HandleConversationsHistory},
	"conversations.list":    {Verb: "POST", Handler: handlers.HandleConversationsList},
	"users.list":            {Verb: "GET", Handler: handlers.HandleUsersList},
}

// TODO: do i want all the parsing logic inside parserequest?
func (s *SlackServer) wrap(method string, h slackHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := utils.ParseRequest(r) // builds utils.Request{Params, Raw}
		if err != nil {
			log.Printf("Error parsing request: %s\n", err)
			utils.WriteJSON(w, http.StatusBadRequest, core.NewSlackError("invalid_arguments"))
			return
		}

		out, serr := h(r.Context(), req, s.deps)
		if serr != nil {
			log.Printf("Error handling request: %s\n", serr.Error)
			utils.WriteJSON(w, http.StatusOK, serr) // Slack errors are usually 200 + ok:false
			return
		}

		utils.WriteJSON(w, http.StatusOK, out)
	}
}
