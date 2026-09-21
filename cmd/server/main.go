package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	tvspoof "github.com/azdharsyahputra/tradingview-spoof"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		// The service is intended for local Next.js development. Keep the
		// browser client usable on localhost while not exposing credentials.
		return true
	},
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", health)
	mux.HandleFunc("/api/history", history)
	mux.HandleFunc("/api/symbols", symbols)
	mux.HandleFunc("/api/calendar", calendar)
	mux.HandleFunc("/api/desk", desk)
	mux.HandleFunc("/api/plans", tradePlansHandler)
	mux.HandleFunc("/ws/quotes", quotes)
	mux.HandleFunc("/ws/bars", barsStream)

	addr := os.Getenv("TVSPOOF_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	server := &http.Server{Addr: addr, Handler: cors(mux), ReadHeaderTimeout: 10 * time.Second}
	log.Printf("tvspoof API listening on http://%s", addr)
	log.Printf("history: GET /api/history?symbol=OANDA%%3AXAUUSD&interval=15&bars=500")
	log.Printf("quotes:   WS  /ws/quotes?symbol=OANDA%%3AXAUUSD")
	log.Printf("bars:     WS  /ws/bars?symbol=OANDA%%3AXAUUSD&interval=15")
	log.Printf("calendar: GET /api/calendar?date=today&currency=USD")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "service": "tvspoof"})
}

func history(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	symbol := q.Get("symbol")
	interval := q.Get("interval")
	if interval == "" {
		interval = "15"
	}
	bars := 500
	if value := q.Get("bars"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			http.Error(w, "bars must be an integer", http.StatusBadRequest)
			return
		}
		bars = parsed
	}
	if symbol == "" {
		http.Error(w, "symbol is required", http.StatusBadRequest)
		return
	}

	timeout := 30 * time.Second
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	data, err := tvspoof.NewClient().GetHistory(ctx, symbol, interval, bars)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true, "symbol": symbol, "interval": interval, "bars": data,
	})
}

func symbols(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		http.Error(w, "q is required", http.StatusBadRequest)
		return
	}
	limit, err := tvspoof.ParseSearchLimit(r.URL.Query().Get("limit"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, err := tvspoof.SearchSymbols(r.Context(), query, r.URL.Query().Get("exchange"), limit)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": data})
}

func desk(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	symbol := q.Get("symbol")
	if symbol == "" {
		symbol = "OANDA:XAUUSD"
	}
	balance := 10000.0
	if val := q.Get("balance"); val != "" {
		if b, err := strconv.ParseFloat(val, 64); err == nil && b > 0 {
			balance = b
		}
	}
	risk := 1.0
	if val := q.Get("risk"); val != "" {
		if rk, err := strconv.ParseFloat(val, 64); err == nil && rk > 0 {
			risk = rk
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	client := tvspoof.NewClient()
	briefing, err := client.AnalyzeDesk(ctx, symbol, balance, risk)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "briefing": briefing})
}

func tradePlansHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		plans, err := tvspoof.LoadTradePlans("")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "plans": plans, "count": len(plans)})

	case http.MethodPost:
		var plan tvspoof.TradePlan
		if err := json.NewDecoder(r.Body).Decode(&plan); err != nil {
			http.Error(w, "invalid request body JSON", http.StatusBadRequest)
			return
		}
		if plan.Symbol == "" {
			http.Error(w, "symbol is required", http.StatusBadRequest)
			return
		}
		if err := tvspoof.UpsertTradePlan("", plan); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "trade plan saved", "plan": plan})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

var forexFactoryCalendarCache struct {
	sync.Mutex
	events    []tvspoof.CalendarEvent
	fetchedAt time.Time
	expiresAt time.Time
}

func calendar(w http.ResponseWriter, r *http.Request) {
	events, fetchedAt, err := cachedForexFactoryCalendar(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}

	query := r.URL.Query()
	currencies := csvFilter(query.Get("currency"))
	impacts := csvFilter(query.Get("impact"))
	var dayStart, dayEnd time.Time
	if value := query.Get("date"); value != "" {
		var err error
		dayStart, dayEnd, err = calendarDayRange(value, time.Now().UTC())
		if err != nil {
			http.Error(w, "date must be today or YYYY-MM-DD", http.StatusBadRequest)
			return
		}
	}
	limit := 0
	if value := query.Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			http.Error(w, "limit must be a positive integer", http.StatusBadRequest)
			return
		}
		limit = parsed
	}

	filtered := make([]tvspoof.CalendarEvent, 0, len(events))
	for _, event := range events {
		if !dayStart.IsZero() && (event.Timestamp < dayStart.Unix() || event.Timestamp >= dayEnd.Unix()) {
			continue
		}
		if len(currencies) > 0 && !currencies[strings.ToLower(event.Country)] {
			continue
		}
		if len(impacts) > 0 && !impacts[strings.ToLower(event.Impact)] {
			continue
		}
		filtered = append(filtered, event)
		if limit > 0 && len(filtered) >= limit {
			break
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":         true,
		"source":     "ForexFactory",
		"source_url": tvspoof.ForexFactoryCalendarURL,
		"fetched_at": fetchedAt,
		"events":     filtered,
	})
}

