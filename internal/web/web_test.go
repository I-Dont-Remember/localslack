package web_test

import (
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"

	"localslack/internal/constants"
	"localslack/internal/service"
	"localslack/internal/storetest"
)

type browser struct {
	t    *testing.T
	app  *storetest.App
	http *http.Client
}

// newBrowser is a cookie-keeping client that does not follow redirects.
func newBrowser(t *testing.T, app *storetest.App) *browser {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	c := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &browser{t: t, app: app, http: c}
}

func (b *browser) do(method, path string, form url.Values) (int, string, http.Header) {
	b.t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(b.t.Context(), method, b.app.URL+path, body)
	if err != nil {
		b.t.Fatalf("build request: %v", err)
	}
	if form != nil {
		req.Header.Set("Content-Type", constants.ContentTypeForm)
	}
	resp, err := b.http.Do(req)
	if err != nil {
		b.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		b.t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(data), resp.Header
}

func (b *browser) get(path string) (int, string, http.Header) { return b.do(http.MethodGet, path, nil) }

func (b *browser) post(path string, form url.Values) (int, string, http.Header) {
	return b.do(http.MethodPost, path, form)
}

type world struct {
	human, other service.User
	general      string
}

func setup(t *testing.T, app *storetest.App) world {
	t.Helper()
	us, err := app.Service.Users(t.Context())
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	var w world
	for _, u := range us {
		switch {
		case !u.IsBot && w.human.ID == "":
			w.human = u
		case w.other.ID == "" && w.human.ID != "":
			w.other = u
		}
	}
	page, err := app.Service.ListConversations(t.Context(), w.human, service.ListOpts{Limit: 1000})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, c := range page.Items {
		if c.Name == "general" {
			w.general = c.ID
		}
	}
	if w.general == "" || w.other.ID == "" {
		t.Fatalf("unexpected seed: %+v", w)
	}
	return w
}

func contains(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

func TestRootRedirectsToGeneral(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	w := setup(t, app)
	status, _, hdr := newBrowser(t, app).get("/")
	if status != http.StatusFound || hdr.Get("Location") != "/ui/c/"+w.general {
		t.Errorf("GET / = %d -> %q, want 302 -> /ui/c/%s", status, hdr.Get("Location"), w.general)
	}
}

func TestChannelPage(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	w := setup(t, app)
	hist, err := app.Service.History(t.Context(), w.human, w.general, service.PageOpts{Limit: 1})
	if err != nil || len(hist.Items) == 0 {
		t.Fatalf("history: %+v, %v", hist, err)
	}
	status, body, _ := newBrowser(t, app).get("/ui/c/" + w.general)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	contains(t, body,
		"#general",                              // sidebar
		html.EscapeString(hist.Items[0].Text),   // seeded message
		`/ui/static/htmx.min.js`,                // vendored htmx, no CDN
		`data-testid="composer-input"`,          // composer
		`hx-trigger="every 2s"`,                 // polling
		`hx-get="/ui/c/`+w.general+`/messages"`, // poll target
		`data-testid="acting-as"`,
	)
	if strings.Contains(body, "http://") && strings.Contains(body, "cdn") {
		t.Error("page must not load assets from a CDN")
	}
}

func TestUnknownChannelIs404(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	setup(t, app)
	b := newBrowser(t, app)
	for _, path := range []string{"/ui/c/CNOPE", "/ui/c/CNOPE/messages", "/ui/c/CNOPE/t/1.000001"} {
		if status, _, _ := b.get(path); status != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, status)
		}
	}
}

func TestPostFromUIIsVisibleToService(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	w := setup(t, app)
	b := newBrowser(t, app)

	status, frag, _ := b.post("/ui/c/"+w.general+"/post", url.Values{"text": {"hello from <b>ui</b>"}})
	if status != http.StatusOK {
		t.Fatalf("post status = %d", status)
	}
	contains(t, frag, html.EscapeString("hello from <b>ui</b>"))
	if strings.Contains(frag, "<b>ui</b>") {
		t.Error("message text must be HTML-escaped")
	}

	hist, err := app.Service.History(t.Context(), w.human, w.general, service.PageOpts{Limit: 1})
	if err != nil || hist.Items[0].Text != "hello from <b>ui</b>" || hist.Items[0].UserID != w.human.ID {
		t.Errorf("history top = %+v, %v; want UI post authored by default human %s", hist.Items, err, w.human.ID)
	}

	before := hist.Items[0].TS
	if status, _, _ := b.post("/ui/c/"+w.general+"/post", url.Values{"text": {"  "}}); status != http.StatusOK {
		t.Errorf("blank post status = %d, want 200 (ignored)", status)
	}
	if after, _ := app.Service.History(t.Context(), w.human, w.general, service.PageOpts{Limit: 1}); after.Items[0].TS != before {
		t.Error("blank post created a message")
	}
}

