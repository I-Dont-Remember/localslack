package api

import (
	"context"
	"log"
	"net/http"

	"localslack/internal/api/handlers"
	"localslack/internal/core"
	"localslack/internal/utils"
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

func (s *SlackServer) wrap(method string, h slackHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Incoming request: %s %s %+v\n", r.Method, r.URL.Path, r)
		req, err := utils.ParseRequest(r)
		if err != nil {
			log.Printf("Error parsing request: %s\n", err)
			utils.WriteJSON(w, http.StatusBadRequest, core.NewSlackError("invalid_arguments"))
			return
		}

		out, serr := h(r.Context(), req, s.deps)
		if serr != nil {
			log.Printf("Error handling request: %s\n", serr.Error)
			utils.WriteJSON(w, http.StatusOK, serr)
			return
		}

		err = utils.WriteJSON(w, http.StatusOK, out)
		if err != nil {
			log.Printf("Error writing response: %s\n", err)
			return
		}
		log.Printf("Response: %s\n", out)
	}
}
