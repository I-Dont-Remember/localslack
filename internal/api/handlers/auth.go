package handlers

import (
	"context"
	"unicode/utf8"
	"localslack/internal/utils"
	"localslack/internal/core"
)

func HandleAuthCheck(ctx context.Context, req utils.Request, d utils.Deps) (any, *core.SlackError) {
	token, err := req.Params.Require("token")
    if err != nil { return nil, err }

	if utf8.RuneCountInString(token) <= 2 {
		return nil, &core.SlackError{
			OK:      false,
			Error:   "invalid_auth",
			Message: "invalid_auth",
		}
	}

	return map[string]any{
		"ok": true,
		// ... other stuff from the token's login like team id 
	 }, nil
}
