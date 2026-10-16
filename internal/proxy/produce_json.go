package proxy

import (
	"encoding/json"
	"fmt"
)

// produceJSON renders the internal node representation as a JSON array, which is
// what Sub-Store's "JSON" (ALL) producer returns. It is the lossless view of the
// parsed nodes and is useful for debugging and for clients that consume the
// internal schema.
func produceJSON(nodes []Node) (string, error) {
	if nodes == nil {
		// Emit [] rather than null for an empty subscription.
		nodes = []Node{}
	}
	raw, err := json.MarshalIndent(nodes, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal json output: %w", err)
	}
	return string(raw), nil
}
