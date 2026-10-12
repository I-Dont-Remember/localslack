package scenario

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// maxIssues caps a Validate report so a hopeless file does not drown the reply.
const maxIssues = 200

// Reserved are identities the server owns (registered app bot users): a fixture
// may reference them as message authors, reactors and DM/private members, but
// must not define or collide with them.
type Reserved struct {
	UserIDs, BotIDs, UserNames []string
}

var (
	idBodyRE   = regexp.MustCompile(`^[A-Z0-9]+$`)
	tsRE       = regexp.MustCompile(`^[0-9]{1,11}\.[0-9]{6}$`)
	userNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	chanNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,79}$`)
	emojiRE    = regexp.MustCompile(`^[a-z0-9_+-]+$`)
	emailRE    = regexp.MustCompile(`[A-Za-z0-9._%+-]+@([A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)+)`)
)

// Validate returns every problem in fx (nil if none), each at a JSON pointer, in
// document order. Call Resolve first for hand-written files that omit ids.
func Validate(fx Fixture, res Reserved) []Issue {
	v := newValidator(res)
	if fx.Version != Version {
		v.add("/version", "unsupported version %d; want %d", fx.Version, Version)
	}
	v.team(fx.Team)
	v.users(fx.Users)
	v.channels(fx.Channels)
	v.dms(fx.DMs)
	v.messages(fx.Messages)
	v.generate(fx)
	if len(v.issues) > maxIssues {
		v.issues = append(v.issues[:maxIssues], Issue{Message: fmt.Sprintf("too many problems; showing the first %d", maxIssues)})
	}
	return v.issues
}

type msgKey struct{ channel, ts string }

type userInfo struct {
	isBot bool
	botID string
}

type validator struct {
	issues []Issue

	resUsers, resBots, resNames map[string]bool

	userSet map[string]userInfo // fixture users by id
	convs   map[string]string   // conversation id -> pointer
	roots   map[msgKey]bool
}

func newValidator(res Reserved) *validator {
	set := func(s []string) map[string]bool {
		m := make(map[string]bool, len(s))
		for _, x := range s {
			m[x] = true
		}
		return m
	}
	return &validator{
		resUsers: set(res.UserIDs), resBots: set(res.BotIDs), resNames: set(res.UserNames),
		userSet: map[string]userInfo{}, convs: map[string]string{},
		roots: map[msgKey]bool{},
	}
}

func (v *validator) add(ptr, format string, args ...any) {
	if len(v.issues) > maxIssues {
		return
	}
	v.issues = append(v.issues, Issue{Pointer: ptr, Message: fmt.Sprintf(format, args...)})
}

func ptr(parts ...any) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteByte('/')
		fmt.Fprint(&b, p)
	}
	return b.String()
}

// prefix checks that id is non-empty, upper-case alphanumeric and starts with one of letters.
func (v *validator) prefix(at, id, letters string) bool {
	switch {
	case id == "":
		v.add(at, "id is required")
	case !strings.ContainsRune(letters, rune(id[0])) || !idBodyRE.MatchString(id):
		v.add(at, "id %q must start with %s", id, orList(letters))
	default:
		return true
	}
	return false
}

func orList(letters string) string {
	return strings.Join(strings.Split(letters, ""), " or ")
}

func (v *validator) team(t *Team) {
	if t == nil {
		return
	}
	v.prefix("/team/id", t.ID, "T")
	if t.Name == "" {
		v.add("/team/name", "name is required")
	}
	if t.Domain == "" {
		v.add("/team/domain", "domain is required")
	}
}

func (v *validator) users(users []User) {
	ids, names, bots := map[string]int{}, map[string]int{}, map[string]int{}
	for i, u := range users {
		v.userID(i, u, ids)
		v.userName(i, u, names)
		v.email(ptr("users", i, "real_name"), u.RealName)
		v.userBot(i, u, bots)
		if _, dup := v.userSet[u.ID]; !dup {
			v.userSet[u.ID] = userInfo{isBot: u.IsBot, botID: u.BotID}
		}
	}
}

func (v *validator) userID(i int, u User, seen map[string]int) {
	at := ptr("users", i, "id")
	if !v.prefix(at, u.ID, "UW") {
		return
	}
	if j, dup := seen[u.ID]; dup {
		v.add(at, "duplicate id %q (also %s)", u.ID, ptr("users", j, "id"))
		return
	}
	seen[u.ID] = i
	if v.resUsers[u.ID] {
		v.add(at, "id %q belongs to an app bot user", u.ID)
	}
}

func (v *validator) userName(i int, u User, seen map[string]int) {
	at := ptr("users", i, "name")
	v.email(at, u.Name)
	switch {
	case u.Name == "":
		v.add(at, "name is required")
		return
	case !userNameRE.MatchString(u.Name):
		v.add(at, "name must be lowercase letters, digits, '.', '_' or '-'")
	}
	key := strings.ToLower(u.Name)
	if j, dup := seen[key]; dup {
		v.add(at, "duplicate name %q (also %s)", u.Name, ptr("users", j, "name"))
		return
	}
	seen[key] = i
	if v.resNames[u.Name] {
		v.add(at, "name %q belongs to an app bot user", u.Name)
	}
}

func (v *validator) userBot(i int, u User, seen map[string]int) {
	at := ptr("users", i, "bot_id")
	switch {
	case u.IsBot && u.BotID == "":
		v.add(at, "is_bot users need a bot_id")
		return
	case !u.IsBot && u.BotID != "":
		v.add(at, "bot_id is only for is_bot users")
		return
	case u.BotID == "":
		return
	}
	if !strings.HasPrefix(u.BotID, "B") || !idBodyRE.MatchString(u.BotID) {
		v.add(at, "bot_id %q must start with B", u.BotID)
		return
	}
	if j, dup := seen[u.BotID]; dup {
		v.add(at, "duplicate bot_id %q (also %s)", u.BotID, ptr("users", j, "bot_id"))
		return
	}
	seen[u.BotID] = i
	if v.resBots[u.BotID] {
		v.add(at, "bot_id %q belongs to an app bot user", u.BotID)
	}
}

// email flags addresses outside example.com: fixtures are shared, so only synthetic ones.
func (v *validator) email(at, s string) {
	for _, m := range emailRE.FindAllStringSubmatch(s, -1) {
		if !strings.EqualFold(m[1], "example.com") {
			v.add(at, "email %q is not synthetic; use @example.com", m[0])
		}
	}
}

// knownUser reports whether id is defined in the fixture or reserved to an app bot.
func (v *validator) knownUser(id string) bool {
	_, ok := v.userSet[id]
	return ok || v.resUsers[id]
}

func (v *validator) user(at, id string) {
	if id != "" && !v.knownUser(id) {
		v.add(at, "unknown user %q", id)
	}
}

func (v *validator) convID(at, id, letters string) {
	if !v.prefix(at, id, letters) {
		return
	}
	if other, dup := v.convs[id]; dup {
		v.add(at, "duplicate id %q (also %s)", id, other)
		return
	}
	v.convs[id] = at
}

func (v *validator) channels(chs []Channel) {
	names := map[string]int{}
	for i, c := range chs {
		v.convID(ptr("channels", i, "id"), c.ID, "CG")
		v.channelName(i, c, names)
		v.user(ptr("channels", i, "creator"), c.Creator)
		if c.Created < 0 {
			v.add(ptr("channels", i, "created"), "created must not be negative")
		}
		v.members(ptr("channels", i, "members"), c.Members, !c.IsPrivate && !c.IsArchived)
	}
}

func (v *validator) channelName(i int, c Channel, seen map[string]int) {
	at := ptr("channels", i, "name")
	if !chanNameRE.MatchString(c.Name) {
		v.add(at, "name must be lowercase letters, digits, '-' or '_' (max 80)")
	}
	key := strings.ToLower(c.Name)
	if j, dup := seen[key]; dup {
		v.add(at, "duplicate name %q (also %s)", c.Name, ptr("channels", j, "name"))
		return
	}
	seen[key] = i
}

// members checks each id is known and unique; autoJoined channels refuse app bots.
func (v *validator) members(at string, members []string, autoJoined bool) {
	seen := map[string]bool{}
	for i, m := range members {
		mp := at + "/" + strconv.Itoa(i)
		switch {
		case seen[m]:
			v.add(mp, "duplicate member %q", m)
		case autoJoined && v.resUsers[m]:
			v.add(mp, "app bot %q is joined to public channels automatically; remove it", m)
		default:
			v.user(mp, m)
		}
		seen[m] = true
	}
}

func (v *validator) dms(dms []DM) {
	sets := map[string]int{}
	for i, d := range dms {
		letters := "D"
		if len(d.Members) > 2 {
			letters = "G"
		}
		at := ptr("dms", i, "id")
		if d.ID != "" && idBodyRE.MatchString(d.ID) && d.ID[0] != letters[0] {
			v.add(at, "id %q must start with %s (%s)", d.ID, letters, sizeRule(letters))
			letters = "DG"
		}
		v.convID(at, d.ID, letters)
		v.members(ptr("dms", i, "members"), d.Members, false)
		if len(d.Members) == 0 {
			v.add(ptr("dms", i, "members"), "need at least one member")
			continue
		}
		key := strings.Join(slices.Sorted(slices.Values(d.Members)), ",")
		if j, dup := sets[key]; dup {
			v.add(ptr("dms", i, "members"), "same members as %s", ptr("dms", j))
			continue
		}
		sets[key] = i
	}
}

func sizeRule(letters string) string {
	if letters == "D" {
		return "2 or fewer members"
	}
	return "3 or more members"
}

func (v *validator) messages(msgs []Message) {
	for _, m := range msgs {
		if m.TS != "" && (m.ThreadTS == "" || m.ThreadTS == m.TS) {
			v.roots[msgKey{m.Channel, m.TS}] = true
		}
	}
	first := map[msgKey]int{}
	for i, m := range msgs {
		v.message(i, m, first)
	}
}

func (v *validator) message(i int, m Message, first map[msgKey]int) {
	p := func(f string) string { return ptr("messages", i, f) }
	if _, ok := v.convs[m.Channel]; !ok {
		if m.Channel == "" {
			v.add(p("channel"), "channel is required")
		} else {
			v.add(p("channel"), "unknown conversation %q", m.Channel)
		}
	}
	tsOK := v.ts(p("ts"), m.TS)
	if m.User == "" {
		v.add(p("user"), "user is required")
	}
	v.user(p("user"), m.User)
	if tsOK {
		k := msgKey{m.Channel, m.TS}
		if j, dup := first[k]; dup {
			v.add(p("ts"), "duplicate message %s/%s (also %s)", m.Channel, m.TS, ptr("messages", j))
		} else {
			first[k] = i
		}
	}
	v.thread(p("thread_ts"), m, tsOK)
	v.messageBot(p("bot_id"), m)
	if m.Edited != nil {
		v.user(p("edited")+"/user", m.Edited.User)
		v.ts(p("edited")+"/ts", m.Edited.TS)
	}
	v.reactions(p("reactions"), m.Reactions)
	v.content(p("blocks"), "blocks", m.Blocks, '[', "array")
	v.content(p("attachments"), "attachments", m.Attachments, '[', "array")
	v.content(p("metadata"), "metadata", m.Metadata, '{', "object")
}

func (v *validator) ts(at, ts string) bool {
	switch {
	case ts == "":
		v.add(at, "ts is required")
	case !tsRE.MatchString(ts):
		v.add(at, "ts %q must look like 1700000000.000100", ts)
	default:
		return true
	}
	return false
}

func (v *validator) thread(at string, m Message, tsOK bool) {
	if m.ThreadTS == "" || m.ThreadTS == m.TS {
		return
	}
	if !tsRE.MatchString(m.ThreadTS) || !v.roots[msgKey{m.Channel, m.ThreadTS}] {
		v.add(at, "thread_ts %q is not a root message in this channel", m.ThreadTS)
		return
	}
	if tsOK && !tsBefore(m.ThreadTS, m.TS) {
		v.add(at, "reply ts must be later than its thread_ts")
	}
}

// tsBefore compares well-formed Slack timestamps numerically.
func tsBefore(a, b string) bool {
	sa, ma, _ := strings.Cut(a, ".")
	sb, mb, _ := strings.Cut(b, ".")
	ia, _ := strconv.ParseInt(sa, 10, 64)
	ib, _ := strconv.ParseInt(sb, 10, 64)
	if ia != ib {
		return ia < ib
	}
	return ma < mb
}

func (v *validator) messageBot(at string, m Message) {
	if m.BotID == "" {
		return
	}
	if !strings.HasPrefix(m.BotID, "B") || !idBodyRE.MatchString(m.BotID) {
		v.add(at, "bot_id %q must start with B", m.BotID)
		return
	}
	author, ok := v.userSet[m.User]
	switch {
	case !ok:
		// app bot (reserved) or unknown author: nothing more to check.
	case !author.isBot:
		v.add(at, "bot_id set but author %s is not a bot", m.User)
	case author.botID != m.BotID:
		v.add(at, "bot_id %q does not match author bot_id %q", m.BotID, author.botID)
	}
}

func (v *validator) reactions(at string, rs []Reaction) {
	seen := map[string]int{}
	for i, r := range rs {
		rp := at + "/" + strconv.Itoa(i)
		if !emojiRE.MatchString(r.Name) {
			v.add(rp+"/name", "reaction name must be lowercase letters, digits, '_', '+' or '-'")
		} else if j, dup := seen[r.Name]; dup {
			v.add(rp+"/name", "duplicate reaction %q (also %s/%d)", r.Name, at, j)
		} else {
			seen[r.Name] = i
		}
		if len(r.Users) == 0 {
			v.add(rp+"/users", "need at least one user")
		}
		users := map[string]bool{}
		for k, u := range r.Users {
			up := rp + "/users/" + strconv.Itoa(k)
			if users[u] {
				v.add(up, "duplicate user %q", u)
			} else {
				v.user(up, u)
			}
			users[u] = true
		}
	}
}

func (v *validator) content(at, name string, raw json.RawMessage, open byte, kind string) {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || string(t) == "null" || t[0] == open {
		return
	}
	v.add(at, "%s must be a JSON %s", name, kind)
}

func (v *validator) generate(fx Fixture) {
	if fx.Generate == nil {
		return
	}
	if len(fx.Users)+len(fx.Channels)+len(fx.DMs)+len(fx.Messages) > 0 {
		v.add("/generate", "generate cannot be combined with users, channels, dms or messages")
	}
	v.issues = append(v.issues, fx.Generate.Issues("/generate")...)
}
