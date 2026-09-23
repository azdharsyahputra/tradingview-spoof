package tvspoof

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	exnessspoof "github.com/azdharsyahputra/api-exness"
)

// ExnessConfig holds authentication and server details for Exness execution.
type ExnessConfig struct {
	Token     string `json:"token"`
	AccountID string `json:"account_id"`
	Server    string `json:"server"`
	WSURL     string `json:"ws_url"`
}

// ExnessPosition represents a formatted open position on Exness.
type ExnessPosition struct {
	ID         string  `json:"id"`
	Instrument string  `json:"instrument"`
	Type       string  `json:"type"` // "BUY" or "SELL"
	Volume     float64 `json:"volume"`
	OpenPrice  float64 `json:"open_price"`
	SL         float64 `json:"sl"`
	TP         float64 `json:"tp"`
	Profit     float64 `json:"profit"`
	OpenTime   string  `json:"open_time"`
}

// LoadExnessConfig attempts to load Exness credentials from environment variables or .env files.
func LoadExnessConfig() (*ExnessConfig, error) {
	cfg := &ExnessConfig{
		Token:     os.Getenv("EXNESS_TOKEN"),
		AccountID: os.Getenv("EXNESS_ACCOUNT_ID"),
		Server:    os.Getenv("EXNESS_SERVER"),
		WSURL:     os.Getenv("EXNESS_WS_URL"),
	}

	// Try reading .env from current directory, parent, or ../api-exness/.env
	envPaths := []string{
		".env",
		filepath.Join("..", ".env"),
		filepath.Join("..", "api-exness", ".env"),
		"/Users/csadeveloper/kkn/api-exness/.env",
	}

	for _, p := range envPaths {
		if f, err := os.Open(p); err == nil {
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				parts := strings.SplitN(line, "=", 2)
				if len(parts) != 2 {
					continue
				}
				key := strings.TrimSpace(parts[0])
				val := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
				switch key {
				case "EXNESS_TOKEN":
					if cfg.Token == "" {
						cfg.Token = val
					}
				case "EXNESS_ACCOUNT_ID":
					if cfg.AccountID == "" {
						cfg.AccountID = val
					}
				case "EXNESS_SERVER":
					if cfg.Server == "" {
						cfg.Server = val
					}
				case "EXNESS_WS_URL":
					if cfg.WSURL == "" {
						cfg.WSURL = val
					}
				}
			}
			f.Close()
		}
	}

	if cfg.Server == "" {
		cfg.Server = "trial6"
	}
	if cfg.WSURL == "" && cfg.AccountID != "" {
		cfg.WSURL = fmt.Sprintf("wss://rtapi-sg.useusecapwaskeyyepjamlaw.com/rtapi/mt5/%s/v2/ws/ticks/accounts/%s", cfg.Server, cfg.AccountID)
	}

	if cfg.Token == "" || cfg.AccountID == "" {
		return nil, fmt.Errorf("EXNESS_TOKEN or EXNESS_ACCOUNT_ID not found in environment or .env file")
	}

	return cfg, nil
}

// NewExnessClient initializes a connected client for Exness trading.
func NewExnessClient(cfg *ExnessConfig) *exnessspoof.Client {
	return exnessspoof.NewClient(
		exnessspoof.WithWSURL(cfg.WSURL),
		exnessspoof.WithToken(cfg.Token),
		exnessspoof.WithOrigin("https://my.exness.com"),
		exnessspoof.WithPingInterval(5*time.Second),
		exnessspoof.WithAutoReconnect(true),
	)
}

