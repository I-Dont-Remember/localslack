package seed_test

import (
	"encoding/json"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/scenario"
	"localslack/internal/seed"
)

func fixedNow() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }

func fixture(seedStr string) scenario.Fixture {
	return seed.Fixture(seed.Options{Seed: seedStr, Now: fixedNow()})
}

func TestSameSeedSameData(t *testing.T) {
	t.Parallel()

	if diff := cmp.Diff(fixture("42"), fixture("42")); diff != "" {
		t.Errorf("same seed produced different data (-a +b):\n%s", diff)
	}
}

func TestDifferentSeedDifferentData(t *testing.T) {
	t.Parallel()

	if cmp.Equal(fixture("1"), fixture("2")) {
		t.Error("different seeds produced identical data")
	}
}

func TestFixtureValidates(t *testing.T) {
	t.Parallel()

	for i := range 40 {
		name := "seed-" + strconv.Itoa(i)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if issues := scenario.Validate(fixture(name), scenario.Reserved{}); len(issues) != 0 {
				t.Errorf("seed fixture invalid: %+v", issues)
			}
		})
	}
}

func TestShapeAcrossSeeds(t *testing.T) {
	t.Parallel()

	for i := range 40 {
		name := "seed-" + strconv.Itoa(i)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			checkShape(t, fixture(name))
		})
	}
}

func checkShape(t *testing.T, fx scenario.Fixture) {
	t.Helper()

	var humans, bots int
	for _, u := range fx.Users {
		if u.IsBot {
			bots++
			if u.Name != "localslack-bot" {
				t.Errorf("bot name = %q, want localslack-bot", u.Name)
			}
			if len(u.BotID) != 11 || u.BotID[0] != 'B' {
				t.Errorf("bot_id = %q, want B + 10 chars", u.BotID)
			}
		} else {
			humans++
			if u.BotID != "" {
				t.Errorf("human %s has bot_id %q", u.ID, u.BotID)
			}
		}
	}
	if humans < 1 || humans > 10 || bots != 1 {
		t.Errorf("humans=%d bots=%d, want 1..10 humans and 1 bot", humans, bots)
	}
	if n := len(fx.Channels); n < 2 || n > 50 {
		t.Errorf("conversations = %d, want 2..50", n)
	}
	general := fx.Channels[0]
	if general.Name != "general" {
		t.Fatalf("first channel = %q, want general", general.Name)
	}
	if len(general.Members) != len(fx.Users) {
		t.Errorf("#general members = %d, want everyone (%d)", len(general.Members), len(fx.Users))
	}
	for _, m := range fx.Messages {
		if m.Channel != general.ID {
			t.Errorf("message in %s, want only #general", m.Channel)
		}
	}
	checkThread(t, fx.Messages)
}

func checkThread(t *testing.T, msgs []scenario.Message) {
	t.Helper()

	if n := len(msgs); n < 3 || n > 25 {
		t.Errorf("#general messages = %d, want 3..25", n)
	}
	roots := map[string]bool{}
	for _, m := range msgs {
		if m.ThreadTS != "" {
			roots[m.ThreadTS] = true
		}
	}
	if len(roots) != 1 {
		t.Errorf("threads = %d, want 1", len(roots))
	}
}

// TestIdsStayStable guards the frozen RNG draw order: golden.json was captured
// from the original store-writing seeder, so any drift in ids, names, ts or
// content fails here.
func TestIdsStayStable(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("testdata/golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden map[string]any
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	for seedStr, want := range golden {
		t.Run(seedStr, func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(want, goldenShape(t, fixture(seedStr))); diff != "" {
				t.Errorf("seeded data drifted (-golden +got):\n%s", diff)
			}
		})
	}
}

// goldenShape renders fx the way testdata/golden.json was written: users and
// conversations by id, messages by conversation then ts, members sorted, with
// reply counts derived from the thread.
func goldenShape(t *testing.T, fx scenario.Fixture) any {
	t.Helper()

	g := map[string]any{"Team": []string{fx.Team.ID, fx.Team.Name, fx.Team.Domain}}
	users := slices.Clone(fx.Users)
	slices.SortFunc(users, func(a, b scenario.User) int { return strings.Compare(a.ID, b.ID) })
	var us [][]any
	for _, u := range users {
		us = append(us, []any{u.ID, u.Name, u.RealName, u.IsBot, u.BotID})
	}
	g["Users"] = us

	chans := slices.Clone(fx.Channels)
	slices.SortFunc(chans, func(a, b scenario.Channel) int { return strings.Compare(a.ID, b.ID) })
	replies, latest := map[string]int{}, map[string]string{}
	for _, m := range fx.Messages {
		if m.ThreadTS != "" {
			replies[m.ThreadTS]++
			latest[m.ThreadTS] = max(latest[m.ThreadTS], m.TS)
		}
	}
	var cs, ms [][]any
	for _, c := range chans {
		members := slices.Clone(c.Members)
		slices.Sort(members)
		cs = append(cs, []any{c.ID, c.Name, c.Creator, c.Created, members})
		for _, m := range fx.Messages {
			if m.Channel == c.ID {
				ms = append(ms, []any{c.ID, m.TS, m.User, m.Text, m.ThreadTS, replies[m.TS], latest[m.TS]})
			}
		}
	}
	g["Convs"], g["Messages"] = cs, ms

	b, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
