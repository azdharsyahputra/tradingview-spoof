package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	tvspoof "github.com/azdharsyahputra/tradingview-spoof"
)

// ANSI Color codes
const (
	colorReset     = "\033[0m"
	colorBold      = "\033[1m"
	colorDim       = "\033[2m"
	colorGreen     = "\033[38;5;48m"
	colorRed       = "\033[38;5;203m"
	colorYellow    = "\033[38;5;220m"
	colorCyan      = "\033[38;5;75m"
	colorBlue      = "\033[38;5;39m"
	colorGray      = "\033[38;5;244m"
	colorDarkGray  = "\033[38;5;238m"
	colorMagenta   = "\033[38;5;177m"
	colorBgDark    = "\033[48;5;235m"
	colorBgLive    = "\033[48;5;220m\033[38;5;232m"
)

type MarketState struct {
	mu           sync.RWMutex
	symbol       string
	interval     string
	historyBars  []tvspoof.Bar
	activeBar    tvspoof.Bar
	hasActiveBar bool

	// Quote / Tick data
	lastPrice     *float64
	bid           *float64
	ask           *float64
	change        *float64
	changePercent *float64
	volume        *float64
	open          *float64
	high          *float64
	low           *float64
	prevClose     *float64
	lastTickTime  time.Time
	tickCount     int
	statusMsg     string
	precision     int
}

