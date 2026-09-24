package tvspoof

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// COTReport represents the parsed and calculated institutional commitments of traders data.
type COTReport struct {
	Symbol              string    `json:"symbol"`
	MarketName          string    `json:"market_name"`
	Exchange            string    `json:"exchange"`
	ReportDate          time.Time `json:"report_date"`
	NonCommLong         int       `json:"non_comm_long"`
	NonCommShort        int       `json:"non_comm_short"`
	NonCommNet          int       `json:"non_comm_net"`
	NonCommLongPct      float64   `json:"non_comm_long_pct"`
	CommLong            int       `json:"comm_long"`
	CommShort           int       `json:"comm_short"`
	CommNet             int       `json:"comm_net"`
	TotalOpenInterest   int       `json:"total_open_interest"`
	InstitutionalBias   string    `json:"institutional_bias"`
	InstitutionalRating string    `json:"institutional_rating"` // e.g. "ULTRA BULLISH", "BULLISH", "BEARISH", "NEUTRAL"
	StrategicDirective  string    `json:"strategic_directive"`
}

// rawCFTCRecord is the JSON schema returned by publicreporting.cftc.gov
type rawCFTCRecord struct {
	MarketAndExchangeNames string `json:"market_and_exchange_names"`
	ReportDateAsYYYYMMDD   string `json:"report_date_as_yyyy_mm_dd"`
	NonCommPositionsLong   string `json:"noncomm_positions_long_all"`
	NonCommPositionsShort  string `json:"noncomm_positions_short_all"`
	CommPositionsLong      string `json:"comm_positions_long_all"`
	CommPositionsShort     string `json:"comm_positions_short_all"`
	TotalOpenInterest      string `json:"open_interest_all"`
}

var cotMarketMap = map[string][]string{
	"XAUUSD": {"GOLD - COMMODITY EXCHANGE INC."},
	"USOIL":  {"WTI FINANCIAL CRUDE OIL - NEW YORK MERCANTILE EXCHANGE", "CRUDE OIL, LIGHT SWEET-WTI - ICE FUTURES EUROPE", "CRUDE OIL, LIGHT SWEET - NEW YORK MERCANTILE EXCHANGE"},
	"USDJPY": {"JAPANESE YEN - CHICAGO MERCANTILE EXCHANGE"},
	"EURUSD": {"EURO FX - CHICAGO MERCANTILE EXCHANGE"},
	"BTCUSD": {"BITCOIN - CHICAGO MERCANTILE EXCHANGE"},
	"XAGUSD": {"SILVER - COMMODITY EXCHANGE INC."},
	"GBPUSD": {"BRITISH POUND - CHICAGO MERCANTILE EXCHANGE"},
	"SPX":    {"E-MINI S&P 500 - CHICAGO MERCANTILE EXCHANGE"},
	"AUDUSD": {"AUSTRALIAN DOLLAR - CHICAGO MERCANTILE EXCHANGE"},
}

// NormalizeCOTSymbol maps common tickers/names to standardized COT asset keys.
func NormalizeCOTSymbol(sym string) string {
	s := strings.ToUpper(strings.TrimSpace(sym))
	s = strings.TrimPrefix(s, "OANDA:")
	s = strings.TrimPrefix(s, "TVC:")
	s = strings.TrimPrefix(s, "FX:")
	s = strings.TrimPrefix(s, "BINANCE:")
	s = strings.TrimPrefix(s, "SP:")

	switch s {
	case "GOLD", "XAU", "XAUUSD", "GC":
		return "XAUUSD"
	case "OIL", "USOIL", "WTI", "CL", "CRUDE":
		return "USOIL"
	case "JPY", "USDJPY", "6J", "YEN":
		return "USDJPY"
	case "EUR", "EURUSD", "6E", "EURO":
		return "EURUSD"
	case "BTC", "BTCUSD", "BITCOIN", "BTCUSDT":
		return "BTCUSD"
	case "SILVER", "XAG", "XAGUSD", "SI":
		return "XAGUSD"
	case "GBP", "GBPUSD", "6B", "POUND", "CABLE":
		return "GBPUSD"
	case "SPX", "US500", "ES", "SP500":
		return "SPX"
	case "AUD", "AUDUSD", "6A", "AUSSIE":
		return "AUDUSD"
	default:
		return s
	}
}

