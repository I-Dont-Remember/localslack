package utils

import (
	"encoding/json"
	"net/http"
)

func WriteJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}

// type SlackBaseResponse struct {
// 	OK    bool   `json:"ok"`
// 	Error string `json:"error,omitempty"`
// 	LocalSlackMessage string `json:"localslack_message,omitempty"` // This one is just for us debugging
// }

// func SlackError(errMsg string) SlackBaseResponse {
// 	return SlackBaseResponse{
// 		OK:    false,
// 		Error: errMsg,
// 	}
// }

// func SlackErrorMsg(errMsg string, message string) SlackBaseResponse {
// 	return SlackBaseResponse{
// 		OK:    false,
// 		Error: errMsg,
// 		LocalSlackMessage: message,
// 	}
// }

type SlackError struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

func NewSlackError(errMsg string) *SlackError {
	return &SlackError{OK: false, Error: errMsg}
}

func NewSlackErrorMsg(errMsg string, message string) *SlackError {
	return &SlackError{OK: false, Error: errMsg, Message: message}
}

func SlackOK(extra map[string]any) map[string]any {
	resp := make(map[string]any, len(extra)+1)
	resp["ok"] = true
	for k, v := range extra {
		if k != "ok" {
			resp[k] = v
		}
	}
	return resp
}
