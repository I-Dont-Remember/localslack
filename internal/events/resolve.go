package events

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// DefaultPath is the Bolt (and Slack SDK) default events path, used when a
// host-only deliver_to meets a manifest without a request_url.
const DefaultPath = "/slack/events"

// URLPolicy is the global part of request URL resolution (docs/TESTING_API.md,
// "Where events go").
type URLPolicy struct {
	// DeliverTo is the default base URL for every app.
	DeliverTo string
	// AllowRemote permits non-loopback/private hosts.
	AllowRemote bool
}

// Target is where one app's events go, or why they go nowhere.
type Target struct {
	URL     string
	Blocked bool
	Reason  string
}

// Resolve picks the request URL: per-app deliver_to, else the global one, else
// the manifest's request_url. A host-only deliver_to keeps the manifest path.
// Remote hosts are blocked unless the policy allows them. Redirects are never
// followed (see HTTPTransport).
func (p URLPolicy) Resolve(manifestURL, appDeliverTo string) Target {
	deliverTo, src := appDeliverTo, "deliver_to"
	if deliverTo == "" {
		deliverTo = p.DeliverTo
	}
	if deliverTo != "" {
		target, err := applyDeliverTo(manifestURL, deliverTo)
		if err != nil {
			return Target{Blocked: true, Reason: err.Error()}
		}
		return p.checkHost(target, src)
	}
	if manifestURL == "" {
		return Target{Blocked: true, Reason: "no request_url in the manifest and no deliver_to"}
	}
	return p.checkHost(manifestURL, "manifest request_url")
}

func (p URLPolicy) checkHost(raw, src string) Target {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return Target{Blocked: true, Reason: fmt.Sprintf("%s %q is not a URL", src, raw)}
	}
	if p.AllowRemote || isLocalHost(u.Hostname()) {
		return Target{URL: raw}
	}
	hint := "set deliver_to or allow remote request URLs"
	if src == "deliver_to" {
		hint = "allow remote request URLs to use it"
	}
	return Target{Blocked: true, Reason: fmt.Sprintf("%s host %q is not loopback/private; %s", src, u.Hostname(), hint)}
}

func applyDeliverTo(manifestURL, deliverTo string) (string, error) {
	d, err := url.Parse(deliverTo)
	if err != nil || !ValidDeliverTo(deliverTo) {
		return "", fmt.Errorf("deliver_to %q is not an http(s) URL", deliverTo)
	}
	if d.Path != "" && d.Path != "/" {
		return deliverTo, nil
	}
	out := url.URL{Scheme: d.Scheme, Host: d.Host, Path: DefaultPath}
	if m, err := url.Parse(manifestURL); err == nil && m.Path != "" {
		out.Path, out.RawQuery = m.Path, m.RawQuery
	}
	return out.String(), nil
}

// ValidDeliverTo reports whether s is an absolute http(s) URL with a host.
func ValidDeliverTo(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func isLocalHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified())
}
