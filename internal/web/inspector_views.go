package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"localslack/internal/calllog"
	"localslack/internal/control"
	"localslack/internal/events"
	"localslack/internal/faults"
	"localslack/internal/service"
)

const timeLayout = "15:04:05.000"

// prettyJSON indents v for display; unmarshalable values fall back to %v.
func prettyJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

type callView struct {
	Seq            int64
	Time           string
	Method         string
	Status         int
	OK             bool
	Error          string
	DurationMS     int64
	Actor          string
	FaultID        string
	NotImplemented bool
	Params         string
	Response       string
	Truncated      bool
}

type callsView struct {
	Missing bool
	Rows    []callView
}

// newCallsView renders records (oldest first) newest first.
func newCallsView(recs []calllog.CallRecord) callsView {
	v := callsView{Rows: make([]callView, 0, len(recs))}
	for _, r := range slices.Backward(recs) {
		v.Rows = append(v.Rows, callView{
			Seq: r.Seq, Time: r.Time.UTC().Format(timeLayout), Method: r.Method, Status: r.Status, OK: r.OK,
			Error: r.Error, DurationMS: r.DurationMS, Actor: actorLabel(r.Actor), FaultID: r.FaultID,
			NotImplemented: r.NotImplemented, Params: prettyJSON(r.Params), Response: prettyJSON(r.Response),
			Truncated: r.ResponseTruncated,
		})
	}
	return v
}

func actorLabel(a calllog.Actor) string {
	switch {
	case a.AppID != "":
		return a.AppID
	case a.UserID != "":
		return a.UserID + " (" + a.TokenKind + ")"
	case a.TokenKind != "" && a.TokenKind != "none":
		return a.TokenKind
	}
	return "-"
}

type attemptView struct {
	N             int
	Time          string
	Status        int
	LatencyMS     int64
	Error         string
	RetryReason   string
	SignatureMode string
}

type deliveryView struct {
	ID, EventID, EventType, AppID, Transport, State string
	Injected                                        bool
	Attempts                                        []attemptView
	Payload                                         string
}

type deliveriesView struct {
	Missing bool
	Message string
	Rows    []deliveryView
}

// newDeliveriesView renders deliveries (oldest first) newest first, keeping at
// most limit rows; eventID, when set, keeps only that event's deliveries.
func newDeliveriesView(items []events.Delivery, eventID string, limit int, msg string) deliveriesView {
	v := deliveriesView{Message: msg, Rows: []deliveryView{}}
	for _, d := range slices.Backward(items) {
		if len(v.Rows) == limit {
			break
		}
		if eventID != "" && d.EventID != eventID {
			continue
		}
		row := deliveryView{
			ID: d.ID, EventID: d.EventID, EventType: d.EventType, AppID: d.AppID, Transport: d.Transport,
			State: d.State, Injected: d.Injected, Payload: indentRaw(d.Payload),
		}
		for _, a := range d.Attempts {
			row.Attempts = append(row.Attempts, attemptView{
				N: a.N, Time: a.At.UTC().Format(timeLayout), Status: a.Status, LatencyMS: a.LatencyMS,
				Error: a.Error, RetryReason: a.RetryReason, SignatureMode: a.SignatureMode,
			})
		}
		v.Rows = append(v.Rows, row)
	}
	return v
}

func indentRaw(raw json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return string(raw)
	}
	return buf.String()
}

type appView struct {
	ID, Name, BotUserID, BotID        string
	Ready                             bool
	BlockedReason                     string
	EffectiveURL, ManifestURL         string
	DeliverTo                         string
	SocketMode, Verified              bool
	VerifyError                       string
	Events, Scopes                    []string
	BotToken, AppToken, SigningSecret string
	VerificationToken                 string
}

type appsView struct {
	Message string
	Rows    []appView
}

