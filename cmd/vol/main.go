package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	tvspoof "github.com/azdharsyahputra/tradingview-spoof"
)

// ANSI Colors
const (
	cReset   = "\033[0m"
	cBold    = "\033[1m"
	cDim     = "\033[2m"
	cRed     = "\033[31m"
	cGreen   = "\033[32m"
	cYellow  = "\033[33m"
	cBlue    = "\033[34m"
	cMagenta = "\033[35m"
	cCyan    = "\033[36m"
	cGray    = "\033[90m"
)

func main() {
	flag.Parse()
	args := flag.Args()

	symbol := "XAUUSD"
	if len(args) > 0 {
		symbol = strings.ToUpper(args[0])
	}

	normSym, precision := normalizeSymbol(symbol)

	fmt.Printf("\n%s[⚡ ZTERM VOLUME & CVD ENGINE]%s Menganalisis Session Profiles, HVN/LVN, nPOC & Delta untuk %s...\n\n", cBold+cCyan, cReset, normSym)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client := tvspoof.NewClient()
	bars15M, err := client.GetHistory(ctx, normSym, "15", 100)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError fetching historical bars: %v%s\n", cRed, err, cReset)
		os.Exit(1)
	}

	if len(bars15M) == 0 {
		fmt.Fprintf(os.Stderr, "%sNo bar data received for %s%s\n", cRed, normSym, cReset)
		os.Exit(1)
	}

	livePrice := bars15M[len(bars15M)-1].Close

	// 1. Calculate Volume Profile & nPOCs
	volReport := tvspoof.AnalyzeVolumeProfile(bars15M, livePrice, precision, normSym)

	// 2. Calculate CVD & Order Flow Delta
	cvdSummary := tvspoof.CalculateCVD(bars15M)

	// Render Full Terminal Card
	printVolumeTerminalDashboard(volReport, cvdSummary, precision)
}

func normalizeSymbol(s string) (string, int) {
	upper := strings.ToUpper(strings.TrimSpace(s))
	switch upper {
	case "XAUUSD", "XAU", "GOLD", "EMAS":
		return "OANDA:XAUUSD", 2
	case "USOIL", "OIL", "CL", "WTI":
		return "TVC:USOIL", 2
	case "USDJPY", "UJ":
		return "FX:USDJPY", 3
	case "EURUSD", "EU":
		return "OANDA:EURUSD", 5
	case "GBPUSD", "GU":
		return "OANDA:GBPUSD", 5
	case "BTCUSD", "BTCUSDT", "BITCOIN":
		return "BINANCE:BTCUSDT", 2
	default:
		if strings.Contains(upper, ":") {
			return upper, 2
		}
		return "OANDA:" + upper, 2
	}
}