func main() {
	symbolFlag := flag.String("symbol", "", "TradingView symbol (e.g., xauusd, btc, FX:EURUSD)")
	intervalFlag := flag.String("interval", "", "Timeframe resolution (1, 5, 15, 30, 60, 240, D, W)")
	barsFlag := flag.Int("bars", 12, "Number of OHLCV candles to display (5 - 30)")
	noClearFlag := flag.Bool("no-clear", false, "Do not clear terminal on update")
	flag.Parse()

	symbol := "OANDA:XAUUSD"
	interval := "15"

	// Check flags first
	if *symbolFlag != "" {
		symbol = normalizeSymbol(*symbolFlag)
	}
	if *intervalFlag != "" {
		interval = strings.TrimSpace(*intervalFlag)
	}

	// Check positional arguments (e.g. `go run ./cmd/thick xauusd 5` or `go run ./cmd/thick 5`)
	args := flag.Args()
	if len(args) == 1 {
		arg := strings.TrimSpace(args[0])
		if isInterval(arg) {
			interval = arg
		} else {
			symbol = normalizeSymbol(arg)
		}
	} else if len(args) >= 2 {
		symbol = normalizeSymbol(args[0])
		interval = strings.TrimSpace(args[1])
	}

	numBars := *barsFlag
	if numBars < 3 {
		numBars = 3
	} else if numBars > 35 {
		numBars = 35
	}

	precision := determinePrecision(symbol, 0)

	state := &MarketState{
		symbol:       symbol,
		interval:     interval,
		statusMsg:    "Mengambil riwayat data OHLCV...",
		lastTickTime: time.Now(),
		precision:    precision,
	}

	client := tvspoof.NewClient(
		tvspoof.WithAutoReconnect(true),
		tvspoof.WithReconnectDelay(3*time.Second),
	)

	// Quote / Tick Callback
	client.OnQuote = func(q tvspoof.QuoteUpdate) {
		if q.Symbol != symbol {
			return
		}
		state.mu.Lock()
		defer state.mu.Unlock()

		if q.Price != nil {
			state.lastPrice = q.Price
			state.precision = determinePrecision(symbol, *q.Price)
		}
		if q.Bid != nil {
			state.bid = q.Bid
		}
		if q.Ask != nil {
			state.ask = q.Ask
		}
		if q.Change != nil {
			state.change = q.Change
		}
		if q.ChangePercent != nil {
			state.changePercent = q.ChangePercent
		}
		if q.Volume != nil {
			state.volume = q.Volume
		}
		if q.Open != nil {
			state.open = q.Open
		}
		if q.High != nil {
			state.high = q.High
		}
		if q.Low != nil {
			state.low = q.Low
		}
		if q.PrevClose != nil {
			state.prevClose = q.PrevClose
		}
		state.lastTickTime = time.Now()
		state.tickCount++
	}

	// Bar / Candle Callback
	client.OnBar = func(update tvspoof.BarUpdate) {
		if update.Symbol != symbol || update.Interval != interval {
			return
		}
		state.mu.Lock()
		defer state.mu.Unlock()

		state.activeBar = update.Bar
		state.hasActiveBar = true

		if state.lastPrice == nil {
			state.precision = determinePrecision(symbol, update.Bar.Close)
		}

		// Replace or append to history bars
		if len(state.historyBars) > 0 {
			lastIdx := len(state.historyBars) - 1
			if state.historyBars[lastIdx].Time == update.Bar.Time {
				state.historyBars[lastIdx] = update.Bar
			} else if update.Bar.Time > state.historyBars[lastIdx].Time {
				state.historyBars = append(state.historyBars, update.Bar)
				if len(state.historyBars) > numBars+10 {
					state.historyBars = state.historyBars[len(state.historyBars)-(numBars+10):]
				}
			}
		} else {
			state.historyBars = append(state.historyBars, update.Bar)
		}
	}

	client.OnError = func(err error) {
		state.mu.Lock()
		state.statusMsg = fmt.Sprintf("Error: %v", err)
		state.mu.Unlock()
	}

	// Load initial OHLCV history
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		bars, err := client.GetHistory(ctx, symbol, interval, numBars+5)
		state.mu.Lock()
		if err != nil {
			state.statusMsg = fmt.Sprintf("Histori gagal: %v (menunggu live stream...)", err)
		} else {
			state.historyBars = bars
			if len(bars) > 0 {
				state.activeBar = bars[len(bars)-1]
				state.hasActiveBar = true
				if state.lastPrice == nil {
					state.precision = determinePrecision(symbol, bars[len(bars)-1].Close)
				}
			}
			state.statusMsg = "Terhubung ke TradingView live stream"
		}
		state.mu.Unlock()
	}()

	// Connect websocket
	if err := client.Connect(); err != nil {
		fmt.Printf("Gagal menghubungkan ke TradingView: %v\n", err)
		os.Exit(1)
	}

	// Subscribe to quotes & bars
	client.AddSymbol(symbol)
	if err := client.SubscribeBars(symbol, interval); err != nil {
		state.mu.Lock()
		state.statusMsg = fmt.Sprintf("Subscribe bar warning: %v", err)
		state.mu.Unlock()
	}

	// Render loop ticker
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-sigChan:
			fmt.Print("\033[?25h\n") // Show cursor
			fmt.Println(colorYellow + "Menutup koneksi..." + colorReset)
			client.Close()
			return
		case <-ticker.C:
			renderDashboard(state, numBars, !*noClearFlag)
		}
	}
}

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
}

func padCellLeft(content string, targetWidth int) string {
	visLen := len([]rune(stripANSI(content)))
	pad := targetWidth - visLen
	if pad <= 0 {
		return content
	}
	return content + strings.Repeat(" ", pad)
}

func padCellRight(content string, targetWidth int) string {
	visLen := len([]rune(stripANSI(content)))
	pad := targetWidth - visLen
	if pad <= 0 {
		return content
	}
	return strings.Repeat(" ", pad) + content
}

func padCellCenter(content string, targetWidth int) string {
	visLen := len([]rune(stripANSI(content)))
	pad := targetWidth - visLen
	if pad <= 0 {
		return content
	}
	leftPad := pad / 2
	rightPad := pad - leftPad
	return strings.Repeat(" ", leftPad) + content + strings.Repeat(" ", rightPad)
}