func TestSDKPostAppearsInPolledFragment(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	w := setup(t, app)
	if _, err := app.Service.PostMessage(t.Context(), w.other, w.general, "from the sdk side", ""); err != nil {
		t.Fatalf("post: %v", err)
	}
	status, frag, _ := newBrowser(t, app).get("/ui/c/" + w.general + "/messages")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	contains(t, frag, "from the sdk side", w.other.Name)
}

func TestThreads(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	w := setup(t, app)
	b := newBrowser(t, app)
	root, err := app.Service.PostMessage(t.Context(), w.other, w.general, "thread root", "")
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	threadPath := "/ui/c/" + w.general + "/t/" + root.TS

	status, frag, _ := b.post(threadPath+"/post", url.Values{"text": {"first reply"}})
	if status != http.StatusOK {
		t.Fatalf("reply status = %d", status)
	}
	contains(t, frag, "thread root", "first reply")

	_, page, _ := b.get(threadPath)
	contains(t, page, `data-testid="thread-pane"`, "first reply", `data-testid="reply-input"`)

	_, msgs, _ := b.get("/ui/c/" + w.general + "/messages")
	contains(t, msgs, "1 reply", threadPath)
	if strings.Count(msgs, "first reply") != 0 {
		t.Error("channel view must not inline thread replies")
	}

	if status, _, _ := b.get("/ui/c/" + w.general + "/t/1.000001"); status != http.StatusNotFound {
		t.Errorf("missing thread status = %d, want 404", status)
	}
}

func TestQuickSwitcherAndDMs(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	w := setup(t, app)
	b := newBrowser(t, app)

	_, chans, _ := b.get("/ui/switch?q=gen")
	contains(t, chans, "#general", `href="/ui/c/`+w.general+`"`)

	_, people, _ := b.get("/ui/switch?q=" + url.QueryEscape(w.other.Name))
	contains(t, people, w.other.Name, `hx-post="/ui/dm"`, w.other.ID)

	_, none, _ := b.get("/ui/switch?q=zzzz-no-such-thing")
	if strings.Contains(none, "data-testid=\"switcher-result\"") {
		t.Errorf("no-match query returned results: %s", none)
	}

	status, _, hdr := b.post("/ui/dm", url.Values{"user": {w.other.ID}})
	loc := hdr.Get("HX-Redirect")
	if status != http.StatusOK || !strings.HasPrefix(loc, "/ui/c/D") {
		t.Fatalf("open dm = %d HX-Redirect %q, want 200 -> /ui/c/D...", status, loc)
	}
	if _, again, h2 := b.post("/ui/dm", url.Values{"user": {w.other.ID}}); h2.Get("HX-Redirect") != loc {
		t.Errorf("reopen redirect = %q (%s), want %q", h2.Get("HX-Redirect"), again, loc)
	}
	if status, _, _ := b.post("/ui/dm", url.Values{"user": {"UNOPE"}}); status != http.StatusNotFound {
		t.Errorf("dm with unknown user = %d, want 404", status)
	}

	_, page, _ := b.get(loc)
	contains(t, page, `data-testid="dm-link"`, w.other.Name)
}

func TestActingAsCookie(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	w := setup(t, app)
	b := newBrowser(t, app)

	status, _, hdr := b.post("/ui/as", url.Values{"user": {w.other.ID}, "next": {"/ui/c/" + w.general}})
	if status != http.StatusSeeOther || hdr.Get("Location") != "/ui/c/"+w.general {
		t.Fatalf("as = %d -> %q, want 303 back to channel", status, hdr.Get("Location"))
	}
	b.post("/ui/c/"+w.general+"/post", url.Values{"text": {"as someone else"}})
	hist, err := app.Service.History(t.Context(), w.human, w.general, service.PageOpts{Limit: 1})
	if err != nil || hist.Items[0].UserID != w.other.ID {
		t.Errorf("author = %+v, %v; want %s", hist.Items, err, w.other.ID)
	}
	if status, _, _ := b.post("/ui/as", url.Values{"user": {"UNOPE"}}); status != http.StatusBadRequest {
		t.Errorf("as unknown user = %d, want 400", status)
	}
	// Open redirects are refused.
	if _, _, h := b.post("/ui/as", url.Values{"user": {w.human.ID}, "next": {"https://evil.example/"}}); h.Get("Location") != "/" {
		t.Errorf("external next -> %q, want /", h.Get("Location"))
	}
}

func TestStaticHTMX(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	status, body, hdr := newBrowser(t, app).get("/ui/static/htmx.min.js")
	if status != http.StatusOK || !strings.Contains(body, "htmx") || !strings.Contains(hdr.Get("Content-Type"), "javascript") {
		t.Errorf("static htmx = %d %q (%d bytes)", status, hdr.Get("Content-Type"), len(body))
	}
}
