package events

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"
)

// Signature modes for deliveries (docs/TESTING_API.md, delivery-config).
const (
	SigValid   = "valid"
	SigInvalid = "invalid"
	SigStale   = "stale"
	SigMissing = "missing"
)

// Sign returns Slack's v0 signature: "v0=" + hex(HMAC_SHA256(secret, "v0:"+ts+":"+body)).
func Sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("v0:" + timestamp + ":"))
	_, _ = mac.Write(body)
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

// SignRequest returns the X-Slack-Request-Timestamp and X-Slack-Signature
// values for mode. Both are empty for SigMissing. SigStale signs correctly but
// backdates the timestamp by staleSeconds; SigInvalid signs with the wrong key.
func SignRequest(secret, mode string, staleSeconds int, now time.Time, body []byte) (timestamp, signature string) {
	ts := now.Unix()
	switch mode {
	case SigMissing:
		return "", ""
	case SigStale:
		ts -= int64(staleSeconds)
	case SigInvalid:
		secret = "invalid:" + secret
	}
	timestamp = strconv.FormatInt(ts, 10)
	return timestamp, Sign(secret, timestamp, body)
}