func padRow(left, right string, innerWidth int) string {
	visLeft := len([]rune(stripANSI(left)))
	visRight := len([]rune(stripANSI(right)))
	pad := innerWidth - visLeft - visRight
	if pad <= 0 {
		return left + " " + right
	}
	return left + strings.Repeat(" ", pad) + right
}

func renderDashboardString(s *MarketState, maxBars int) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var sb strings.Builder

	now := time.Now().UTC().Format("15:04:05")
	p := s.precision

	// Total inner width: 92 chars
	// Indent: 2 spaces. Total outer width: 2 + 1 + 92 + 1 = 96 chars.
	const innerW = 92

	// --- Summary Card Top Box ---
	sb.WriteString("  " + colorCyan + "┌" + strings.Repeat("─", innerW) + "┐\n" + colorReset)

	headerTitle := fmt.Sprintf("  ⚡ ZTERM TICK & OHLCV MONITOR  •  %s%s%s  •  TF: %s%s%s",
		colorYellow+colorBold, s.symbol, colorReset+colorCyan,
		colorGreen+colorBold, formatInterval(s.interval), colorReset+colorCyan,
	)
	timeText := fmt.Sprintf("Waktu: %s UTC  ", now)
	headerContent := padRow(headerTitle, timeText, innerW)
	sb.WriteString("  " + colorCyan + "│" + colorReset + headerContent + colorCyan + "│\n" + colorReset)
	sb.WriteString("  " + colorCyan + "├" + strings.Repeat("─", innerW) + "┤\n" + colorReset)

	// Format Card Values
	priceStr := "---"
	if s.lastPrice != nil {
		priceStr = formatVal(*s.lastPrice, p)
	}

	changeValStr := "---"
	chgColor := colorReset
	if s.change != nil && s.changePercent != nil {
		if *s.change >= 0 {
			chgColor = colorGreen
			changeValStr = fmt.Sprintf("+%s (+%.2f%%)", formatVal(*s.change, p), *s.changePercent)
		} else {
			chgColor = colorRed
			changeValStr = fmt.Sprintf("%s (%.2f%%)", formatVal(*s.change, p), *s.changePercent)
		}
	}

	bidStr := "---"
	if s.bid != nil {
		bidStr = formatVal(*s.bid, p)
	}
	askStr := "---"
	if s.ask != nil {
		askStr = formatVal(*s.ask, p)
	}
	bidAskStr := "---"
	if s.bid != nil && s.ask != nil {
		bidAskStr = fmt.Sprintf("%s / %s", bidStr, askStr)
	}

	spreadStr := "---"
	if s.bid != nil && s.ask != nil && *s.ask >= *s.bid {
		spreadStr = formatVal(*s.ask-*s.bid, p)
	}

	volStr := "---"
	if s.volume != nil {
		volStr = formatVolume(*s.volume)
	}

	rangeStr := "---"
	if s.low != nil && s.high != nil {
		rangeStr = fmt.Sprintf("%s - %s", formatVal(*s.low, p), formatVal(*s.high, p))
	}

	openStr := "---"
	if s.open != nil {
		openStr = formatVal(*s.open, p)
	}

	prevCloseStr := "---"
	if s.prevClose != nil {
		prevCloseStr = formatVal(*s.prevClose, p)
	}

	// 3-Column Card Layout (Columns: 34 | 32 | 24 = 90 + 2 inner separators = 92)
	// Row 1: LAST PRICE | CHANGE | TICKS
	c1r1 := "  " + colorBold + "LAST PRICE" + colorReset + " : " + colorBold + chgColor + priceStr + colorReset
	c2r1 := "  " + colorBold + "CHANGE" + colorReset + " : " + chgColor + changeValStr + colorReset
	c3r1 := fmt.Sprintf("  "+colorBold+"TICKS"+colorReset+"  : %d", s.tickCount)
	sb.WriteString("  " + colorCyan + "│" + colorReset + padCellLeft(c1r1, 34) + colorDarkGray + "│" + colorReset + padCellLeft(c2r1, 32) + colorDarkGray + "│" + colorReset + padCellLeft(c3r1, 24) + colorCyan + "│\n" + colorReset)

	// Row 2: BID / ASK | SPREAD | VOLUME
	c1r2 := "  " + colorBold + "BID / ASK " + colorReset + " : " + bidAskStr
	c2r2 := "  " + colorBold + "SPREAD" + colorReset + " : " + spreadStr
	c3r2 := "  " + colorBold + "VOLUME" + colorReset + " : " + volStr
	sb.WriteString("  " + colorCyan + "│" + colorReset + padCellLeft(c1r2, 34) + colorDarkGray + "│" + colorReset + padCellLeft(c2r2, 32) + colorDarkGray + "│" + colorReset + padCellLeft(c3r2, 24) + colorCyan + "│\n" + colorReset)

	// Row 3: DAY RANGE | DAY OPEN | PREV CLOSE
	c1r3 := "  " + colorBold + "DAY RANGE " + colorReset + " : " + rangeStr
	c2r3 := "  " + colorBold + "OPEN  " + colorReset + " : " + openStr
	c3r3 := "  " + colorBold + "P.CLOSE" + colorReset + ": " + prevCloseStr
	sb.WriteString("  " + colorCyan + "│" + colorReset + padCellLeft(c1r3, 34) + colorDarkGray + "│" + colorReset + padCellLeft(c2r3, 32) + colorDarkGray + "│" + colorReset + padCellLeft(c3r3, 24) + colorCyan + "│\n" + colorReset)

	sb.WriteString("  " + colorCyan + "└" + strings.Repeat("─", innerW) + "┘\n\n" + colorReset)

	// --- Active Desk Trade Plans Section ---
	plans, _ := tvspoof.LoadTradePlans("")
	if len(plans) > 0 {
		sb.WriteString(colorBold + colorYellow + "  📋 DESK TRADE PLANS (tradeplans.json):" + colorReset + "\n")
		sb.WriteString(colorDarkGray + "  ┌────────────────────────┬──────────┬──────────┬──────────┬──────────┬──────────┬────────────┐\n" + colorReset)
		sb.WriteString("  " + colorDarkGray + "│" + colorBold +
			padCellLeft("  Asset / Plan", 24) + colorDarkGray + "│" + colorBold +
			padCellCenter("Action", 10) + colorDarkGray + "│" + colorBold +
			padCellCenter("Status", 10) + colorDarkGray + "│" + colorBold +
			padCellRight("Entry  ", 10) + colorDarkGray + "│" + colorBold +
			padCellRight("SL  ", 10) + colorDarkGray + "│" + colorBold +
			padCellRight("TP1  ", 10) + colorDarkGray + "│" + colorBold +
			padCellCenter("RRR", 12) + colorDarkGray + "│\n" + colorReset)
		sb.WriteString(colorDarkGray + "  ├────────────────────────┼──────────┼──────────┼──────────┼──────────┼──────────┼────────────┤\n" + colorReset)

		for _, pl := range plans {
			planPrec := determinePrecision(pl.Symbol, pl.Entry)

			// Asset name
			assetLabel := pl.Asset
			if assetLabel == "" {
				assetLabel = pl.Symbol
			}
			if len([]rune(assetLabel)) > 21 {
				assetLabel = string([]rune(assetLabel)[:18]) + "..."
			}
			col1 := padCellLeft("  "+assetLabel, 24)

			// Direction with color
			dirStr := pl.Direction
			switch strings.ToUpper(pl.Direction) {
			case "BUY":
				dirStr = colorGreen + "▲ BUY" + colorReset
			case "SELL":
				dirStr = colorRed + "▼ SELL" + colorReset
			case "BUY_LIMIT":
				dirStr = colorCyan + "⏳ BUY LMT" + colorReset
			case "SELL_LIMIT":
				dirStr = colorMagenta + "⏳ SELL LMT" + colorReset
			}
			col2 := padCellCenter(dirStr, 10)

			// Status badge
			stStr := pl.Status
			switch strings.ToUpper(pl.Status) {
			case "RUNNING":
				stStr = colorGreen + colorBold + "RUNNING" + colorReset
			case "PENDING", "PENDING_LIMIT":
				stStr = colorYellow + "PENDING" + colorReset
			case "WATCHING":
				stStr = colorCyan + "WATCHING" + colorReset
			case "HIT_TP", "PROFIT":
				stStr = colorGreen + colorBold + "HIT TP" + colorReset
			case "HIT_SL", "CL":
				stStr = colorRed + colorBold + "HIT SL" + colorReset
			}
			col3 := padCellCenter(stStr, 10)

			col4 := padCellRight(formatVal(pl.Entry, planPrec)+"  ", 10)
			col5 := padCellRight(colorRed+formatVal(pl.SL, planPrec)+colorReset+"  ", 10)
			col6 := padCellRight(colorGreen+formatVal(pl.TP1, planPrec)+colorReset+"  ", 10)
			col7 := padCellCenter(pl.RRR, 12)

			pRow := "  " + colorDarkGray + "│" + colorReset +
				col1 + colorDarkGray + "│" + colorReset +
				col2 + colorDarkGray + "│" + colorReset +
				col3 + colorDarkGray + "│" + colorReset +
				col4 + colorDarkGray + "│" + colorReset +
				col5 + colorDarkGray + "│" + colorReset +
				col6 + colorDarkGray + "│" + colorReset +
				col7 + colorDarkGray + "│\n" + colorReset
			sb.WriteString(pRow)

			if pl.Notes != "" {
				noteTxt := "     ↳ Note: " + pl.Notes
				if len([]rune(noteTxt)) > 90 {
					noteTxt = string([]rune(noteTxt)[:87]) + "..."
				}
				noteCell := padCellLeft(colorGray+noteTxt+colorReset, innerW)
				sb.WriteString("  " + colorDarkGray + "│" + colorReset + noteCell + colorDarkGray + "│\n" + colorReset)
			}
		}
		sb.WriteString(colorDarkGray + "  └────────────────────────┴──────────┴──────────┴──────────┴──────────┴──────────┴────────────┘\n\n" + colorReset)
	}

	// --- OHLCV Table Section ---
	// Columns: Time(24) | Open(10) | High(10) | Low(10) | Close(10) | Range(10) | Trend(12)
	// Total: 24 + 10 + 10 + 10 + 10 + 10 + 12 = 86 + 6 inner separators = 92 inner width!
	sb.WriteString(colorBold + "  📊 RIWAYAT CANDLESTICK OHLCV (" + formatInterval(s.interval) + "):" + colorReset + "\n")
	sb.WriteString(colorDarkGray + "  ┌────────────────────────┬──────────┬──────────┬──────────┬──────────┬──────────┬────────────┐\n" + colorReset)
	
	tblHdr := "  " + colorDarkGray + "│" + colorBold +
		padCellLeft("  Waktu (UTC)", 24) + colorDarkGray + "│" + colorBold +
		padCellRight("Open  ", 10) + colorDarkGray + "│" + colorBold +
		padCellRight("High  ", 10) + colorDarkGray + "│" + colorBold +
		padCellRight("Low  ", 10) + colorDarkGray + "│" + colorBold +
		padCellRight("Close  ", 10) + colorDarkGray + "│" + colorBold +
		padCellRight("Range  ", 10) + colorDarkGray + "│" + colorBold +
		padCellCenter("Trend", 12) + colorDarkGray + "│\n" + colorReset
	sb.WriteString(tblHdr)
	sb.WriteString(colorDarkGray + "  ├────────────────────────┼──────────┼──────────┼──────────┼──────────┼──────────┼────────────┤\n" + colorReset)

	displayList := s.historyBars
	if len(displayList) > maxBars {
		displayList = displayList[len(displayList)-maxBars:]
	}

	if len(displayList) == 0 {
		sb.WriteString("  " + colorDarkGray + "│" + colorReset + padCellCenter("Menunggu data candlestick dari stream...", innerW) + colorDarkGray + "│\n" + colorReset)
	} else {
		for i, bar := range displayList {
			candleTime := time.Unix(bar.Time, 0).UTC().Format("2006-01-02 15:04")
			isBullish := bar.Close >= bar.Open
			rowColor := colorGreen
			trendText := "▲ BULL"
			if !isBullish {
				rowColor = colorRed
				trendText = "▼ BEAR"
			}

			isLive := (i == len(displayList)-1) && s.hasActiveBar

			var timeCell string
			if isLive {
				timeCell = padCellLeft(" "+candleTime+" "+colorBgLive+" LIVE "+colorReset, 24)
			} else {
				timeCell = padCellLeft(" "+candleTime, 24)
			}

			openCell := padCellRight(formatVal(bar.Open, p)+"  ", 10)
			highCell := padCellRight(formatVal(bar.High, p)+"  ", 10)
			lowCell := padCellRight(formatVal(bar.Low, p)+"  ", 10)
			closeCell := padCellRight(colorBold+rowColor+formatVal(bar.Close, p)+colorReset+"  ", 10)
			rangeCell := padCellRight(formatVal(math.Abs(bar.High-bar.Low), p)+"  ", 10)
			trendCell := padCellCenter(rowColor+trendText+colorReset, 12)

			rowStr := "  " + colorDarkGray + "│" + colorReset +
				timeCell + colorDarkGray + "│" + colorReset +
				openCell + colorDarkGray + "│" + colorReset +
				highCell + colorDarkGray + "│" + colorReset +
				lowCell + colorDarkGray + "│" + colorReset +
				closeCell + colorDarkGray + "│" + colorReset +
				rangeCell + colorDarkGray + "│" + colorReset +
				trendCell + colorDarkGray + "│\n" + colorReset
			sb.WriteString(rowStr)
		}
	}
	sb.WriteString(colorDarkGray + "  └────────────────────────┴──────────┴──────────┴──────────┴──────────┴──────────┴────────────┘\n" + colorReset)

	// --- Mini ASCII Chart of Close Prices ---
	if len(displayList) >= 3 {
		sb.WriteString("\n" + colorBold + "  📈 SPARKLINE CANDLE CLOSE:" + colorReset + "\n  ")
		minP := displayList[0].Close
		maxP := displayList[0].Close
		for _, b := range displayList {
			if b.Close < minP {
				minP = b.Close
			}
			if b.Close > maxP {
				maxP = b.Close
			}
		}

		sparks := []rune{' ', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
		for _, b := range displayList {
			ratio := 0.0
			if maxP > minP {
				ratio = (b.Close - minP) / (maxP - minP)
			}
			idx := int(ratio * float64(len(sparks)-1))
			if idx < 0 {
				idx = 0
			}
			if idx >= len(sparks) {
				idx = len(sparks) - 1
			}

			if b.Close >= b.Open {
				sb.WriteString(colorGreen + string(sparks[idx]) + colorReset)
			} else {
				sb.WriteString(colorRed + string(sparks[idx]) + colorReset)
			}
		}
		sb.WriteString(fmt.Sprintf(colorGray+"  [Low: %s | High: %s | Range: %s]\n"+colorReset,
			formatVal(minP, p), formatVal(maxP, p), formatVal(maxP-minP, p)))
	}

	// Status line
	sb.WriteString("\n" + colorGray + "  ● Status: " + s.statusMsg + " (Tekan Ctrl+C untuk keluar)\n" + colorReset)

	return sb.String()
}