// FetchCOT retrieves the latest official CFTC Commitments of Traders data for a given asset.
func FetchCOT(ctx context.Context, symbol string) (*COTReport, error) {
	norm := NormalizeCOTSymbol(symbol)
	targetMarkets, exists := cotMarketMap[norm]
	if !exists {
		targetMarkets = []string{norm}
	}

	var lastErr error
	for _, mkt := range targetMarkets {
		rep, err := fetchSingleCOTMarket(ctx, norm, mkt)
		if err == nil && rep != nil {
			return rep, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, fmt.Errorf("failed to fetch COT data for %s: %w", symbol, lastErr)
	}
	return nil, fmt.Errorf("no COT data available for %s", symbol)
}

func fetchSingleCOTMarket(ctx context.Context, symKey string, marketName string) (*COTReport, error) {
	where := fmt.Sprintf("market_and_exchange_names = '%s'", marketName)
	endpoint := fmt.Sprintf("https://publicreporting.cftc.gov/resource/jun7-fc8e.json?$limit=1&$order=report_date_as_yyyy_mm_dd%%20DESC&$where=%s", url.QueryEscape(where))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ZTerm-Quant-Desk/1.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CFTC API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var records []rawCFTCRecord
	if err := json.Unmarshal(body, &records); err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no records returned for %s", marketName)
	}

	r := records[0]
	ncLong, _ := strconv.Atoi(r.NonCommPositionsLong)
	ncShort, _ := strconv.Atoi(r.NonCommPositionsShort)
	cLong, _ := strconv.Atoi(r.CommPositionsLong)
	cShort, _ := strconv.Atoi(r.CommPositionsShort)
	oi, _ := strconv.Atoi(r.TotalOpenInterest)

	parsedDate, _ := time.Parse("2006-01-02T15:04:05.000", r.ReportDateAsYYYYMMDD)
	if parsedDate.IsZero() {
		parsedDate, _ = time.Parse("2006-01-02", r.ReportDateAsYYYYMMDD)
	}

	ncNet := ncLong - ncShort
	cNet := cLong - cShort

	var longPct float64
	if ncLong+ncShort > 0 {
		longPct = float64(ncLong) / float64(ncLong+ncShort) * 100.0
	}

	rating := "NEUTRAL"
	bias := "🟡 NETRAL / BALANCE"
	directive := "Struktur pasar seimbang. Tunggu konfirmasi breakout zona Value Area."

	if longPct >= 80.0 || ncNet > 100000 {
		rating = "ULTRA BULLISH"
		bias = "🟢 ULTRA BULLISH (Extreme Smart Money Long)"
		directive = "DILARANG KERAS COUNTER-SHORT! Fokus 100% pada Buy Limit saat harga pullback ke Demand Zone / Discount Area."
	} else if longPct >= 58.0 || ncNet > 0 {
		rating = "BULLISH"
		bias = "🟢 BULLISH BIAS (Smart Money Accumulating)"
		directive = "Prioritaskan setup BUY. Cari konfirmasi MSS dan Fair Value Gap Bullish di M15."
	} else if longPct <= 35.0 || ncNet < -50000 {
		rating = "ULTRA BEARISH"
		bias = "🔴 ULTRA BEARISH (Heavy Smart Money Short)"
		directive = "DILARANG COUNTER-BUY! Fokus 100% pada Sell on Rallies di area Supply Zone / Premium Area."
	} else if longPct < 48.0 || ncNet < 0 {
		rating = "BEARISH"
		bias = "🔴 BEARISH BIAS (Smart Money Distributing)"
		directive = "Prioritaskan setup SELL. Cari rejection di Point of Control atau Daily Open."
	}

	parts := strings.Split(r.MarketAndExchangeNames, " - ")
	mName := r.MarketAndExchangeNames
	exch := "CME/CFTC"
	if len(parts) == 2 {
		mName = parts[0]
		exch = parts[1]
	}

	return &COTReport{
		Symbol:              symKey,
		MarketName:          mName,
		Exchange:            exch,
		ReportDate:          parsedDate,
		NonCommLong:         ncLong,
		NonCommShort:        ncShort,
		NonCommNet:          ncNet,
		NonCommLongPct:      longPct,
		CommLong:            cLong,
		CommShort:           cShort,
		CommNet:             cNet,
		TotalOpenInterest:   oi,
		InstitutionalBias:   bias,
		InstitutionalRating: rating,
		StrategicDirective:  directive,
	}, nil
}

// FetchAllCoreCOT retrieves COT reports for the primary monitored macro assets.
func FetchAllCoreCOT(ctx context.Context) ([]*COTReport, error) {
	coreSymbols := []string{"XAUUSD", "USOIL", "USDJPY", "EURUSD", "BTCUSD", "XAGUSD"}
	var results []*COTReport
	for _, sym := range coreSymbols {
		rep, err := FetchCOT(ctx, sym)
		if err == nil && rep != nil {
			results = append(results, rep)
		}
	}
	return results, nil
}
