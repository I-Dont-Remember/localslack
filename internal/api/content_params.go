package api

import (
	"encoding/json"

	"localslack/internal/service"
)

// contentParams reads text/blocks/attachments/metadata from a call. Form posts
// carry the structured fields as JSON strings; JSON posts may carry them as
// decoded values (or strings, which Slack also accepts).
func contentParams(c *call) service.Content {
	return service.Content{
		Text:        c.params.Get("text"),
		Blocks:      jsonParam(c, "blocks"),
		Attachments: jsonParam(c, "attachments"),
		Metadata:    jsonParam(c, "metadata"),
	}
}

func jsonParam(c *call, key string) json.RawMessage {
	if v, ok := c.raw[key]; ok {
		if _, isStr := v.(string); !isStr {
			b, _ := json.Marshal(v) // re-encoding a decoded value cannot fail
			return b
		}
	}
	if s := c.params.Get(key); s != "" {
		return json.RawMessage(s)
	}
	return nil
}

// withoutMetadata drops metadata from message DTOs unless include_all_metadata is set.
func withoutMetadata(c *call, msgs []map[string]any) {
	if boolParam(c.params, "include_all_metadata") {
		return
	}
	for _, m := range msgs {
		delete(m, "metadata")
	}
}
