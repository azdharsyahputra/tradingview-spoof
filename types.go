package tvspoof

// QuoteUpdate represents a real-time market data update from TradingView.
//
// Because TradingView sends partial updates (e.g., only the price changes
// while volume stays the same), all fields are pointers. A nil field means
// that specific value was not included in this particular update.
type QuoteUpdate struct {
	// Symbol is the full TradingView symbol identifier (e.g., "OANDA:XAUUSD").
	Symbol string

	// Price is the last traded price (field: "lp").
	Price *float64

	// Volume is the current trading volume (field: "volume").
	Volume *float64

	// Bid is the current best bid price (field: "bid").
	Bid *float64

	// Ask is the current best ask price (field: "ask").
	Ask *float64

	// Open is the current session open price (field: "open_price").
	Open *float64

	// High is the current session high price (field: "high_price").
	High *float64

	// Low is the current session low price (field: "low_price").
	Low *float64

	// PrevClose is the previous session close price (field: "prev_close_price").
	PrevClose *float64

	// Change is the absolute price change (field: "ch").
	Change *float64

	// ChangePercent is the percentage price change (field: "chp").
	ChangePercent *float64
}

// OnQuoteFunc is the callback signature for receiving market data updates.
type OnQuoteFunc func(update QuoteUpdate)

// OnErrorFunc is the callback signature for receiving error notifications.
type OnErrorFunc func(err error)

// tvMessage represents the top-level JSON frame from TradingView's WebSocket protocol.
type tvMessage struct {
	M string        `json:"m"` // Method (e.g., "qsd" for Quote Session Data)
	P []interface{} `json:"p"` // Payload array: [session_id, data_object]
}

// quoteData represents the data object nested inside a "qsd" payload.
type quoteData struct {
	N string                 `json:"n"` // Ticker name (e.g., "OANDA:XAUUSD")
	S string                 `json:"s"` // Status (e.g., "ok")
	V map[string]interface{} `json:"v"` // Values map containing price, volume, etc.
}
