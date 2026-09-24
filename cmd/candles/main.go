package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
	"time"

	tvspoof "github.com/azdharsyahputra/tradingview-spoof"
)

// ANSI Color definitions
const (
	cReset    = "\033[0m"
	cBold     = "\033[1m"
	cDim      = "\033[2m"
	cGreen    = "\033[38;5;48m"
	cRed      = "\033[38;5;203m"
	cYellow   = "\033[38;5;220m"
	cCyan     = "\033[38;5;75m"
	cBlue     = "\033[38;5;39m"
	cMagenta  = "\033[38;5;177m"
	cGray     = "\033[38;5;244m"
	cDarkGray = "\033[38;5;238m"
	cWhite    = "\033[38;5;255m"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
}

func printBoxRow(text string, innerWidth int) {
	visibleLen := len([]rune(stripANSI(text)))
	pad := innerWidth - visibleLen
	if pad < 0 {
		pad = 0
	}
	fmt.Println(cCyan + "║" + cReset + text + strings.Repeat(" ", pad) + cCyan + "║" + cReset)
}

// CandleAnalysis holds rich metrics per bar.
type CandleAnalysis struct {
	Index        int       `json:"index"`
	Timestamp    time.Time `json:"timestamp"`
	TimeUTC      string    `json:"time_utc"`
	TimeLocal    string    `json:"time_local"`
	Open         float64   `json:"open"`
	High         float64   `json:"high"`
	Low          float64   `json:"low"`
	Close        float64   `json:"close"`
	Volume       float64   `json:"volume"`
	Change       float64   `json:"change"`
	ChangePerc   float64   `json:"change_perc"`
	Range        float64   `json:"range"`
	BodySize     float64   `json:"body_size"`
	UpperWick    float64   `json:"upper_wick"`
	LowerWick    float64   `json:"lower_wick"`
	IsBullish    bool      `json:"is_bullish"`
	PatternTag   string    `json:"pattern_tag"`
	StructureTag string    `json:"structure_tag,omitempty"`
}

type CandlesFeedReport struct {
	Symbol          string           `json:"symbol"`
	Interval        string           `json:"interval"`
	RequestedBars   int              `json:"requested_bars"`
	ReceivedBars    int              `json:"received_bars"`
	LivePrice       float64          `json:"live_price"`
	PeriodHigh      float64          `json:"period_high"`
	PeriodLow       float64          `json:"period_low"`
	PeriodRange     float64          `json:"period_range"`
	NetChange       float64          `json:"net_change"`
	NetChangePerc   float64          `json:"net_change_perc"`
	BullishCount    int              `json:"bullish_count"`
	BearishCount    int              `json:"bearish_count"`
	AverageVolume   float64          `json:"average_volume"`
	ATR14           float64          `json:"atr14"`
	LastSwingHigh   float64          `json:"last_swing_high,omitempty"`
	LastSwingLow    float64          `json:"last_swing_low,omitempty"`
	Candles         []CandleAnalysis `json:"candles"`
}

func main() {
	symbolFlag := flag.String("symbol", "", "Target symbol (e.g. xauusd, btcusd, usoil, eurusd)")
	intervalFlag := flag.String("interval", "15", "Timeframe interval (1, 5, 15, 60, 240, D)")
	barsFlag := flag.Int("bars", 100, "Number of OHLCV bars (default: 100, max: 500)")
	jsonFlag := flag.Bool("json", false, "Output raw JSON structure for automated feeding/ingestion")
	limitFlag := flag.Int("limit", 0, "Limit rendered table rows to last N candles (0 = all)")
	flag.Parse()

	symbol := *symbolFlag
	interval := *intervalFlag
	barsCount := *barsFlag

	args := flag.Args()
	if len(args) >= 1 && symbol == "" {
		symbol = args[0]
	}
	if len(args) >= 2 {
		interval = args[1]
	}
	if len(args) >= 3 {
		fmt.Sscanf(args[2], "%d", &barsCount)
	}

	if symbol == "" {
		symbol = "xauusd"
	}
	// Institutional requirement: minimum mandatory 100 candles for robust structure analysis
	if barsCount < 100 {
		barsCount = 100
	}
	if barsCount > 1000 {
		barsCount = 1000
	}

	normSym := normalizeSymbol(symbol)

	if !*jsonFlag {
		fmt.Print("\033[?25l")
		fmt.Printf("%s[⚡ ZTERM OHLCV FEED ENGINE]%s Mengunduh %s%d candle%s (%s) untuk %s%s%s...\n",
			cBold+cCyan, cReset, cBold+cYellow, barsCount, cReset, interval, cBold+cGreen, normSym, cReset)
	}

	client := tvspoof.NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	bars, err := client.GetHistory(ctx, normSym, interval, barsCount)
	if !*jsonFlag {
		fmt.Print("\033[?25h")
	}

	if err != nil {
		if *jsonFlag {
			out, _ := json.Marshal(map[string]interface{}{"error": err.Error(), "symbol": normSym})
			fmt.Println(string(out))
			os.Exit(1)
		}
		fmt.Printf("\n%s[ERROR]%s Gagal menarik data OHLCV: %v\n", cRed+cBold, cReset, err)
		os.Exit(1)
	}

	if len(bars) == 0 {
		if *jsonFlag {
			out, _ := json.Marshal(map[string]interface{}{"error": "no bars received", "symbol": normSym})
			fmt.Println(string(out))
			os.Exit(1)
		}
		fmt.Printf("\n%s[ERROR]%s Data OHLCV kosong dari feed.\n", cRed+cBold, cReset)
		os.Exit(1)
	}

	report := analyzeCandles(normSym, interval, barsCount, bars)

	if *jsonFlag {
		jsonData, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Printf("{\"error\": %q}\n", err.Error())
			os.Exit(1)
		}
		fmt.Println(string(jsonData))
		return
	}

	renderCandlesTable(report, *limitFlag)
}

