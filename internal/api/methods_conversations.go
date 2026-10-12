package api

import (
	"context"

	"localslack/internal/service"
)

func (a *API) conversationsCreate(ctx context.Context, c *call) (map[string]any, error) {
	conv, err := a.svc.CreateChannel(ctx, c.user, c.params.Get("name"))
	if err != nil {
		return nil, err
	}
	return map[string]any{"channel": conversationDTO(conv)}, nil
}

func (a *API) conversationsList(ctx context.Context, c *call) (map[string]any, error) {
	page, err := a.svc.ListConversations(ctx, c.user, service.ListOpts{
		Types:           csv(c.params.Get("types")),
		ExcludeArchived: boolParam(c.params, "exclude_archived"),
		Cursor:          c.params.Get("cursor"),
		Limit:           intParam(c.params, "limit"),
	})
	if err != nil {
		return nil, err
	}
	channels := make([]map[string]any, 0, len(page.Items))
	for _, conv := range page.Items {
		channels = append(channels, conversationDTO(conv))
	}
	return map[string]any{"channels": channels, "response_metadata": cursorMeta(page.NextCursor)}, nil
}

func (a *API) conversationsOpen(ctx context.Context, c *call) (map[string]any, error) {
	var conv service.Conversation
	var err error
	if id := c.params.Get("channel"); id != "" {
		conv, err = a.svc.Conversation(ctx, c.user, id)
	} else {
		conv, err = a.svc.OpenDM(ctx, c.user, csv(c.params.Get("users")))
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"channel": conversationDTO(conv)}, nil
}

func (a *API) conversationsHistory(ctx context.Context, c *call) (map[string]any, error) {
	page, err := a.svc.History(ctx, c.user, c.params.Get("channel"), service.PageOpts{
		Cursor: c.params.Get("cursor"), Limit: intParam(c.params, "limit"),
		Oldest: c.params.Get("oldest"), Latest: c.params.Get("latest"), Inclusive: boolParam(c.params, "inclusive"),
	})
	if err != nil {
		return nil, err
	}
	return messagesBody(page), nil
}

func (a *API) conversationsReplies(ctx context.Context, c *call) (map[string]any, error) {
	page, err := a.svc.Replies(ctx, c.user, c.params.Get("channel"), c.params.Get("ts"), service.PageOpts{
		Cursor: c.params.Get("cursor"), Limit: intParam(c.params, "limit"),
	})
	if err != nil {
		return nil, err
	}
	return messagesBody(page), nil
}

func messagesBody(page service.Page[service.Message]) map[string]any {
	msgs := make([]map[string]any, 0, len(page.Items))
	for _, m := range page.Items {
		msgs = append(msgs, messageDTO(m))
	}
	return map[string]any{
		"messages": msgs, "has_more": page.NextCursor != "", "pin_count": 0,
		"response_metadata": cursorMeta(page.NextCursor),
	}
}

func (a *API) conversationMethods() map[string]spec {
	return map[string]spec{
		"conversations.create":  tok(a.conversationsCreate),
		"conversations.list":    tok(a.conversationsList),
		"conversations.open":    tok(a.conversationsOpen),
		"conversations.history": tok(a.conversationsHistory),
		"conversations.replies": tok(a.conversationsReplies),
		"conversations.archive": tok(a.conversationsArchive),
	}
}