// NormalizeExnessSymbol adapts standard symbols (e.g. XAUUSD) to Exness instrument naming (e.g. XAUUSDm).
func NormalizeExnessSymbol(sym string) string {
	s := strings.ToUpper(strings.TrimSpace(sym))
	s = strings.TrimPrefix(s, "OANDA:")
	s = strings.TrimPrefix(s, "FX:")
	s = strings.TrimPrefix(s, "TVC:")

	// Exness standard accounts usually append 'm' (mini/standard), raw accounts do not.
	// If already has 'm' or other suffix, keep it.
	if strings.HasSuffix(s, "M") {
		return s
	}
	if s == "XAUUSD" || s == "GOLD" {
		return "XAUUSDm"
	}
	if s == "BTCUSD" || s == "BTCUSDT" {
		return "BTCUSDm"
	}
	if s == "ETHUSD" || s == "ETHUSDT" {
		return "ETHUSDm"
	}
	if len(s) == 6 {
		return s + "m"
	}
	return s
}

// FetchLiveExnessPrice connects briefly via WebSocket to get the latest Ask/Bid for an instrument.
func FetchLiveExnessPrice(ctx context.Context, cfg *ExnessConfig, instrument string) (ask, bid float64, err error) {
	client := NewExnessClient(cfg)
	if err := client.Connect(); err != nil {
		return 0, 0, fmt.Errorf("exness ws connect error: %w", err)
	}
	defer client.Close()

	tickChan := make(chan exnessspoof.TickUpdate, 10)
	client.OnTick = func(tick exnessspoof.TickUpdate) {
		if strings.EqualFold(tick.Instrument, instrument) && tick.Ask != nil && tick.Bid != nil {
			select {
			case tickChan <- tick:
			default:
			}
		}
	}

	client.Subscribe(instrument)

	select {
	case tick := <-tickChan:
		return *tick.Ask, *tick.Bid, nil
	case <-time.After(8 * time.Second):
		return 0, 0, fmt.Errorf("timeout waiting for live tick for %s", instrument)
	case <-ctx.Done():
		return 0, 0, ctx.Err()
	}
}

// ExnessOrderParams contains parameters for executing a trade.
type ExnessOrderParams struct {
	Symbol    string
	Direction string // "BUY" or "SELL"
	Volume    float64
	SL        float64
	TP        float64
	Price     float64 // optional, 0 for market price
}

// ExecuteExnessOrder executes a BUY or SELL order with live price streaming.
func ExecuteExnessOrder(ctx context.Context, params ExnessOrderParams) (map[string]interface{}, error) {
	cfg, err := LoadExnessConfig()
	if err != nil {
		return nil, err
	}

	instrument := NormalizeExnessSymbol(params.Symbol)
	client := NewExnessClient(cfg)

	orderType := exnessspoof.OrderTypeBuy
	if strings.ToUpper(params.Direction) == "SELL" {
		orderType = exnessspoof.OrderTypeSell
	}

	execPrice := params.Price
	if execPrice <= 0 {
		// Stream live tick price
		ask, bid, err := FetchLiveExnessPrice(ctx, cfg, instrument)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch live price for order execution: %w", err)
		}
		if orderType == exnessspoof.OrderTypeBuy {
			execPrice = ask
		} else {
			execPrice = bid
		}
	}

	res, err := client.OpenPosition(cfg.Server, cfg.AccountID, instrument, orderType, params.Volume, execPrice, params.SL, params.TP)
	if err != nil {
		return nil, fmt.Errorf("exness order execution failed: %w", err)
	}

	return res, nil
}

