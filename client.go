package tvspoof

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultWSURL     = "wss://data.tradingview.com/socket.io/websocket"
	defaultOrigin    = "https://www.tradingview.com"
	defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"
)

var defaultQuoteFields = []string{
	"lp", "volume", "bid", "ask",
	"open_price", "high_price", "low_price", "prev_close_price",
	"ch", "chp",
}

// Client manages a real-time WebSocket connection to TradingView.
//
// It handles connection establishment with TLS fingerprint spoofing (Chrome),
// automatic heartbeat responses, symbol subscription management, and optional
// auto-reconnection with exponential backoff.
type Client struct {
	conn      *websocket.Conn
	connMu    sync.Mutex // protects concurrent writes to websocket
	sessionID string

	// Callbacks — set these before calling Connect().
	OnQuote OnQuoteFunc
	OnBar   OnBarFunc
	OnError OnErrorFunc

	// Configuration
	origin      string
	userAgent   string
	quoteFields []string

	// State management
	symbols          []string // subscribed symbols (preserved for reconnect)
	barSubscriptions []barSubscription
	done             chan struct{}
	closed           bool
	mu               sync.Mutex // protects internal state

	// Reconnect configuration
	autoReconnect    bool
	reconnectDelay   time.Duration
	maxReconnectWait time.Duration
}

