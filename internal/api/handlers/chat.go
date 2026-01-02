package handlers

import (
	"context"
	"localslack/internal/core"
	"localslack/internal/utils"
)

func HandleChatPostMessage(ctx context.Context, req utils.ApiRequest, d utils.Deps) (any, *utils.SlackError) {
	conversationId, err := req.Params.Require("channel")
	if err != nil {
		return nil, err
	}
	newTs := "111111111"
	newMessage := &core.SlackMessage{
		Text: "new posted message",
		Ts:   newTs,
	}
	return utils.SlackOK(map[string]any{
		"channel": conversationId,
		"ts":      newTs,
		"message": newMessage,
	}), nil
}

func HandleChatUpdate(ctx context.Context, req utils.ApiRequest, d utils.Deps) (any, *utils.SlackError) {
	conversationId, err := req.Params.Require("channel")
	tsToBeUpdated, err := req.Params.Require("ts")
	if err != nil {
		return nil, err
	}
	text := "Text from an update"

	return utils.SlackOK(map[string]any{
		"channel": conversationId,
		"ts":      tsToBeUpdated,
		"text":    text,
		"message": &core.SlackMessage{
			Text: text,
			User: "U000001",
		},
	}), nil
}
