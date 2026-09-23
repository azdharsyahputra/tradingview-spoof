package main

import (
	"context"
	"flag"
	"fmt"
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
	cGreen    = "\033[38;5;48m"
	cRed      = "\033[38;5;203m"
	cYellow   = "\033[38;5;220m"
	cCyan     = "\033[38;5;75m"
	cBlue     = "\033[38;5;39m"
	cMagenta  = "\033[38;5;177m"
	cGray     = "\033[38;5;244m"
	cDarkGray = "\033[38;5;238m"
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

func main() {
	symbolFlag := flag.String("symbol", "OANDA:XAUUSD", "Target symbol (e.g. xauusd, btc, eurusd)")
	flag.Parse()

	symbol := *symbolFlag
	args := flag.Args()
	if len(args) >= 1 {
		symbol = args[0]
	}

	normSym := tvspoof.NormalizeDeskSymbol(symbol)

	fmt.Print("\033[?25l")
	fmt.Printf("%s[⚡ ZTERM LEVELS ENGINE]%s Mengkalkulasi High/Low, S/R & Supply/Demand untuk %s%s%s...\n",
		cBold+cCyan, cReset, cBold+cYellow, normSym, cReset)

	client := tvspoof.NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	briefing, err := client.AnalyzeDesk(ctx, normSym, 10000, 1.0)
	fmt.Print("\033[?25h")

	if err != nil {
		fmt.Printf("\n%s[ERROR]%s Gagal menghitung Key Levels: %v\n", cRed+cBold, cReset, err)
		os.Exit(1)
	}

	renderLevelsReport(briefing)
}

func renderLevelsReport(b *tvspoof.DeskBriefing) {
	p := b.Precision
	w := 96

	fmt.Println()
	fmt.Println(cCyan + "╔" + strings.Repeat("═", w) + "╗" + cReset)

	timeStr := fmt.Sprintf("%s UTC", b.Timestamp.Format("2006-01-02 15:04:05"))
	headerText := fmt.Sprintf("  📐  %sZTERM INSTITUTIONAL S/R & SUPPLY/DEMAND RADAR%s     %s%s%s  ",
		cBold, cReset, cGray, timeStr, cReset)
	printBoxRow(headerText, w)
	fmt.Println(cCyan + "╠" + strings.Repeat("═", w) + "╣" + cReset)

	// Ribbon
	ribbon := fmt.Sprintf("  TARGET ASSET: %s%s%s  |  LIVE PRICE: %s%s%s  |  TODAY RANGE: %s%s - %s (%.2f pts)%s",
		cBold+cYellow, b.Symbol, cReset,
		cBold+cGreen, tvspoof.FormatDeskPrice(b.CurrentPrice, p), cReset,
		cBold+cCyan, tvspoof.FormatDeskPrice(b.Levels.TodayLow, p), tvspoof.FormatDeskPrice(b.Levels.TodayHigh, p),
		b.Levels.TodayRange, cReset)
	printBoxRow(ribbon, w)

	if b.SMC.StructureSeq.LastHigh.Price > 0 || b.SMC.StructureSeq.LastLow.Price > 0 {
		structLine := fmt.Sprintf("  STRUCTURE: %s  |  SWINGS: %s%s%s (High: %s @ %s | Low: %s @ %s)",
			b.SMC.StructureSeq.TrendBias,
			cBold+cYellow, b.SMC.StructureSeq.SequenceFlow, cReset,
			b.SMC.StructureSeq.LastHigh.Type, tvspoof.FormatDeskPrice(b.SMC.StructureSeq.LastHigh.Price, p),
			b.SMC.StructureSeq.LastLow.Type, tvspoof.FormatDeskPrice(b.SMC.StructureSeq.LastLow.Price, p))
		printBoxRow(structLine, w)
	}
	fmt.Println(cCyan + "╠" + strings.Repeat("═", w) + "╣" + cReset)

	// Section 1: Key Reference Price Points
	printBoxRow(cBold+cCyan+"  📍 1. KEY BENCHMARK REFERENCE LEVELS (HIERARCHY)"+cReset, w)
	printBoxRow(cGray+"     Level Name                   | Price        | Distance    | Type         | Notes"+cReset, w)
	printBoxRow(cDarkGray+"     ─────────────────────────────┼──────────────┼─────────────┼──────────────┼─────────────────────────"+cReset, w)

	for _, kl := range b.Levels.KeyLevels {
		typeColor := cYellow
		distSign := "+"
		distColor := cRed
		if kl.Distance < 0 {
			distSign = "-"
			distColor = cGreen
		}
		if kl.Type == "Support" {
			typeColor = cGreen
		} else if kl.Type == "Resistance" {
			typeColor = cRed
		} else if kl.Type == "Benchmark" {
			typeColor = cBlue
		}

		distStr := fmt.Sprintf("%s%s%.2f pts%s", distColor, distSign, kl.DistancePts, cReset)
		if kl.DistancePts < 0.1 {
			distStr = cBold + cGreen + "At Level" + cReset
		}

		row := fmt.Sprintf("     %-27s | %-12s | %-20s | %s%-12s%s | %s",
			kl.Name, tvspoof.FormatDeskPrice(kl.Price, p),
			distStr,
			typeColor, kl.Type, cReset,
			kl.Description)
		printBoxRow(row, w)
	}

	fmt.Println(cCyan + "╟" + strings.Repeat("─", w) + "╢" + cReset)

	// Section 2: Support & Resistance Matrix
	printBoxRow(cBold+cYellow+"  🛡️ 2. CLASSIC & DYNAMIC PIVOT SUPPORT / RESISTANCE (S/R) MATRIX"+cReset, w)
	pivotLine := fmt.Sprintf("     • Central Daily Pivot Point : %s%s%s  (Mean Price Reference)",
		cBold+cYellow, tvspoof.FormatDeskPrice(b.Levels.DailyPivot, p), cReset)
	printBoxRow(pivotLine, w)

	printBoxRow(cGray+"     Level Type        | Price        | Distance     | Tests Count  | Strength   | Source"+cReset, w)
	printBoxRow(cDarkGray+"     ──────────────────┼──────────────┼──────────────┼──────────────┼────────────┼────────────────────────"+cReset, w)

	// Resistances (reverse order R3 -> R1)
	for i := len(b.Levels.Resistances) - 1; i >= 0; i-- {
		r := b.Levels.Resistances[i]
		row := fmt.Sprintf("     %s🔴 %-14s%s | %-12s | +%-11.2f | %-12d | %-10s | %s",
			cBold+cRed, r.Level, cReset,
			tvspoof.FormatDeskPrice(r.Price, p),
			r.DistancePts, r.TestCount, r.Strength, r.Source)
		printBoxRow(row, w)
	}

	// Current price divider
	currDivider := fmt.Sprintf("     ──▶ %s📍 CURRENT PRICE: %s%s ◀──────────────────────────────────────────────────────────",
		cBold+cGreen, tvspoof.FormatDeskPrice(b.CurrentPrice, p), cReset)
	printBoxRow(currDivider, w)

	// Supports (S1 -> S3)
	for _, s := range b.Levels.Supports {
		row := fmt.Sprintf("     %s🟢 %-14s%s | %-12s | -%-11.2f | %-12d | %-10s | %s",
			cBold+cGreen, s.Level, cReset,
			tvspoof.FormatDeskPrice(s.Price, p),
			s.DistancePts, s.TestCount, s.Strength, s.Source)
		printBoxRow(row, w)
	}

	fmt.Println(cCyan + "╟" + strings.Repeat("─", w) + "╢" + cReset)

	// Section 3: Supply & Demand Zones
	printBoxRow(cBold+cMagenta+"  ⚡ 3. INSTITUTIONAL ORDER FLOW SUPPLY & DEMAND ZONES"+cReset, w)
	printBoxRow(cGray+"     Zone Type     | Price Range               | Distance  | Timeframe | Status     | Strength"+cReset, w)
	printBoxRow(cDarkGray+"     ──────────────┼───────────────────────────┼───────────┼───────────┼────────────┼────────────────────────"+cReset, w)

	// Supply Zones
	for idx, sz := range b.Levels.SupplyZones {
		distStr := fmt.Sprintf("+%.2f pts", sz.DistancePts)
		if sz.DistancePts == 0 {
			distStr = "In Zone"
		}
		row := fmt.Sprintf("     %s🔴 Supply #%d%s  | %-25s | %-9s | %-9s | %-10s | %s",
			cBold+cRed, idx+1, cReset,
			fmt.Sprintf("%s - %s", tvspoof.FormatDeskPrice(sz.BottomPrice, p), tvspoof.FormatDeskPrice(sz.TopPrice, p)),
			distStr, sz.Timeframe, sz.Status, sz.Strength)
		printBoxRow(row, w)
	}

	// Demand Zones
	for idx, dz := range b.Levels.DemandZones {
		distStr := fmt.Sprintf("-%.2f pts", dz.DistancePts)
		if dz.DistancePts == 0 {
			distStr = "In Zone"
		}
		row := fmt.Sprintf("     %s🟢 Demand #%d%s  | %-25s | %-9s | %-9s | %-10s | %s",
			cBold+cGreen, idx+1, cReset,
			fmt.Sprintf("%s - %s", tvspoof.FormatDeskPrice(dz.BottomPrice, p), tvspoof.FormatDeskPrice(dz.TopPrice, p)),
			distStr, dz.Timeframe, dz.Status, dz.Strength)
		printBoxRow(row, w)
	}

	fmt.Println(cCyan + "╚" + strings.Repeat("═", w) + "╝" + cReset)
	fmt.Printf(cGray+"  💡 Tip: Gunakan `go run ./cmd/levels [symbol]` untuk radar level & `go run ./cmd/desk [symbol]` untuk full desk.\n\n"+cReset)
}
