package service

import (
	"context"

	"localslack/internal/store"
)

func appFrom(a store.App) App {
	return App{
		ID: a.ID, Name: a.Name, BotToken: a.BotToken, AppToken: a.AppToken,
		BotUserID: a.BotUserID, BotID: a.BotID, Scopes: a.Scopes, SocketMode: a.SocketMode,
	}
}

// RefreshApps reloads the token->app cache from the store. Call on startup and
// after any register/delete/revoke/reset.
func (s *Service) RefreshApps(ctx context.Context) error {
	rows, err := s.st.Apps(ctx)
	if err != nil {
		return err
	}
	apps := make([]App, 0, len(rows))
	for _, r := range rows {
		apps = append(apps, appFrom(r))
	}
	s.SetApps(apps...)
	return nil
}