func renderDashboard(s *MarketState, maxBars int, clearScreen bool) {
	output := renderDashboardString(s, maxBars)
	if clearScreen {
		fmt.Print("\033[H\033[2J\033[?25l") // Clear screen, move to top-left, hide cursor
	}
	fmt.Print(output)
}

func determinePrecision(symbol string, price float64) int {
	s := strings.ToUpper(symbol)
	if strings.Contains(s, "XAU") || strings.Contains(s, "GOLD") || strings.Contains(s, "BTC") || strings.Contains(s, "ETH") || strings.Contains(s, "SOL") || strings.Contains(s, "SPX") || strings.Contains(s, "NDX") || strings.Contains(s, "NAS100") || strings.Contains(s, "AAPL") || strings.Contains(s, "TSLA") || strings.Contains(s, "NVDA") {
		return 2
	}
	if strings.Contains(s, "JPY") {
		return 3
	}
	if strings.HasPrefix(s, "FX:") || len(s) == 6 {
		return 5
	}
	if price >= 100 {
		return 2
	} else if price >= 1 {
		return 4
	}
	return 6
}

func formatVal(val float64, precision int) string {
	return fmt.Sprintf("%.*f", precision, val)
}

func formatVolume(vol float64) string {
	if vol >= 1_000_000_000 {
		return fmt.Sprintf("%.2fB", vol/1_000_000_000)
	} else if vol >= 1_000_000 {
		return fmt.Sprintf("%.2fM", vol/1_000_000)
	} else if vol >= 1_000 {
		return fmt.Sprintf("%.2fK", vol/1_000)
	}
	return fmt.Sprintf("%.2f", vol)
}

