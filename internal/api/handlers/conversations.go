package handlers

import (
	"context"
	"localslack/internal/core"
	"localslack/internal/utils"
	"strconv"
)

func HandleConversationsArchive(ctx context.Context, req utils.ApiRequest, d utils.Deps) (any, *utils.SlackError) {
	return nil, nil
}

func HandleConversationsCreate(ctx context.Context, req utils.ApiRequest, d utils.Deps) (any, *utils.SlackError) {
	name, err := req.Params.Require("name")
	if err != nil {
		return nil, err
	}

	IsPrivateParam := req.Params.GetOrDefault("is_private", "false")
	isPrivate, parseErr := strconv.ParseBool(IsPrivateParam)
	if parseErr != nil {
		return nil, utils.NewSlackError(parseErr.Error())
	}
	// teamID := req.Params.GetOrDefault("team_id", "") // TODO: only needed for org tokens

	conversation := core.SlackConversation{
		ID:        "FAKE_CONVERSATION_ID",
		Name:      name,
		IsChannel: true,
		IsPrivate: isPrivate,
		IsIM:      false,
		IsMpim:    false,
	}
	return utils.SlackOK(map[string]any{
		"channel": conversation,
	}), nil
}

func HandleConversationsHistory(ctx context.Context, req utils.ApiRequest, d utils.Deps) (any, *utils.SlackError) {
	return nil, nil
}

func HandleConversationsInfo(ctx context.Context, req utils.ApiRequest, d utils.Deps) (any, *utils.SlackError) {
	return nil, nil
}

func HandleConversationsList(ctx context.Context, req utils.ApiRequest, d utils.Deps) (any, *utils.SlackError) {
	limit, err := strconv.Atoi(req.Params.GetOrDefault("limit", "100"))
	if err != nil || limit <= 0 {
		return nil, utils.NewSlackErrorMsg("invalid_arguments", "invalid limit")
	}

	conversations := []core.SlackConversation{
		{
			ID:        "FAKE_CONVERSATION_ID",
			Name:      "FAKE_CONVERSATION_NAME",
			IsChannel: true,
			IsPrivate: false,
			IsIM:      false,
			IsMpim:    false,
		},
	}

	return map[string]any{
		"ok":       true,
		"channels": conversations,
		"response_metadata": map[string]string{
			"next_cursor": "dXNlcjpVMEc5V0ZYTlo=",
		},
	}, nil
}