func normalizeSymbol(sym string) string {
	s := strings.TrimSpace(sym)
	if s == "" {
		return "OANDA:XAUUSD"
	}
	if strings.Contains(s, ":") {
		return strings.ToUpper(s)
	}
	upper := strings.ToUpper(s)
	switch upper {
	case "XAUUSD", "XAU", "GOLD", "EMAS":
		return "OANDA:XAUUSD"
	case "BTC", "BTCUSD", "BTCUSDT", "BITCOIN":
		return "FX:BTCUSD"
	case "USOIL", "OIL", "WTI", "CRUDE":
		return "TVC:USOIL"
	case "ETH", "ETHUSD", "ETHUSDT", "ETHEREUM":
		return "BINANCE:ETHUSDT"
	case "EURUSD", "EUR":
		return "FX:EURUSD"
	case "GBPUSD", "GBP":
		return "FX:GBPUSD"
	case "USDJPY", "JPY":
		return "FX:USDJPY"
	case "DXY", "USD":
		return "TVC:DXY"
	case "SPX", "SPX500", "US500", "S&P500", "SP500":
		return "TVC:SPX"
	case "NDX", "NAS100", "US100", "NASDAQ":
		return "TVC:NDX"
	case "XAGUSD", "SILVER", "PERAK":
		return "OANDA:XAGUSD"
	default:
		if len(upper) == 6 {
			return "FX:" + upper
		}
		return upper
	}
}

func analyzeCandles(symbol, interval string, requested int, bars []tvspoof.Bar) CandlesFeedReport {
	n := len(bars)
	report := CandlesFeedReport{
		Symbol:        symbol,
		Interval:      interval,
		RequestedBars: requested,
		ReceivedBars:  n,
		Candles:       make([]CandleAnalysis, n),
	}

	if n == 0 {
		return report
	}

	report.LivePrice = bars[n-1].Close
	minPrice := bars[0].Low
	maxPrice := bars[0].High
	totalVol := 0.0
	bullCount := 0
	bearCount := 0

	// Calculate Volumes and basic extremes
	for _, b := range bars {
		if b.High > maxPrice {
			maxPrice = b.High
		}
		if b.Low < minPrice {
			minPrice = b.Low
		}
		totalVol += b.Volume
		if b.Close >= b.Open {
			bullCount++
		} else {
			bearCount++
		}
	}

	report.PeriodHigh = maxPrice
	report.PeriodLow = minPrice
	report.PeriodRange = maxPrice - minPrice
	report.NetChange = bars[n-1].Close - bars[0].Open
	if bars[0].Open > 0 {
		report.NetChangePerc = (report.NetChange / bars[0].Open) * 100.0
	}
	report.BullishCount = bullCount
	report.BearishCount = bearCount
	report.AverageVolume = totalVol / float64(n)

	// Calculate ATR14
	report.ATR14 = calculateATR(bars, 14)

	// Analyze each candle individually
	for i := 0; i < n; i++ {
		b := bars[i]
		t := time.Unix(b.Time, 0).UTC()
		tLocal := t.Local()

		cRange := b.High - b.Low
		body := math.Abs(b.Close - b.Open)
		isBull := b.Close >= b.Open

		var upperWick, lowerWick float64
		if isBull {
			upperWick = b.High - b.Close
			lowerWick = b.Open - b.Low
		} else {
			upperWick = b.High - b.Open
			lowerWick = b.Close - b.Low
		}

		chg := b.Close - b.Open
		var chgPerc float64
		if b.Open > 0 {
			chgPerc = (chg / b.Open) * 100.0
		}

		// Pattern Classification
		pattern := classifyCandle(b, isBull, cRange, body, upperWick, lowerWick, report.AverageVolume)

		// Swing Detection (Lookback 3 left, 3 right if available)
		structTag := ""
		if i >= 3 && i < n-3 {
			isSwingHigh := true
			isSwingLow := true
			for k := i - 3; k <= i+3; k++ {
				if k == i {
					continue
				}
				if bars[k].High >= b.High {
					isSwingHigh = false
				}
				if bars[k].Low <= b.Low {
					isSwingLow = false
				}
			}
			if isSwingHigh {
				structTag = "👑 SWING HIGH [SH]"
				report.LastSwingHigh = b.High
			} else if isSwingLow {
				structTag = "🛡️ SWING LOW [SL]"
				report.LastSwingLow = b.Low
			}
		}

		// Check FVG Imbalance with 3-bar window
		if i >= 2 {
			prev2 := bars[i-2]
			if isBull && b.Low > prev2.High {
				structTag += " ⚡ BULL FVG"
			} else if !isBull && b.High < prev2.Low {
				structTag += " ⚡ BEAR FVG"
			}
		}

		report.Candles[i] = CandleAnalysis{
			Index:        i + 1,
			Timestamp:    t,
			TimeUTC:      t.Format("15:04"),
			TimeLocal:    tLocal.Format("15:04"),
			Open:         b.Open,
			High:         b.High,
			Low:          b.Low,
			Close:        b.Close,
			Volume:       b.Volume,
			Change:       chg,
			ChangePerc:   chgPerc,
			Range:        cRange,
			BodySize:     body,
			UpperWick:    upperWick,
			LowerWick:    lowerWick,
			IsBullish:    isBull,
			PatternTag:   pattern,
			StructureTag: strings.TrimSpace(structTag),
		}
	}

	return report
}