// newAppsView resolves each app's delivery target with ctl (nil: assume ready).
func newAppsView(apps []service.App, ctl events.Controller, msg string) appsView {
	v := appsView{Message: msg, Rows: make([]appView, 0, len(apps))}
	for _, a := range apps {
		row := appView{
			ID: a.ID, Name: a.Name, BotUserID: a.BotUserID, BotID: a.BotID, Ready: true, ManifestURL: a.RequestURL,
			DeliverTo: a.DeliverTo, SocketMode: a.SocketMode, Verified: a.Verified, Events: a.BotEvents, Scopes: a.Scopes,
			BotToken: a.BotToken, AppToken: a.AppToken, SigningSecret: a.SigningSecret, VerificationToken: a.VerificationToken,
		}
		if ctl != nil && !a.SocketMode {
			if t := ctl.Resolve(a); t.Blocked {
				row.Ready, row.BlockedReason = false, t.Reason
			} else {
				row.EffectiveURL = t.URL
			}
			row.VerifyError = ctl.VerifyError(a.ID)
		}
		v.Rows = append(v.Rows, row)
	}
	return v
}

type faultView struct {
	ID, Method, Match, Limits, Action string
	Seen, Fired                       int
	Spent                             bool
}

type faultsView struct {
	Missing bool
	Message string
	Rows    []faultView
}

func newFaultsView(rules []faults.Rule, msg string) faultsView {
	v := faultsView{Message: msg, Rows: make([]faultView, 0, len(rules))}
	for _, r := range rules {
		v.Rows = append(v.Rows, faultView{
			ID: r.ID, Method: r.Method, Match: matchLabel(r.Match), Limits: limitsLabel(r.Spec), Action: actionLabel(r.Action),
			Seen: r.Seen, Fired: r.Fired, Spent: r.Spent,
		})
	}
	return v
}

func matchLabel(m faults.Match) string {
	return joinKV(kv{"channel", m.Channel}, kv{"app_id", m.AppID}, kv{"user_id", m.UserID}, kv{"token_kind", m.TokenKind})
}

func limitsLabel(s faults.Spec) string {
	once := ""
	if s.Once {
		once = "true"
	}
	return joinKV(kv{"nth", itoa(s.Nth)}, kv{"every", itoa(s.Every)}, kv{"times", itoa(s.Times)}, kv{"once", once})
}

func actionLabel(a faults.Action) string {
	return joinKV(kv{"status", itoa(a.Status)}, kv{"error", a.Error}, kv{"retry_after", itoa(a.RetryAfter)}, kv{"delay_ms", itoa(a.DelayMS)})
}

type kv struct{ k, v string }

// joinKV renders the non-empty pairs as "k=v k=v".
func joinKV(pairs ...kv) string {
	var parts []string
	for _, p := range pairs {
		if p.v != "" {
			parts = append(parts, p.k+"="+p.v)
		}
	}
	return strings.Join(parts, " ")
}

// itoa renders n, or "" for zero (unset).
func itoa(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

type controlsView struct {
	Missing       bool
	Error         string
	Signature     string
	Signatures    []string
	Paused        bool
	TimeoutMS     int64
	StaleSeconds  int
	EventsDropped int64
}

func newControlsView(c events.DeliveryConfig, dropped int64, errMsg string) controlsView {
	return controlsView{
		Error: errMsg, Signature: c.Signature, Signatures: []string{events.SigValid, events.SigInvalid, events.SigStale, events.SigMissing},
		Paused: c.Paused, TimeoutMS: c.TimeoutMS, StaleSeconds: c.StaleSeconds, EventsDropped: dropped,
	}
}

type resetView struct {
	Error                     string
	Mode, Seed                string
	Users, Channels, Messages int
	TookMS                    int64
}

func newResetView(r control.ResetResult) resetView {
	return resetView{
		Mode: r.Mode, Seed: r.Seed, Users: r.Counts.Users, Channels: r.Counts.Channels,
		Messages: r.Counts.Messages, TookMS: r.TookMS,
	}
}
