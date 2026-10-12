package api

import (
	"encoding/json"

	"localslack/internal/service"
)

// addContent adds the rich-content fields (subtype, blocks, attachments, metadata).
func addContent(out map[string]any, m service.Message) {
	if m.Subtype != "" {
		out["subtype"] = m.Subtype
	}
	for k, raw := range map[string]json.RawMessage{"blocks": m.Blocks, "attachments": m.Attachments, "metadata": m.Metadata} {
		if len(raw) > 0 {
			out[k] = raw
		}
	}
}

// addEdited adds the edited marker.
func addEdited(out map[string]any, m service.Message) {
	if m.Edited != nil {
		out["edited"] = map[string]any{"user": m.Edited.User, "ts": m.Edited.TS}
	}
}
