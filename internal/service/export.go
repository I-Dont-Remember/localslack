package service

import (
	"context"
	"slices"

	"localslack/internal/scenario"
	"localslack/internal/store"
)

// Export returns the whole workspace as a scenario Fixture (the snapshot and
// --scenario format), read in ONE store read transaction so concurrent writes
// can never tear it.
//
// App bot users are never defined in Users: apps persist across reset and
// restore, which re-create their bot users. Their membership of public,
// unarchived channels is omitted too, because restore re-joins them there; their
// membership of private or archived channels and of DMs, and the messages and
// reactions they authored, stay so a restore loses nothing. Such a fixture
// references app bot ids without defining them, so only a workspace that still
// has the apps accepts it.
//
// Thread roots carry no ThreadTS (only replies do), reply counts are derived.
func (s *Service) Export(ctx context.Context) (scenario.Fixture, error) {
	var out scenario.Fixture
	err := s.st.Read(ctx, func(r *store.Reader) error {
		var err error
		out, err = exportFrom(ctx, r)
		return err
	})
	return out, err
}

func exportFrom(ctx context.Context, r *store.Reader) (scenario.Fixture, error) {
	team, err := r.Team(ctx)
	if err != nil {
		return scenario.Fixture{}, err
	}
	apps, err := r.Apps(ctx)
	if err != nil {
		return scenario.Fixture{}, err
	}
	appBots := map[string]bool{}
	for _, a := range apps {
		appBots[a.BotUserID] = true
	}
	fx := scenario.Fixture{Version: scenario.Version, Team: &scenario.Team{ID: team.ID, Name: team.Name, Domain: team.Domain}}
	if err := exportUsers(ctx, r, appBots, &fx); err != nil {
		return scenario.Fixture{}, err
	}
	if err := exportConversations(ctx, r, appBots, &fx); err != nil {
		return scenario.Fixture{}, err
	}
	if err := exportMessages(ctx, r, &fx); err != nil {
		return scenario.Fixture{}, err
	}
	return fx, nil
}

func exportUsers(ctx context.Context, r *store.Reader, appBots map[string]bool, fx *scenario.Fixture) error {
	users, err := r.Users(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		if !appBots[u.ID] {
			fx.Users = append(fx.Users, scenario.User{ID: u.ID, Name: u.Name, RealName: u.RealName, IsBot: u.IsBot, BotID: u.BotID})
		}
	}
	return nil
}

func exportConversations(ctx context.Context, r *store.Reader, appBots map[string]bool, fx *scenario.Fixture) error {
	convs, err := r.Conversations(ctx)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(convs))
	for _, c := range convs {
		ids = append(ids, c.ID)
	}
	members, err := r.MembersOf(ctx, ids)
	if err != nil {
		return err
	}
	for _, c := range convs {
		m := slices.Clone(members[c.ID])
		if c.Kind == KindChannel {
			if !c.IsPrivate && !c.IsArchived { // restore re-joins app bots here
				m = slices.DeleteFunc(m, func(u string) bool { return appBots[u] })
			}
			fx.Channels = append(fx.Channels, scenario.Channel{
				ID: c.ID, Name: c.Name, Creator: c.Creator, Created: c.Created,
				IsArchived: c.IsArchived, IsPrivate: c.IsPrivate, Members: m,
			})
			continue
		}
		fx.DMs = append(fx.DMs, scenario.DM{ID: c.ID, Creator: c.Creator, Created: c.Created, Members: m})
	}
	return nil
}

func exportMessages(ctx context.Context, r *store.Reader, fx *scenario.Fixture) error {
	msgs, err := r.AllMessages(ctx)
	if err != nil {
		return err
	}
	reacts, err := r.AllReactions(ctx)
	if err != nil {
		return err
	}
	type key struct{ channel, ts string }
	byMsg := map[key][]scenario.Reaction{}
	for _, rc := range reacts {
		k := key{rc.ChannelID, rc.TS}
		list := byMsg[k]
		i := slices.IndexFunc(list, func(x scenario.Reaction) bool { return x.Name == rc.Name })
		if i < 0 {
			list = append(list, scenario.Reaction{Name: rc.Name})
			i = len(list) - 1
		}
		list[i].Users = append(list[i].Users, rc.UserID)
		byMsg[k] = list
	}
	for _, m := range msgs {
		out := scenario.Message{
			Channel: m.ChannelID, TS: m.TS, User: m.UserID, Text: m.Text, ThreadTS: m.ThreadTS,
			BotID: m.BotID, Subtype: m.Subtype, Blocks: m.Blocks, Attachments: m.Attachments, Metadata: m.Metadata,
			Reactions: byMsg[key{m.ChannelID, m.TS}],
		}
		if m.Edited != nil {
			out.Edited = &scenario.Edit{User: m.Edited.User, TS: m.Edited.TS}
		}
		fx.Messages = append(fx.Messages, out)
	}
	return nil
}