// GetExnessPositions retrieves all currently open positions on Exness.
func GetExnessPositions(ctx context.Context) ([]ExnessPosition, error) {
	cfg, err := LoadExnessConfig()
	if err != nil {
		return nil, err
	}

	client := NewExnessClient(cfg)
	raw, err := client.GetPositions(cfg.Server, cfg.AccountID)
	if err != nil {
		return nil, err
	}

	posListRaw, ok := raw["positions"].([]interface{})
	if !ok {
		return nil, nil
	}

	var positions []ExnessPosition
	for _, p := range posListRaw {
		pMap, ok := p.(map[string]interface{})
		if !ok {
			continue
		}

		var idStr string
		switch v := pMap["position_id"].(type) {
		case float64:
			idStr = fmt.Sprintf("%.0f", v)
		case int64, int:
			idStr = fmt.Sprintf("%d", v)
		default:
			idStr = fmt.Sprintf("%v", v)
		}

		posType := "BUY"
		if t, ok := pMap["type"].(float64); ok && int(t) == exnessspoof.OrderTypeSell {
			posType = "SELL"
		}

		inst, _ := pMap["instrument"].(string)
		price, _ := pMap["price"].(float64)
		vol, _ := pMap["volume"].(float64)
		sl, _ := pMap["sl"].(float64)
		tp, _ := pMap["tp"].(float64)
		profit, _ := pMap["profit"].(float64)
		openTime, _ := pMap["open_time"].(string)

		positions = append(positions, ExnessPosition{
			ID:         idStr,
			Instrument: inst,
			Type:       posType,
			Volume:     vol,
			OpenPrice:  price,
			SL:         sl,
			TP:         tp,
			Profit:     profit,
			OpenTime:   openTime,
		})
	}

	return positions, nil
}

// CloseExnessPosition closes a specific position by ID.
func CloseExnessPosition(ctx context.Context, positionID string, volume float64) (map[string]interface{}, error) {
	cfg, err := LoadExnessConfig()
	if err != nil {
		return nil, err
	}

	client := NewExnessClient(cfg)

	// Fetch open positions to find the instrument and open price if volume is not given
	positions, err := GetExnessPositions(ctx)
	if err != nil {
		return nil, err
	}

	var targetPos *ExnessPosition
	for i := range positions {
		if positions[i].ID == positionID {
			targetPos = &positions[i]
			break
		}
	}

	if targetPos == nil {
		return nil, fmt.Errorf("position #%s not found among open positions", positionID)
	}

	closeVol := volume
	if closeVol <= 0 {
		closeVol = targetPos.Volume
	}

	// Fetch live price for closing
	ask, bid, err := FetchLiveExnessPrice(ctx, cfg, targetPos.Instrument)
	closePrice := targetPos.OpenPrice
	if err == nil {
		if targetPos.Type == "BUY" {
			closePrice = bid // close buy at bid
		} else {
			closePrice = ask // close sell at ask
		}
	}

	res, err := client.ClosePosition(cfg.Server, cfg.AccountID, positionID, closeVol, closePrice)
	if err != nil {
		return nil, fmt.Errorf("failed to close position #%s: %w", positionID, err)
	}

	return res, nil
}

// CloseAllExnessPositions closes all open positions sequentially.
func CloseAllExnessPositions(ctx context.Context) ([]string, error) {
	positions, err := GetExnessPositions(ctx)
	if err != nil {
		return nil, err
	}

	var closedIDs []string
	var errs []string

	for _, p := range positions {
		_, err := CloseExnessPosition(ctx, p.ID, p.Volume)
		if err != nil {
			errs = append(errs, fmt.Sprintf("#%s (%v)", p.ID, err))
		} else {
			closedIDs = append(closedIDs, p.ID)
		}
		time.Sleep(300 * time.Millisecond)
	}

	if len(errs) > 0 {
		return closedIDs, fmt.Errorf("some positions failed to close: %s", strings.Join(errs, ", "))
	}

	return closedIDs, nil
}

// SaveExnessCredentials saves the provided credentials to the local .env file.
func SaveExnessCredentials(token, accountID, server, wsURL string) error {
	envPath := ".env"
	if server == "" {
		server = "trial6"
	}
	if wsURL == "" && accountID != "" {
		wsURL = fmt.Sprintf("wss://rtapi-sg.useusecapwaskeyyepjamlaw.com/rtapi/mt5/%s/v2/ws/ticks/accounts/%s", server, accountID)
	}

	content := fmt.Sprintf("# Exness API Trading Configuration\nEXNESS_TOKEN=%s\nEXNESS_ACCOUNT_ID=%s\nEXNESS_SERVER=%s\nEXNESS_WS_URL=%s\n",
		token, accountID, server, wsURL)

	return os.WriteFile(envPath, []byte(content), 0600)
}
