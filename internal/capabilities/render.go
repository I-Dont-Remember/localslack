package capabilities

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Render produces docs/COMPATIBILITY.md. Output is deterministic so a golden test
// can catch stale or hand-edited copies.
func Render(r Report) string {
	var b strings.Builder
	m := r.Manifest
	b.WriteString("# Compatibility\n\n")
	b.WriteString("> Generated from `internal/capabilities/capabilities.toml`, the conformance fixtures and the smoke tests by `make docs`. Do not edit by hand.\n\n")
	b.WriteString("What LocalSlack supports, how well each piece is verified against real Slack, and where it knowingly differs. ")
	b.WriteString("Anything not listed as implemented returns `404 not_implemented`.\n\n")
	b.WriteString("**Scope:** LocalSlack deliberately covers a small part of Slack's API: the common surface Bolt and SDK apps use. ")
	b.WriteString("Many methods (admin.*, Enterprise Grid, paid-plan features, destructive operations) may never be recorded from real Slack; ")
	b.WriteString("this page says honestly what is verified and what is a best guess.\n\n")
	b.WriteString(summary(m, r) + "\n\n")
	b.WriteString("## How to read the evidence column\n\n")
	b.WriteString("| Level | Meaning |\n|---|---|\n")
	b.WriteString("| `recorded` | A sanitized response captured from real Slack is committed as a conformance fixture. |\n")
	b.WriteString("| `docs` | Checked against a cited source (Slack docs or SDK source). |\n")
	b.WriteString("| `llm` | Written from model knowledge and never checked. Treat as a best guess. |\n")
	b.WriteString("| `none` | No conformance fixture exists. |\n\n")
	b.WriteString("\"Red test\" in the deviations table means an executable strict-xfail test in `tests/smoke/known_deviations_test.py` encodes the real Slack behavior. ")
	b.WriteString("Tier and scopes are transcribed from Slack's docs and are unverified until a recording says otherwise. ")
	b.WriteString("\"SDK-called\" is a text scan: a Slack SDK client call to the method appears in `tests/smoke` or `tests/e2e`. It is a hint, not proof the behavior is verified.\n\n")

	renderShipped(&b, r)
	renderPlanned(&b, m)
	renderBehaviors(&b, m)
	renderDeviations(&b, r)
	renderClients(&b, r)
	return b.String()
}

func summary(m Manifest, r Report) string {
	counts := map[Status]int{}
	for _, meth := range m.Methods {
		counts[meth.Status]++
	}
	var parts []string
	for _, s := range []Status{StatusImplemented, StatusPartial, StatusInProgress, StatusPlanned} {
		if counts[s] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[s], s))
		}
	}
	ev := map[Evidence]int{}
	for name, meth := range m.Methods {
		if shipped(meth.Status) {
			ev[r.level(name)]++
		}
	}
	return fmt.Sprintf("**Methods:** %s. **Evidence of shipped methods:** %d recorded, %d docs, %d llm, %d none.",
		strings.Join(parts, ", "), ev[EvidenceRecorded], ev[EvidenceDocs], ev[EvidenceLLM], ev[EvidenceNone])
}

func renderShipped(b *strings.Builder, r Report) {
	b.WriteString("## Methods\n\n")
	b.WriteString("| Method | Status | Evidence | Tier | Scopes | SDK-called | Deviations |\n|---|---|---|---|---|---|---|\n")
	for _, name := range sortedKeys(r.Manifest.Methods) {
		meth := r.Manifest.Methods[name]
		if !shipped(meth.Status) {
			continue
		}
		sdk := "—"
		if len(r.SDK[name]) > 0 {
			sdk = "yes"
		}
		fmt.Fprintf(b, "| `%s` | %s | %s | %s | %s | %s | %s |\n",
			name, meth.Status, r.levelLabel(name), tier(meth.Tier), codes(meth.Scopes), sdk, codes(meth.Deviations))
	}
	b.WriteString("\n")
}