func calculateATR(bars []tvspoof.Bar, period int) float64 {
	n := len(bars)
	if n < 2 {
		return 0.0
	}
	if period > n-1 {
		period = n - 1
	}

	trSum := 0.0
	for i := n - period; i < n; i++ {
		high := bars[i].High
		low := bars[i].Low
		prevClose := bars[i-1].Close

		tr := math.Max(high-low, math.Max(math.Abs(high-prevClose), math.Abs(low-prevClose)))
		trSum += tr
	}
	return trSum / float64(period)
}

func classifyCandle(b tvspoof.Bar, isBull bool, cRange, body, upperWick, lowerWick, avgVol float64) string {
	if cRange <= 0.000001 {
		return "➖ Flat / Zero Range"
	}

	bodyRatio := body / cRange
	upperRatio := upperWick / cRange
	lowerRatio := lowerWick / cRange

	isVolSpike := b.Volume > (avgVol * 1.8) && avgVol > 0

	var pattern string
	if bodyRatio < 0.10 {
		if upperRatio > 0.40 && lowerRatio > 0.40 {
			pattern = "🟡 Long-Legged Doji"
		} else if upperRatio > 0.60 {
			pattern = "🔴 Gravestone Doji"
		} else if lowerRatio > 0.60 {
			pattern = "🟢 Dragonfly Doji"
		} else {
			pattern = "🟡 Neutral Doji"
		}
	} else if lowerRatio >= 0.55 && upperRatio <= 0.20 {
		if isBull {
			pattern = "🟢 Hammer Rejection"
		} else {
			pattern = "🟢 Bullish Wick Rejection (Absorption)"
		}
	} else if upperRatio >= 0.55 && lowerRatio <= 0.20 {
		if !isBull {
			pattern = "🔴 Shooting Star Rejection"
		} else {
			pattern = "🔴 Bearish Wick Rejection"
		}
	} else if bodyRatio >= 0.75 {
		if isBull {
			pattern = "🟢 Bullish Marubozu (Impulse)"
		} else {
			pattern = "🔴 Bearish Marubozu (Impulse)"
		}
	} else if isBull {
		pattern = "🟢 Bullish Candle"
	} else {
		pattern = "🔴 Bearish Candle"
	}

	if isVolSpike {
		pattern += " 💥 [VOL SPIKE]"
	}

	return pattern
}