func printVolumeTerminalDashboard(v tvspoof.VolumeAnalysisReport, cvd tvspoof.CVDSummary, p int) {
	w := 98

	printBoxHeader("📊  ZTERM INSTITUTIONAL VOLUME & CVD DELTA RADAR", v.Timestamp.Format("2006-01-02 15:04:05 MST"), w)
	printBoxRow(fmt.Sprintf("  TARGET ASSET: %s%s%s  |  LIVE PRICE: %s%s%s  |  CURRENT SESSION: %s%s%s",
		cBold+cYellow, v.Symbol, cReset,
		cBold+cGreen, tvspoof.FormatDeskPrice(v.CurrentPrice, p), cReset,
		cBold+cCyan, v.CurrentSession, cReset), w)
	printBoxRow(fmt.Sprintf("  COMPOSITE 100-BAR POC: %s%s%s  |  VAH: %s  |  VAL: %s  |  TOTAL VOL: %s",
		cBold+cYellow, tvspoof.FormatDeskPrice(v.CompositePOC, p), cReset,
		tvspoof.FormatDeskPrice(v.CompositeVAH, p),
		tvspoof.FormatDeskPrice(v.CompositeVAL, p),
		formatCompact(v.TotalVolume)), w)
	printBoxDivider(w)

	// Section 1: Session Volume Profiles (Asia, London, NY)
	printBoxRow(cBold+cMagenta+"  🏛️ 1. SESSION VOLUME PROFILES & DELTA BREAKDOWN (AUCTION MAP)"+cReset, w)
	printBoxRow(cGray+"     Session       | Date       | POC Price   | Value Area (VAL - VAH)    | Total Vol | Delta Vol   | Ratio"+cReset, w)
	printBoxRow(cGray+"     ──────────────┼────────────┼─────────────┼───────────────────────────┼───────────┼─────────────┼──────"+cReset, w)

	for _, s := range v.SessionProfiles {
		deltaColor := cGreen
		deltaSign := "+"
		if s.Delta < 0 {
			deltaColor = cRed
			deltaSign = "-"
		}
		ratioColor := cGreen
		if s.DeltaRatio < 0.48 {
			ratioColor = cRed
		} else if s.DeltaRatio <= 0.52 {
			ratioColor = cYellow
		}

		sessName := fmt.Sprintf("%-12s", s.Session)
		dateStr := fmt.Sprintf("%-10s", s.Date)
		pocStr := fmt.Sprintf("%-11s", tvspoof.FormatDeskPrice(s.POC, p))
		vaStr := fmt.Sprintf("%-10s - %-12s", tvspoof.FormatDeskPrice(s.VAL, p), tvspoof.FormatDeskPrice(s.VAH, p))
		totVolStr := fmt.Sprintf("%-9s", formatCompact(s.TotalVolume))
		deltaStr := fmt.Sprintf("%s%s%-9s%s", deltaColor, deltaSign, formatCompact(math.Abs(s.Delta)), cReset)
		ratioStr := fmt.Sprintf("%s%4.1f%%%s", ratioColor, s.DeltaRatio*100.0, cReset)

		printBoxRow(fmt.Sprintf("     %s | %s | %s | %s | %s | %s | %s",
			sessName, dateStr, pocStr, vaStr, totVolStr, deltaStr, ratioStr), w)
	}
	printBoxDivider(w)

	// Section 2: Naked POCs (nPOCs)
	printBoxRow(cBold+cYellow+"  🎯 2. NAKED POC (nPOC) RADAR — HIGH PROBABILITY PRICE MAGNETS"+cReset, w)
	if len(v.NakedPOCs) == 0 {
		printBoxRow(cDim+"     • Tidak ada Naked POC yang aktif di range data 100 bar terakhir (Seluruh POC termitigasi)."+cReset, w)
	} else {
		printBoxRow(cGray+"     nPOC Price    | Origin Session        | Date       | Distance    | Status / Significance"+cReset, w)
		printBoxRow(cGray+"     ──────────────┼───────────────────────┼────────────┼─────────────┼──────────────────────"+cReset, w)
		for _, n := range v.NakedPOCs {
			distSign := "+"
			distColor := cRed
			if n.Distance < 0 {
				distSign = "-"
				distColor = cGreen
			}
			distStr := fmt.Sprintf("%s%s%s pts (%4.2f%%)%s", distColor, distSign, tvspoof.FormatDeskPrice(math.Abs(n.Distance), p), n.DistancePct, cReset)
			statusColor := cBold + cGreen
			if strings.Contains(n.Status, "TESTED") {
				statusColor = cDim + cGray
			}

			printBoxRow(fmt.Sprintf("     %-12s | %-21s | %-10s | %-20s | %s%s (%s)%s",
				tvspoof.FormatDeskPrice(n.Price, p),
				n.Session,
				n.Date,
				distStr,
				statusColor, n.Status, n.Significance, cReset), w)
		}
	}
	printBoxDivider(w)

	// Section 3: High Volume Nodes & Low Volume Nodes
	printBoxRow(cBold+cCyan+"  🧱 3. INSTITUTIONAL VOLUME NODES (HVN WALLS vs LVN VACUUMS)"+cReset, w)
	hvnStr := "None"
	if len(v.ActiveHVNs) > 0 {
		var hList []string
		for _, h := range v.ActiveHVNs {
			hList = append(hList, tvspoof.FormatDeskPrice(h, p))
		}
		hvnStr = strings.Join(hList, "  |  ")
	}
	lvnStr := "None"
	if len(v.ActiveLVNs) > 0 {
		var lList []string
		for _, l := range v.ActiveLVNs {
			lList = append(lList, tvspoof.FormatDeskPrice(l, p))
		}
		lvnStr = strings.Join(lList, "  |  ")
	}
	printBoxRow(fmt.Sprintf("     • %sHigh Volume Nodes (HVN Support/Resistance Shelves)%s : %s%s%s", cBold+cGreen, cReset, cBold+cYellow, hvnStr, cReset), w)
	printBoxRow(fmt.Sprintf("     • %sLow Volume Nodes (LVN Slippage / Fast Vacuum Corridors)%s : %s%s%s", cBold+cMagenta, cReset, cBold+cCyan, lvnStr, cReset), w)
	printBoxDivider(w)

	// Section 4: Cumulative Volume Delta (CVD)
	printBoxRow(cBold+cBlue+"  ⚡ 4. CUMULATIVE VOLUME DELTA (CVD) & ORDER FLOW ABSORPTION"+cReset, w)
	netDeltaColor := cGreen
	if cvd.NetDelta < 0 {
		netDeltaColor = cRed
	}
	printBoxRow(fmt.Sprintf("     • Total Volume Tracked : %s (Buy: %s / Sell: %s)",
		formatCompact(cvd.TotalVolume), formatCompact(cvd.TotalBuyVolume), formatCompact(cvd.TotalSellVolume)), w)
	printBoxRow(fmt.Sprintf("     • Buyer vs Seller Pct  : %sBuyer: %4.1f%%%s  vs  %sSeller: %4.1f%%%s  |  Net Delta: %s%s%s",
		cGreen, cvd.BuyerDominancePct, cReset,
		cRed, cvd.SellerDominancePct, cReset,
		netDeltaColor, formatCompact(cvd.NetDelta), cReset), w)
	printBoxRow(fmt.Sprintf("     • Divergence Signal    : %s%s%s", cBold+cYellow, cvd.DivergenceSignal, cReset), w)
	printBoxRow(fmt.Sprintf("     • Order Flow Context   : %s", cvd.DivergenceDescription), w)
	printBoxDivider(w)

	// Section 5: Strategic Directives
	printBoxRow(cBold+cGreen+"  🎯 5. QUANTITATIVE VOLUME DIRECTIVES & EXECUTION PLAYBOOK"+cReset, w)
	printBoxRow(fmt.Sprintf("     • Directive Status    : %s", cvd.InstitutionalDirective), w)
	printBoxRow(fmt.Sprintf("     • Auction Reversion   : Tunggu rotasi menuju HVN/POC (%s) dan hindari chasing di zona LVN.", tvspoof.FormatDeskPrice(v.CompositePOC, p)), w)
	printBoxFooter(w)
	fmt.Println()
}

