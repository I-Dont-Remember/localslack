package api

import (
	"context"
	"log"
	"net/http"

	"localslack/internal/api/handlers"
	"localslack/internal/utils"
)

type slackHandler func(ctx context.Context, req utils.ApiRequest, d utils.Deps) (any, *utils.SlackError)

type MethodSpec struct {
	Verbs   []string
	Handler slackHandler
}

var SlackMethodRegistry = map[string]MethodSpec{
	// !! IMPORTANT: turns out Slack supports POST against all their method endpoints,
	// and the Python SDK will send as POST every time and ignore if it's a GET or other request method
	// in the code - https://github.com/slackapi/python-slack-sdk/blob/6fc2d5c082f70ca3e222a7644e6eb202f2140893/slack/web/base_client.py#L402
	"auth.test":             {Verbs: []string{"POST"}, Handler: handlers.HandleAuthCheck},
	"conversations.archive": {Verbs: []string{"POST"}, Handler: handlers.HandleConversationsArchive},
	"conversations.info":    {Verbs: []string{"POST"}, Handler: handlers.HandleConversationsInfo},
	"conversations.history": {Verbs: []string{"POST"}, Handler: handlers.HandleConversationsHistory},
	"conversations.create":  {Verbs: []string{"POST"}, Handler: handlers.HandleConversationsCreate},
	"conversations.list":    {Verbs: []string{"GET", "POST"}, Handler: handlers.HandleConversationsList},
	"users.list":            {Verbs: []string{"GET", "POST"}, Handler: handlers.HandleUsersList}, // Requires POST because Slack Python client always uses it.
}

func (s *SlackServer) wrap(method string, h slackHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Incoming request: %s %s %+v\n", r.Method, r.URL.Path, r)
		req, err := utils.ParseRequest(r)
		if err != nil {
			log.Printf("Error parsing request: %s\n", err)
			utils.WriteJSON(w, http.StatusBadRequest, utils.NewSlackError("invalid_arguments"))
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
	}
}
