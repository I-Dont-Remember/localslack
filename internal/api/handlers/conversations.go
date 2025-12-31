package handlers

import (
	"context"
	"localslack/internal/core"
	"localslack/internal/utils"
	"strconv"
)

func HandleConversationsArchive(ctx context.Context, req utils.ApiRequest, d utils.Deps) (any, *core.SlackError) {
	return nil, nil
}

func HandleConversationsCreate(ctx context.Context, req utils.ApiRequest, d utils.Deps) (any, *core.SlackError) {
	return nil, nil
}

func HandleConversationsHistory(ctx context.Context, req utils.ApiRequest, d utils.Deps) (any, *core.SlackError) {
	return nil, nil
}

func HandleConversationsInfo(ctx context.Context, req utils.ApiRequest, d utils.Deps) (any, *core.SlackError) {
	return nil, nil
}

func HandleConversationsList(ctx context.Context, req utils.ApiRequest, d utils.Deps) (any, *core.SlackError) {
	limit, err := strconv.Atoi(req.Params.GetOrDefault("limit", "100"))
	if err != nil || limit <= 0 {
		return nil, &core.SlackError{
			OK:      false,
			Error:   "invalid_arguments",
			Message: "invalid_arguments",
		}
	}

	return nil, nil
}