func formatCompact(v float64) string {
	absV := math.Abs(v)
	sign := ""
	if v < 0 {
		sign = "-"
	}
	if absV >= 1_000_000 {
		return fmt.Sprintf("%s%.2fM", sign, absV/1_000_000)
	} else if absV >= 1_000 {
		return fmt.Sprintf("%s%.1fk", sign, absV/1_000)
	}
	return fmt.Sprintf("%s%.0f", sign, absV)
}

func printBoxHeader(title, subtitle string, width int) {
	fmt.Printf("╔%s╗\n", strings.Repeat("═", width))
	content := fmt.Sprintf("  %s%-*s%s%s", cBold+cYellow, width-len(subtitle)-4, title, subtitle, cReset)
	printRawBoxRow(content, width)
	fmt.Printf("╠%s╣\n", strings.Repeat("═", width))
}

func printBoxRow(text string, width int) {
	strippedLen := visualLen(text)
	padding := width - strippedLen
	if padding < 0 {
		padding = 0
	}
	fmt.Printf("║%s%s║\n", text, strings.Repeat(" ", padding))
}

func printRawBoxRow(formatted string, width int) {
	strippedLen := visualLen(formatted)
	padding := width - strippedLen
	if padding < 0 {
		padding = 0
	}
	fmt.Printf("║%s%s║\n", formatted, strings.Repeat(" ", padding))
}

func printBoxDivider(width int) {
	fmt.Printf("╟%s╢\n", strings.Repeat("─", width))
}

func printBoxFooter(width int) {
	fmt.Printf("╚%s╝\n", strings.Repeat("═", width))
}

func visualLen(s string) int {
	inEscape := false
	l := 0
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		l++
	}
	return l
}