func renderPlanned(b *strings.Builder, m Manifest) {
	b.WriteString("### Planned and in progress\n\n")
	b.WriteString("| Method | Status | Priority | Tier | Scopes | Plan |\n|---|---|---|---|---|---|\n")
	order := func(name string) string { return m.Methods[name].Priority + "\x00" + name }
	names := sortedKeys(m.Methods)
	slices.SortFunc(names, func(a, c string) int { return strings.Compare(order(a), order(c)) })
	for _, name := range names {
		meth := m.Methods[name]
		if shipped(meth.Status) {
			continue
		}
		fmt.Fprintf(b, "| `%s` | %s | %s | %s | %s | %s |\n",
			name, meth.Status, dash(meth.Priority), tier(meth.Tier), codes(meth.Scopes), dash(meth.Plan))
	}
	b.WriteString("\n")
}

func renderBehaviors(b *strings.Builder, m Manifest) {
	b.WriteString("## Behaviors and quirks\n\n")
	b.WriteString("| Behavior | Status | Milestone | Evidence | Summary | Deviations |\n|---|---|---|---|---|---|\n")
	for _, name := range sortedKeys(m.Behaviors) {
		bh := m.Behaviors[name]
		ev := "—"
		if bh.Evidence != "" {
			ev = string(bh.Evidence)
		}
		fmt.Fprintf(b, "| `%s` | %s | %s | %s | %s | %s |\n",
			name, bh.Status, dash(bh.Milestone), ev, cell(bh.Summary), codes(bh.Deviations))
	}
	b.WriteString("\n")
}

func renderDeviations(b *strings.Builder, r Report) {
	m := r.Manifest
	b.WriteString("## Known deviations from real Slack\n\n")
	b.WriteString("| Deviation | Severity | What differs | Affects | Red test |\n|---|---|---|---|---|\n")
	affects := map[string][]string{}
	for name, meth := range m.Methods {
		for _, d := range meth.Deviations {
			affects[d] = append(affects[d], name)
		}
	}
	for name, bh := range m.Behaviors {
		for _, d := range bh.Deviations {
			affects[d] = append(affects[d], name)
		}
	}
	rank := map[string]int{"high": 0, "medium": 1, "low": 2}
	names := sortedKeys(m.Deviations)
	slices.SortStableFunc(names, func(a, c string) int { return rank[m.Deviations[a].Severity] - rank[m.Deviations[c].Severity] })
	for _, name := range names {
		d := m.Deviations[name]
		slices.Sort(affects[name])
		red := "—"
		if len(r.DevTests[name]) > 0 {
			red = "yes"
		}
		fmt.Fprintf(b, "| `%s` | %s | %s | %s | %s |\n", name, d.Severity, cell(d.Summary), codes(affects[name]), red)
	}
	b.WriteString("\n")
}

func renderClients(b *strings.Builder, r Report) {
	b.WriteString("## Clients verified\n\n")
	b.WriteString("Versions pinned by `tests/smoke/uv.lock`, i.e. what the smoke suite really runs.\n\n")
	b.WriteString("| Client | Version |\n|---|---|\n")
	for _, c := range r.Clients {
		v := c.Version
		if v == "" {
			v = "not yet"
		}
		fmt.Fprintf(b, "| %s | %s |\n", c.Name, v)
	}
}

func tier(t int) string {
	if t == 0 {
		return "—"
	}
	return strconv.Itoa(t)
}

func dash(s string) string {
	if s == "" {
		return "—"
	}
	return cell(s)
}

func codes(items []string) string {
	if len(items) == 0 {
		return "—"
	}
	quoted := make([]string, 0, len(items))
	for _, it := range items {
		quoted = append(quoted, "`"+it+"`")
	}
	return strings.Join(quoted, ", ")
}

// cell escapes text for a markdown table cell.
func cell(s string) string { return strings.ReplaceAll(s, "|", `\|`) }
