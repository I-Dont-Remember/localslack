package handlers

import (
	"context"
	"localslack/internal/utils"
)

func ApiCheck(ctx context.Context, req utils.ApiRequest, d utils.Deps) (any, *utils.SlackError) {
	// TODO: this handles api.test functionality - not fully correct yet, but also assuming this endpoint is not super used and can be left out for now.
	errMsg := req.Params.Get("error")
	if errMsg != "" {
		return nil, utils.NewSlackError(errMsg)
	}

	return utils.SlackOK(map[string]any{}), nil
}