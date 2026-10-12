package service_test

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/service"
)

func TestUsersPage(t *testing.T) {
	t.Parallel()
	svc, _ := newSvc(t)
	all, err := svc.Users(t.Context())
	if err != nil || len(all) < 3 {
		t.Fatalf("Users = %d, %v; want >= 3 seeded users", len(all), err)
	}
	ids := func(us []service.User) []string {
		var out []string
		for _, u := range us {
			out = append(out, u.ID)
		}
		return out
	}

	t.Run("no limit returns everyone", func(t *testing.T) {
		t.Parallel()
		got, err := svc.UsersPage(t.Context(), service.PageOpts{})
		if err != nil || got.NextCursor != "" {
			t.Fatalf("page = %+v, %v; want all users and no cursor", got, err)
		}
		if diff := cmp.Diff(ids(all), ids(got.Items)); diff != "" {
			t.Errorf("users (-want +got):\n%s", diff)
		}
	})
	t.Run("walking pages yields every user once", func(t *testing.T) {
		t.Parallel()
		var got []service.User
		cursor := ""
		for range len(all) + 1 {
			page, err := svc.UsersPage(t.Context(), service.PageOpts{Limit: 2, Cursor: cursor})
			if err != nil {
				t.Fatalf("UsersPage: %v", err)
			}
			if len(page.Items) > 2 {
				t.Fatalf("page has %d items, want <= 2", len(page.Items))
			}
			got = append(got, page.Items...)
			if cursor = page.NextCursor; cursor == "" {
				break
			}
		}
		if diff := cmp.Diff(ids(all), ids(got)); diff != "" {
			t.Errorf("users (-want +got):\n%s", diff)
		}
	})
	t.Run("invalid cursor", func(t *testing.T) {
		t.Parallel()
		_, err := svc.UsersPage(t.Context(), service.PageOpts{Cursor: "garbage"})
		if !errors.Is(err, service.ErrInvalidCursor) {
			t.Errorf("err = %v, want ErrInvalidCursor", err)
		}
	})
}