func formatInterval(tf string) string {
	switch tf {
	case "1":
		return "1m"
	case "5":
		return "5m"
	case "15":
		return "15m"
	case "30":
		return "30m"
	case "60":
		return "1H"
	case "240":
		return "4H"
	case "D", "1D":
		return "1D"
	case "W", "1W":
		return "1W"
	case "M", "1M":
		return "1M"
	default:
		return tf
	}
}

func isInterval(val string) bool {
	switch strings.ToUpper(val) {
	case "1", "5", "15", "30", "60", "240", "D", "1D", "W", "1W", "M", "1M":
		return true
	default:
		return false
	}
}

func normalizeSymbol(input string) string {
	s := strings.TrimSpace(input)
	if s == "" {
		return "OANDA:XAUUSD"
	}
	if strings.Contains(s, ":") {
		parts := strings.SplitN(s, ":", 2)
		return strings.ToUpper(parts[0]) + ":" + strings.ToUpper(parts[1])
	}

	upper := strings.ToUpper(s)
	switch upper {
	case "XAUUSD", "XAU", "GOLD", "EMAS":
		return "OANDA:XAUUSD"
	case "BTC", "BTCUSDT", "BITCOIN":
		return "BINANCE:BTCUSDT"
	case "ETH", "ETHUSDT", "ETHEREUM":
		return "BINANCE:ETHUSDT"
	case "SOL", "SOLUSDT":
		return "BINANCE:SOLUSDT"
	case "EURUSD", "EUR":
		return "FX:EURUSD"
	case "GBPUSD", "GBP":
		return "FX:GBPUSD"
	case "USDJPY", "JPY":
		return "FX:USDJPY"
	case "AUDUSD", "AUD":
		return "FX:AUDUSD"
	case "USDCAD", "CAD":
		return "FX:USDCAD"
	case "USDCHF", "CHF":
		return "FX:USDCHF"
	case "DXY", "USD":
		return "TVC:DXY"
	case "SPX", "SP500":
		return "TVC:SPX"
	case "NDX", "NAS100", "NASDAQ":
		return "FOREXCOM:NAS100"
	case "AAPL":
		return "NASDAQ:AAPL"
	case "TSLA":
		return "NASDAQ:TSLA"
	case "NVDA":
		return "NASDAQ:NVDA"
	default:
		if len(upper) == 6 {
			return "FX:" + upper
		}
		return upper
	}
}
