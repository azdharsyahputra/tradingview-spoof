package tvspoof

import "time"

// Option is a functional option for configuring the Client.
type Option func(*Client)

// WithAutoReconnect enables or disables automatic reconnection on disconnect.
// Default: true.
func WithAutoReconnect(enabled bool) Option {
	return func(c *Client) {
		c.autoReconnect = enabled
	}
}

// WithReconnectDelay sets the initial delay before the first reconnect attempt.
// Subsequent attempts use exponential backoff up to MaxReconnectWait.
// Default: 3 seconds.
func WithReconnectDelay(d time.Duration) Option {
	return func(c *Client) {
		c.reconnectDelay = d
	}
}

// WithMaxReconnectWait sets the maximum wait time between reconnect attempts.
// Default: 60 seconds.
func WithMaxReconnectWait(d time.Duration) Option {
	return func(c *Client) {
		c.maxReconnectWait = d
	}
}

// WithOrigin sets the HTTP Origin header sent during the WebSocket handshake.
// Default: "https://www.tradingview.com".
func WithOrigin(origin string) Option {
	return func(c *Client) {
		c.origin = origin
	}
}

// WithUserAgent sets the User-Agent header sent during the WebSocket handshake.
// Default: latest Chrome on Windows 10.
func WithUserAgent(ua string) Option {
	return func(c *Client) {
		c.userAgent = ua
	}
}

// WithQuoteFields sets the market data fields to subscribe to.
// Default: ["lp", "volume", "bid", "ask", "open_price", "high_price", "low_price", "prev_close_price", "ch", "chp"].
func WithQuoteFields(fields []string) Option {
	return func(c *Client) {
		c.quoteFields = fields
	}
}
