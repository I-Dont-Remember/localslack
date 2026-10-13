package scenario_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/scenario"
)

const jsonDoc = `{"users":[{"id":"U1","name":"ann"}],"channels":[{"id":"C1","name":"general","members":["U1"]}],
"messages":[{"channel":"C1","ts":"1.000001","user":"U1","text":"hi","blocks":[{"type":"divider"}],"reactions":[{"name":"eyes","users":["U1"]}]}]}`

const yamlDoc = `
users:
  - {id: U1, name: ann}
channels:
  - {id: C1, name: general, members: [U1]}
messages:
  - channel: C1
    ts: "1.000001"
    user: U1
    text: hi
    blocks: [{type: divider}]
    reactions: [{name: eyes, users: [U1]}]
`

const tomlDoc = `
[[users]]
id = "U1"
name = "ann"
[[channels]]
id = "C1"
name = "general"
members = ["U1"]
[[messages]]
channel = "C1"
ts = "1.000001"
user = "U1"
text = "hi"
blocks = [{type = "divider"}]
reactions = [{name = "eyes", users = ["U1"]}]
`

func wantParsed() scenario.Fixture {
	return scenario.Fixture{
		Version:  1,
		Users:    []scenario.User{{ID: "U1", Name: "ann"}},
		Channels: []scenario.Channel{{ID: "C1", Name: "general", Members: []string{"U1"}}},
		Messages: []scenario.Message{{
			Channel: "C1", TS: "1.000001", User: "U1", Text: "hi",
			Blocks:    json.RawMessage(`[{"type":"divider"}]`),
			Reactions: []scenario.Reaction{{Name: "eyes", Users: []string{"U1"}}},
		}},
	}
}

func TestParseFormatsAgree(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		format scenario.Format
		doc    string
	}{
		{"json", scenario.JSON, jsonDoc},
		{"yaml", scenario.YAML, yamlDoc},
		{"toml", scenario.TOML, tomlDoc},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := scenario.Parse([]byte(tt.doc), tt.format)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if diff := cmp.Diff(wantParsed(), got); diff != "" {
				t.Errorf("fixture (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		doc        string
		wantSyntax bool
		want       []scenario.Issue
	}{
		{name: "syntax", doc: `{`, wantSyntax: true},
		{name: "not an object", doc: `[1]`, wantSyntax: true},
		{name: "unknown field", doc: `{"usrs":[]}`, want: []scenario.Issue{{Pointer: "/usrs", Message: "unknown field usrs"}}},
		{
			name: "computed fields and unknown nested, all reported",
			doc:  `{"messages":[{"channel":"C1","reply_count":2},{"channel":"C1","latest_reply":"1.000001","nope":1}]}`,
			want: []scenario.Issue{
				{Pointer: "/messages/0/reply_count", Message: "reply_count is computed from the thread; remove it"},
				{Pointer: "/messages/1/latest_reply", Message: "latest_reply is computed from the thread; remove it"},
				{Pointer: "/messages/1/nope", Message: "unknown field nope"},
			},
		},
		{name: "wrong type", doc: `{"users":[{"name":5}]}`, want: []scenario.Issue{{Pointer: "/users/name", Message: "want string, got number"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := scenario.Parse([]byte(tt.doc), scenario.JSON)
			if tt.wantSyntax {
				if !errors.Is(err, scenario.ErrSyntax) {
					t.Fatalf("err = %v, want ErrSyntax", err)
				}
				return
			}
			var se *scenario.Error
			if !errors.As(err, &se) {
				t.Fatalf("err = %v, want *scenario.Error", err)
			}
			if diff := cmp.Diff(tt.want, se.Issues); diff != "" {
				t.Errorf("issues (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoadFileByExtension(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for name, doc := range map[string]string{"a.json": jsonDoc, "a.yml": yamlDoc, "a.yaml": yamlDoc, "a.toml": tomlDoc} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(doc), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"a.json", "a.yml", "a.yaml", "a.toml"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := scenario.LoadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("LoadFile: %v", err)
			}
			if diff := cmp.Diff(wantParsed(), got); diff != "" {
				t.Errorf("fixture (-want +got):\n%s", diff)
			}
		})
	}
	if _, err := scenario.LoadFile(filepath.Join(dir, "a.txt")); err == nil {
		t.Error("LoadFile(.txt) = nil, want extension error")
	}
	if _, err := scenario.LoadFile(filepath.Join(dir, "missing.json")); err == nil {
		t.Error("LoadFile(missing) = nil, want error")
	}
}
