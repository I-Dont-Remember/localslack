package events_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"localslack/internal/events"
)

func TestSignature(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	body := []byte(`{"a":1}`)
	mac := hmac.New(sha256.New, []byte("s3cret"))
	mac.Write([]byte("v0:1700000000:" + string(body)))
	valid := "v0=" + hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name          string
		mode          string
		stale         int
		wantTS        string
		wantSig       string
		wantSigFormat bool
	}{
		{name: "valid", mode: events.SigValid, wantTS: "1700000000", wantSig: valid},
		{name: "empty mode is valid", mode: "", wantTS: "1700000000", wantSig: valid},
		{name: "missing", mode: events.SigMissing, wantTS: "", wantSig: ""},
		{name: "invalid", mode: events.SigInvalid, wantTS: "1700000000", wantSigFormat: true},
		{name: "stale", mode: events.SigStale, stale: 600, wantTS: "1699999400", wantSigFormat: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ts, sig := events.SignRequest("s3cret", tt.mode, tt.stale, now, body)
			if ts != tt.wantTS {
				t.Errorf("timestamp = %q, want %q", ts, tt.wantTS)
			}
			switch {
			case tt.wantSigFormat:
				if len(sig) != len(valid) || sig[:3] != "v0=" {
					t.Errorf("signature = %q, want v0=<64 hex>", sig)
				}
				if tt.mode == events.SigInvalid && sig == valid {
					t.Error("invalid signature equals the valid one")
				}
			case sig != tt.wantSig:
				t.Errorf("signature = %q, want %q", sig, tt.wantSig)
			}
		})
	}
}

func TestStaleSignatureVerifiesAgainstItsOwnTimestamp(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	body := []byte("x")
	ts, sig := events.SignRequest("k", events.SigStale, 600, now, body)
	if want := events.Sign("k", ts, body); sig != want {
		t.Errorf("stale signature = %q, want a valid signature over the old timestamp %q", sig, want)
	}
}