func renderCandlesTable(r CandlesFeedReport, limit int) {
	p := tvspoof.DeterminePrecisionForSymbol(r.Symbol)
	w := 116

	fmt.Println()
	fmt.Println(cCyan + "╔" + strings.Repeat("═", w) + "╗" + cReset)

	title := fmt.Sprintf("  📊  %sZTERM INSTITUTIONAL OHLCV CANDLESTICK MATRIX (%d BARS / %s)%s",
		cBold, r.ReceivedBars, r.Interval, cReset)
	printBoxRow(title, w)
	fmt.Println(cCyan + "╠" + strings.Repeat("═", w) + "╣" + cReset)

	// Summary Ribbon
	changeColor := cGreen
	plusSign := "+"
	if r.NetChange < 0 {
		changeColor = cRed
		plusSign = ""
	}
	ribbon1 := fmt.Sprintf("  ASSET: %s%s%s  |  LIVE: %s%s%s  |  PERIOD RANGE: %s%s - %s (%.2f pts)%s  |  NET: %s%s%.2f (%.2f%%)%s",
		cBold+cYellow, r.Symbol, cReset,
		cBold+cGreen, tvspoof.FormatDeskPrice(r.LivePrice, p), cReset,
		cBold+cCyan, tvspoof.FormatDeskPrice(r.PeriodLow, p), tvspoof.FormatDeskPrice(r.PeriodHigh, p), r.PeriodRange, cReset,
		cBold+changeColor, plusSign, r.NetChange, r.NetChangePerc, cReset)
	printBoxRow(ribbon1, w)

	bullPerc := (float64(r.BullishCount) / float64(r.ReceivedBars)) * 100.0
	bearPerc := (float64(r.BearishCount) / float64(r.ReceivedBars)) * 100.0
	ribbon2 := fmt.Sprintf("  BULL/BEAR RATIO: %s%d Bull (%.1f%%)%s vs %s%d Bear (%.1f%%)%s  |  ATR(14): %s%.2f pts%s  |  AVG VOL: %s%.1f%s",
		cGreen, r.BullishCount, bullPerc, cReset,
		cRed, r.BearishCount, bearPerc, cReset,
		cBold+cYellow, r.ATR14, cReset,
		cCyan, r.AverageVolume, cReset)
	printBoxRow(ribbon2, w)
	fmt.Println(cCyan + "╠" + strings.Repeat("═", w) + "╣" + cReset)

	// Table Header
	header := fmt.Sprintf("  %-4s │ %-7s │ %-10s │ %-10s │ %-10s │ %-10s │ %-10s │ %-12s │ %-32s",
		"BAR#", "TIME", "OPEN", "HIGH", "LOW", "CLOSE", "DELTA", "VOL", "CANDLE TYPE & STRUCTURE")
	printBoxRow(header, w)
	fmt.Println(cCyan + "╟" + strings.Repeat("─", w) + "╢" + cReset)

	// Determine candles to render
	startIdx := 0
	if limit > 0 && limit < r.ReceivedBars {
		startIdx = r.ReceivedBars - limit
	}

	for i := startIdx; i < r.ReceivedBars; i++ {
		c := r.Candles[i]
		rowColor := cGreen
		deltaSign := "+"
		if !c.IsBullish {
			rowColor = cRed
			deltaSign = ""
		}

		tagStr := c.PatternTag
		if c.StructureTag != "" {
			tagStr = c.StructureTag + " | " + c.PatternTag
		}

		// Highlight latest candle
		idxStr := fmt.Sprintf("#%03d", c.Index)
		if i == r.ReceivedBars-1 {
			idxStr = fmt.Sprintf("%sLIVE%s", cBold+cYellow, cReset)
		}

		deltaStr := fmt.Sprintf("%s%s%.2f%s", rowColor, deltaSign, c.Change, cReset)
		volStr := fmt.Sprintf("%.0f", c.Volume)
		if c.Volume > r.AverageVolume*1.8 && r.AverageVolume > 0 {
			volStr = fmt.Sprintf("%s%.0f 💥%s", cBold+cYellow, c.Volume, cReset)
		}

		line := fmt.Sprintf("  %-4s │ %-7s │ %-10s │ %-10s │ %-10s │ %-10s │ %-18s │ %-18s │ %-32s",
			idxStr,
			c.TimeUTC,
			tvspoof.FormatDeskPrice(c.Open, p),
			tvspoof.FormatDeskPrice(c.High, p),
			tvspoof.FormatDeskPrice(c.Low, p),
			tvspoof.FormatDeskPrice(c.Close, p),
			deltaStr,
			volStr,
			tagStr,
		)
		printBoxRow(line, w)
	}

	fmt.Println(cCyan + "╚" + strings.Repeat("═", w) + "╝" + cReset)
	fmt.Printf("  💡 Tip: Gunakan `go run ./cmd/candles [symbol] [interval] [bars]` (contoh: `go run ./cmd/candles xauusd 15 100` atau `go run ./cmd/candles xauusd 5 300`)\n")
	fmt.Printf("          Tambahkan flag `--json` untuk output JSON murni yang bisa di-pipe ke subagent / script.\n\n")
}
