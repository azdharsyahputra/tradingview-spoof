package tvspoof

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
)

// Precompiled regex for splitting Engine.IO framed messages.
var framePattern = regexp.MustCompile(`~m~\d+~m~`)

// parseMessages splits a raw Engine.IO WebSocket frame into individual
// message strings by stripping the ~m~LEN~m~ framing.
func parseMessages(raw []byte) []string {
	parts := framePattern.Split(string(raw), -1)

	var messages []string
	for _, part := range parts {
		cleanPart := strings.TrimSpace(part)
		if cleanPart != "" {
			messages = append(messages, cleanPart)
		}
	}
	return messages
}

// processPayload handles a single decoded message from the WebSocket stream.
// It responds to heartbeats, reports server errors, and extracts quote data.
func (c *Client) processPayload(jsonStr string) {
	// 1. Handle heartbeat/ping (format: ~h~1, ~h~2, ...)
	// The server sends these periodically. If not echoed back, the connection
	// will be terminated by the server after a timeout (~10 seconds).
	if strings.HasPrefix(jsonStr, "~h~") {
		c.safeSend(jsonStr)
		return
	}

	// 2. Parse the JSON message envelope
	var msg tvMessage
	if err := json.Unmarshal([]byte(jsonStr), &msg); err != nil {
		// Silently ignore non-JSON messages (e.g., initial session ack)
		return
	}

	// 3. Handle server-side errors
	if msg.M == "critical_error" || msg.M == "error" {
		errMsg := fmt.Sprintf("tradingview server %s: %v", msg.M, msg.P)
		log.Printf("[tvspoof] %s", errMsg)
		if c.OnError != nil {
			c.OnError(fmt.Errorf("%s", errMsg))
		}
		return
	}

	// 4. Extract quote data from "qsd" (Quote Session Data) messages
	if msg.M != "qsd" || len(msg.P) < 2 {
		return
	}

	payloadBytes, err := json.Marshal(msg.P[1])
	if err != nil {
		return
	}

	var qData quoteData
	if err := json.Unmarshal(payloadBytes, &qData); err != nil {
		return
	}

	if qData.S != "ok" {
		return
	}

	update := QuoteUpdate{
		Symbol:        qData.N,
		Price:         extractFloat(qData.V, "lp"),
		Volume:        extractFloat(qData.V, "volume"),
		Bid:           extractFloat(qData.V, "bid"),
		Ask:           extractFloat(qData.V, "ask"),
		Open:          extractFloat(qData.V, "open_price"),
		High:          extractFloat(qData.V, "high_price"),
		Low:           extractFloat(qData.V, "low_price"),
		PrevClose:     extractFloat(qData.V, "prev_close_price"),
		Change:        extractFloat(qData.V, "ch"),
		ChangePercent: extractFloat(qData.V, "chp"),
	}

	if c.OnQuote != nil {
		c.OnQuote(update)
	}
}

// extractFloat safely extracts a float64 pointer from a map.
// Returns nil if the key does not exist or the value is not a number.
func extractFloat(m map[string]interface{}, key string) *float64 {
	if raw, exists := m[key]; exists {
		if val, ok := raw.(float64); ok {
			return &val
		}
	}
	return nil
}