func calendarDayRange(value string, now time.Time) (time.Time, time.Time, error) {
	var start time.Time
	if value == "today" {
		current := now.UTC()
		start = time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, time.UTC)
	} else {
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		start = parsed.UTC()
	}
	return start, start.Add(24 * time.Hour), nil
}

func cachedForexFactoryCalendar(ctx context.Context) ([]tvspoof.CalendarEvent, time.Time, error) {
	forexFactoryCalendarCache.Lock()
	defer forexFactoryCalendarCache.Unlock()
	if time.Now().Before(forexFactoryCalendarCache.expiresAt) {
		return forexFactoryCalendarCache.events, forexFactoryCalendarCache.fetchedAt, nil
	}

	events, err := tvspoof.FetchForexFactoryCalendar(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	forexFactoryCalendarCache.events = events
	forexFactoryCalendarCache.fetchedAt = time.Now().UTC()
	forexFactoryCalendarCache.expiresAt = time.Now().Add(60 * time.Second)
	return events, forexFactoryCalendarCache.fetchedAt, nil
}

func csvFilter(value string) map[string]bool {
	result := make(map[string]bool)
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		result[strings.ToLower(item)] = true
	}
	return result
}

func quotes(w http.ResponseWriter, r *http.Request) {
	symbol := strings.TrimSpace(r.URL.Query().Get("symbol"))
	if symbol == "" {
		http.Error(w, "symbol is required", http.StatusBadRequest)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	var writeMu sync.Mutex
	latest := quoteSnapshot{Symbol: symbol}
	send := func(value interface{}) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(value)
	}

	client := tvspoof.NewClient()
	client.OnQuote = func(update tvspoof.QuoteUpdate) {
		latest.merge(update)
		if err := send(map[string]interface{}{"type": "quote", "data": latest}); err != nil {
			_ = client.Close()
		}
	}
	client.OnError = func(streamErr error) {
		_ = send(map[string]interface{}{"type": "error", "error": streamErr.Error()})
	}
	if err := client.Connect(); err != nil {
		_ = send(map[string]interface{}{"type": "error", "error": err.Error()})
		return
	}
	defer client.Close()
	client.AddSymbol(symbol)
	_ = send(map[string]interface{}{"type": "connected", "symbol": symbol})

	// Keep the connection alive until the browser disconnects.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func barsStream(w http.ResponseWriter, r *http.Request) {
	symbol := strings.TrimSpace(r.URL.Query().Get("symbol"))
	interval := strings.TrimSpace(r.URL.Query().Get("interval"))
	if symbol == "" {
		http.Error(w, "symbol is required", http.StatusBadRequest)
		return
	}
	if interval == "" {
		interval = "15"
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	var writeMu sync.Mutex
	send := func(value interface{}) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(value)
	}

	client := tvspoof.NewClient()
	client.OnBar = func(update tvspoof.BarUpdate) {
		if err := send(map[string]interface{}{
			"type": "bar",
			"data": map[string]interface{}{
				"symbol":   update.Symbol,
				"interval": update.Interval,
				"bar":      update.Bar,
			},
		}); err != nil {
			_ = client.Close()
		}
	}
	client.OnError = func(streamErr error) {
		_ = send(map[string]interface{}{"type": "error", "error": streamErr.Error()})
	}
	if err := client.Connect(); err != nil {
		_ = send(map[string]interface{}{"type": "error", "error": err.Error()})
		return
	}
	defer client.Close()
	if err := client.SubscribeBars(symbol, interval); err != nil {
		_ = send(map[string]interface{}{"type": "error", "error": err.Error()})
		return
	}
	_ = send(map[string]interface{}{"type": "connected", "symbol": symbol, "interval": interval})

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

// TradingView quote messages are partial updates. Keep the latest value for
// every field so API consumers always receive a coherent snapshot instead of
// losing bid/ask/change fields whenever they are omitted from the next frame.
type quoteSnapshot struct {
	Symbol        string   `json:"Symbol"`
	Price         *float64 `json:"Price,omitempty"`
	Volume        *float64 `json:"Volume,omitempty"`
	Bid           *float64 `json:"Bid,omitempty"`
	Ask           *float64 `json:"Ask,omitempty"`
	Open          *float64 `json:"Open,omitempty"`
	High          *float64 `json:"High,omitempty"`
	Low           *float64 `json:"Low,omitempty"`
	PrevClose     *float64 `json:"PrevClose,omitempty"`
	Change        *float64 `json:"Change,omitempty"`
	ChangePercent *float64 `json:"ChangePercent,omitempty"`
}

func (q *quoteSnapshot) merge(update tvspoof.QuoteUpdate) {
	if update.Symbol != "" {
		q.Symbol = update.Symbol
	}
	if update.Price != nil {
		q.Price = update.Price
	}
	if update.Volume != nil {
		q.Volume = update.Volume
	}
	if update.Bid != nil {
		q.Bid = update.Bid
	}
	if update.Ask != nil {
		q.Ask = update.Ask
	}
	if update.Open != nil {
		q.Open = update.Open
	}
	if update.High != nil {
		q.High = update.High
	}
	if update.Low != nil {
		q.Low = update.Low
	}
	if update.PrevClose != nil {
		q.PrevClose = update.PrevClose
	}
	if update.Change != nil {
		q.Change = update.Change
	}
	if update.ChangePercent != nil {
		q.ChangePercent = update.ChangePercent
	}
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