// NewClient creates a new TradingView WebSocket client.
//
// Use functional options to customize behavior:
//
//	client := tvspoof.NewClient(
//	    tvspoof.WithAutoReconnect(true),
//	    tvspoof.WithOrigin("https://id.tradingview.com"),
//	)
func NewClient(opts ...Option) *Client {
	c := &Client{
		sessionID:        generateSessionID("qs_"),
		done:             make(chan struct{}),
		origin:           defaultOrigin,
		userAgent:        defaultUserAgent,
		quoteFields:      defaultQuoteFields,
		autoReconnect:    true,
		reconnectDelay:   3 * time.Second,
		maxReconnectWait: 60 * time.Second,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Connect establishes the WebSocket connection to TradingView.
//
// Set OnQuote and OnError callbacks before calling Connect.
// After a successful connection, use AddSymbol to start receiving data.
func (c *Client) Connect() error {
	c.mu.Lock()
	c.closed = false
	c.mu.Unlock()

	return c.dial()
}

// AddSymbol subscribes to real-time market data for a symbol.
//
// The symbol must follow TradingView's "EXCHANGE:TICKER" format.
// Examples: "OANDA:XAUUSD", "BINANCE:BTCUSDT", "NASDAQ:AAPL".
//
// Duplicate symbols are silently ignored.
func (c *Client) AddSymbol(symbol string) {
	c.mu.Lock()
	for _, s := range c.symbols {
		if s == symbol {
			c.mu.Unlock()
			return
		}
	}
	c.symbols = append(c.symbols, symbol)
	c.mu.Unlock()

	c.sendAddSymbol(symbol)
}

// RemoveSymbol unsubscribes from a previously subscribed symbol.
func (c *Client) RemoveSymbol(symbol string) {
	c.mu.Lock()
	for i, s := range c.symbols {
		if s == symbol {
			c.symbols = append(c.symbols[:i], c.symbols[i+1:]...)
			break
		}
	}
	c.mu.Unlock()

	payload := fmt.Sprintf(`{"m":"quote_remove_symbols","p":["%s","%s"]}`, c.sessionID, symbol)
	c.safeSend(payload)
}

// SubscribeBars subscribes to chart OHLCV updates for one symbol and
// resolution. Each update replaces the active candle or begins the next one.
// The subscription automatically reconnects with the Client.
func (c *Client) SubscribeBars(symbol string, interval string) error {
	symbol = strings.TrimSpace(symbol)
	interval = strings.TrimSpace(interval)
	if symbol == "" {
		return fmt.Errorf("symbol is required")
	}
	if interval == "" {
		return fmt.Errorf("interval is required")
	}

	c.mu.Lock()
	for _, subscription := range c.barSubscriptions {
		if subscription.symbol == symbol && subscription.interval == interval {
			c.mu.Unlock()
			return nil
		}
	}
	subscription := barSubscription{
		symbol:    symbol,
		interval:  interval,
		sessionID: generateSessionID("cs_"),
	}
	c.barSubscriptions = append(c.barSubscriptions, subscription)
	c.mu.Unlock()

	c.connMu.Lock()
	connected := c.conn != nil
	c.connMu.Unlock()
	if !connected {
		return nil
	}
	return c.startBarSubscription(subscription)
}

// Close gracefully shuts down the connection.
//
// After Close is called, auto-reconnect is disabled. To reconnect,
// create a new Client instance.
func (c *Client) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()

	select {
	case <-c.done:
	default:
		close(c.done)
	}

	c.connMu.Lock()
	defer c.connMu.Unlock()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// --- internal methods ---

func (c *Client) dial() error {
	dialer := websocket.Dialer{
		NetDialTLSContext: customUTLSDialer,
		EnableCompression: true,
	}

	headers := http.Header{
		"Origin":          {c.origin},
		"User-Agent":      {c.userAgent},
		"Accept-Language": {"en-US,en;q=0.9"},
		"Cache-Control":   {"no-cache"},
		"Pragma":          {"no-cache"},
	}

	conn, _, err := dialer.Dial(defaultWSURL, headers)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()

	go c.listen()

	// 1. Auth token (required, even for unauthorized access)
	c.safeSend(`{"m":"set_auth_token","p":["unauthorized_user_token"]}`)

	// 2. Set locale
	c.safeSend(`{"m":"set_locale","p":["en","US"]}`)

	// 3. Create quote session
	c.safeSend(fmt.Sprintf(`{"m":"quote_create_session","p":["%s"]}`, c.sessionID))

	// 4. Set requested data fields
	fields := `"` + strings.Join(c.quoteFields, `","`) + `"`
	c.safeSend(fmt.Sprintf(`{"m":"quote_set_fields","p":["%s",%s]}`, c.sessionID, fields))

	// 5. Re-subscribe all tracked symbols (important for reconnect)
	c.mu.Lock()
	symbols := make([]string, len(c.symbols))
	copy(symbols, c.symbols)
	c.mu.Unlock()

	for _, sym := range symbols {
		c.sendAddSymbol(sym)
	}
	c.resubscribeBars()

	return nil
}

func (c *Client) sendAddSymbol(symbol string) {
	payload := fmt.Sprintf(`{"m":"quote_add_symbols","p":["%s","%s"]}`, c.sessionID, symbol)
	c.safeSend(payload)
}

func (c *Client) resubscribeBars() {
	c.mu.Lock()
	subscriptions := make([]barSubscription, len(c.barSubscriptions))
	for index, subscription := range c.barSubscriptions {
		subscription.sessionID = generateSessionID("cs_")
		c.barSubscriptions[index].sessionID = subscription.sessionID
		subscriptions[index] = subscription
	}
	c.mu.Unlock()

	for _, subscription := range subscriptions {
		if err := c.startBarSubscription(subscription); err != nil && c.OnError != nil {
			c.OnError(err)
		}
	}
}

func (c *Client) startBarSubscription(subscription barSubscription) error {
	resolve := fmt.Sprintf("={\"symbol\":%q,\"adjustment\":\"splits\",\"session\":\"extended\"}", subscription.symbol)
	commands := []struct {
		method string
		params []interface{}
	}{
		{"chart_create_session", []interface{}{subscription.sessionID, ""}},
		{"switch_timezone", []interface{}{subscription.sessionID, "Etc/UTC"}},
		{"resolve_symbol", []interface{}{subscription.sessionID, "symbol_1", resolve}},
		{"create_series", []interface{}{subscription.sessionID, "s1", "s1", "symbol_1", subscription.interval, 2}},
	}
	for _, command := range commands {
		if err := c.sendCommand(command.method, command.params...); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) sendCommand(method string, params ...interface{}) error {
	payload, err := json.Marshal(map[string]interface{}{"m": method, "p": params})
	if err != nil {
		return err
	}
	return c.safeSend(string(payload))
}

// safeSend wraps a JSON payload with Engine.IO framing and sends it thread-safely.
func (c *Client) safeSend(jsonPayload string) error {
	msg := fmt.Sprintf("~m~%d~m~%s", len(jsonPayload), jsonPayload)

	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("connection is nil")
	}

	return c.conn.WriteMessage(websocket.TextMessage, []byte(msg))
}

func (c *Client) listen() {
	for {
		c.connMu.Lock()
		conn := c.conn
		c.connMu.Unlock()

		if conn == nil {
			return
		}

		_, msg, err := conn.ReadMessage()
		if err != nil {
			c.mu.Lock()
			isClosed := c.closed
			c.mu.Unlock()

			if isClosed {
				return
			}

			if c.OnError != nil {
				c.OnError(err)
			}

			if c.autoReconnect {
				c.reconnect()
			}
			return
		}

		messages := parseMessages(msg)
		for _, m := range messages {
			c.processPayload(m)
		}
	}
}

func (c *Client) reconnect() {
	delay := c.reconnectDelay
	for {
		c.mu.Lock()
		isClosed := c.closed
		c.mu.Unlock()

		if isClosed {
			return
		}

		log.Printf("[tvspoof] reconnecting in %v...", delay)
		time.Sleep(delay)

		// Fresh session for new connection
		c.sessionID = generateSessionID("qs_")

		if err := c.dial(); err != nil {
			log.Printf("[tvspoof] reconnect failed: %v", err)
			delay *= 2
			if delay > c.maxReconnectWait {
				delay = c.maxReconnectWait
			}
			continue
		}

		log.Println("[tvspoof] reconnected successfully")
		return
	}
}

func generateSessionID(prefix string) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 12)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return prefix + string(b)
}
