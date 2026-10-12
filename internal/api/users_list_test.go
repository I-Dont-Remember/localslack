package api_test

import (
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/storetest"
)

func TestUsersListPagination(t *testing.T) {
	t.Parallel()
	app := storetest.NewApp(t)
	all, err := app.Service.Users(t.Context())
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	var want []string
	for _, u := range all {
		want = append(want, u.ID)
	}

	var got []string
	cursor := ""
	for range len(all) + 1 {
		params := map[string]string{"limit": "2"}
		if cursor != "" {
			params["cursor"] = cursor
		}
		_, body := call(t, app, "users.list", "xoxb-test", params)
		members, _ := body["members"].([]any)
		if len(members) > 2 {
			t.Fatalf("page has %d members, want <= 2", len(members))
		}
		for _, m := range members {
			got = append(got, m.(map[string]any)["id"].(string))
		}
		cursor, _ = body["response_metadata"].(map[string]any)["next_cursor"].(string)
		if cursor == "" {
			break
		}
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("users across pages (-want +got):\n%s", diff)
	}

	_, body := call(t, app, "users.list", "xoxb-test", map[string]string{"cursor": "garbage", "limit": strconv.Itoa(2)})
	if body["ok"] != false || body["error"] != "invalid_cursor" {
		t.Errorf("bad cursor body = %v, want invalid_cursor", body)
	}
}
