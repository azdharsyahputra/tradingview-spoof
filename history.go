package tvspoof

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Bar is one historical OHLCV candle returned by TradingView's chart session.
// Time is a Unix timestamp in seconds.
type Bar struct {
	Time   int64   `json:"time"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume,omitempty"`
}

// GetHistory requests the most recent candles through TradingView's chart
// session protocol. It uses the same uTLS/WebSocket dial path as Client.
//
// interval accepts TradingView resolutions such as "1", "15", "60", "240",
// "D", "W", and "M". This method opens a short-lived chart connection and
// does not affect quote subscriptions on the Client.
func (c *Client) GetHistory(ctx context.Context, symbol, interval string, bars int) ([]Bar, error) {
	if strings.TrimSpace(symbol) == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if strings.TrimSpace(interval) == "" {
		return nil, fmt.Errorf("interval is required")
	}
	if bars < 1 || bars > 10000 {
		return nil, fmt.Errorf("bars must be between 1 and 10000")
	}

	dialer := websocket.Dialer{
		NetDialTLSContext: customUTLSDialer,
		EnableCompression: true,
	}
	conn, _, err := dialer.DialContext(ctx, defaultWSURL, historyHeaders(c.origin, c.userAgent))
	if err != nil {
		return nil, fmt.Errorf("historical websocket dial failed: %w", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(30 * time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetReadDeadline(deadline)

	sessionID := generateSessionID("cs_")
	if err := sendHistoryCommand(conn, "set_auth_token", "unauthorized_user_token"); err != nil {
		return nil, err
	}
	if err := sendHistoryCommand(conn, "set_locale", "en", "US"); err != nil {
		return nil, err
	}
	if err := sendHistoryCommand(conn, "chart_create_session", sessionID, ""); err != nil {
		return nil, err
	}
	quoteSessionID := generateSessionID("qs_")
	if err := sendHistoryCommand(conn, "quote_create_session", quoteSessionID); err != nil {
		return nil, err
	}
	if err := sendHistoryCommand(conn, "quote_set_fields", quoteSessionID,
		"ch", "chp", "current_session", "description", "exchange", "lp", "lp_time", "volume"); err != nil {
		return nil, err
	}
	if err := sendHistoryCommand(conn, "quote_add_symbols", quoteSessionID, symbol); err != nil {
		return nil, err
	}
	if err := sendHistoryCommand(conn, "switch_timezone", sessionID, "Etc/UTC"); err != nil {
		return nil, err
	}

	// TradingView expects the symbol description as a JSON string inside the
	// command's parameter array.
	resolve := fmt.Sprintf(`={"symbol":%q,"adjustment":"splits","session":"extended"}`, symbol)
	if err := sendHistoryCommand(conn, "resolve_symbol", sessionID, "symbol_1", resolve); err != nil {
		return nil, err
	}
	if err := sendHistoryCommand(conn, "create_series", sessionID, "s1", "s1", "symbol_1", interval, bars, ""); err != nil {
		return nil, err
	}

	result := make(map[int64]Bar, bars)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("historical websocket read failed: %w", err)
		}
		for _, message := range parseMessages(raw) {
			if strings.HasPrefix(message, "~h~") {
				if err := writeFramed(conn, message); err != nil {
					return nil, err
				}
				continue
			}
			method, payload, err := decodeTVMessage(message)
			if err != nil {
				continue
			}
			switch method {
			case "critical_error", "protocol_error", "series_error", "error":
				return nil, fmt.Errorf("tradingview historical error: %s", message)
			case "timescale_update", "du":
				extractHistoryBars(payload, result)
				if len(result) >= bars {
					return sortedBars(result), nil
				}
			case "series_completed":
				if len(result) > 0 {
					return sortedBars(result), nil
				}
			}
		}
	}
}

func historyHeaders(origin, userAgent string) http.Header {
	return http.Header{
		"Origin":          {origin},
		"User-Agent":      {userAgent},
		"Accept-Language": {"en-US,en;q=0.9"},
		"Cache-Control":   {"no-cache"},
		"Pragma":          {"no-cache"},
	}
}

func sendHistoryCommand(conn *websocket.Conn, method string, params ...interface{}) error {
	payload := map[string]interface{}{"m": method, "p": params}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return writeFramed(conn, string(data))
}

func writeFramed(conn *websocket.Conn, payload string) error {
	frame := fmt.Sprintf("~m~%d~m~%s", len(payload), payload)
	return conn.WriteMessage(websocket.TextMessage, []byte(frame))
}

func decodeTVMessage(raw string) (string, []interface{}, error) {
	var message struct {
		M string        `json:"m"`
		P []interface{} `json:"p"`
	}
	if err := json.Unmarshal([]byte(raw), &message); err != nil {
		return "", nil, err
	}
	return message.M, message.P, nil
}

func extractHistoryBars(payload []interface{}, result map[int64]Bar) {
	if len(payload) < 2 {
		return
	}
	state, ok := payload[1].(map[string]interface{})
	if !ok {
		return
	}
	// Depending on the server rollout, bars arrive either directly under p[1]
	// (timescale_update) or nested under the series id (du).
	if series, ok := state["s"].([]interface{}); ok {
		extractSeriesBars(series, result)
	}
	for _, value := range state {
		nested, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		series, ok := nested["s"].([]interface{})
		if ok {
			extractSeriesBars(series, result)
		}
	}
}

func extractSeriesBars(series []interface{}, result map[int64]Bar) {
	for _, item := range series {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		values, ok := entry["v"].([]interface{})
		if !ok || len(values) < 5 {
			continue
		}
		timestamp, ok := numberAsInt64(values[0])
		if !ok || timestamp <= 0 {
			continue
		}
		o, ok1 := numberAsFloat(values[1])
		h, ok2 := numberAsFloat(values[2])
		l, ok3 := numberAsFloat(values[3])
		cl, ok4 := numberAsFloat(values[4])
		if !(ok1 && ok2 && ok3 && ok4) {
			continue
		}
		bar := Bar{Time: timestamp, Open: o, High: h, Low: l, Close: cl}
		if len(values) > 5 {
			bar.Volume, _ = numberAsFloat(values[5])
		}
		result[timestamp] = bar
	}
}

func sortedBars(result map[int64]Bar) []Bar {
	keys := make([]int64, 0, len(result))
	for key := range result {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	bars := make([]Bar, 0, len(keys))
	for _, key := range keys {
		bars = append(bars, result[key])
	}
	return bars
}

func numberAsFloat(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func numberAsInt64(value interface{}) (int64, bool) {
	parsed, ok := numberAsFloat(value)
	if !ok {
		return 0, false
	}
	return int64(parsed), true
}
