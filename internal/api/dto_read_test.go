package api

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/service"
)

func TestUserDTOBot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		user service.User
		want map[string]any
	}{
		{
			"human",
			service.User{ID: "U1", Name: "ada", RealName: "Ada"},
			map[string]any{"real_name": "Ada", "display_name": "ada"},
		},
		{
			"bot",
			service.User{ID: "U2", Name: "bot", RealName: "Bot", IsBot: true, BotID: "B1"},
			map[string]any{"real_name": "Bot", "display_name": "bot", "bot_id": "B1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := userDTO(tt.user, "T1")["profile"]
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("profile mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConversationDTOPrivacy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		conv service.Conversation
		want map[string]bool
	}{
		{
			"public channel",
			service.Conversation{Kind: service.KindChannel},
			map[string]bool{"is_channel": true, "is_group": false, "is_private": false},
		},
		{
			"private channel",
			service.Conversation{Kind: service.KindChannel, IsPrivate: true},
			map[string]bool{"is_channel": false, "is_group": true, "is_private": true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dto := conversationDTO(tt.conv)
			got := map[string]bool{}
			for k := range tt.want {
				got[k], _ = dto[k].(bool)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("flags mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
