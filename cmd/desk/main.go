package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	tvspoof "github.com/azdharsyahputra/tradingview-spoof"
)

// ANSI Color definitions
const (
	cReset      = "\033[0m"
	cBold       = "\033[1m"
	cDim        = "\033[2m"
	cGreen      = "\033[38;5;48m"
	cRed        = "\033[38;5;203m"
	cYellow     = "\033[38;5;220m"
	cCyan       = "\033[38;5;75m"
	cBlue       = "\033[38;5;39m"
	cMagenta    = "\033[38;5;177m"
	cGray       = "\033[38;5;244m"
	cDarkGray   = "\033[38;5;238m"
	cBgGreen    = "\033[48;5;48m\033[38;5;232m"
	cBgRed      = "\033[48;5;203m\033[38;5;255m"
	cBgYellow   = "\033[48;5;220m\033[38;5;232m"
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
	symbolFlag := flag.String("symbol", "OANDA:XAUUSD", "Target TradingView symbol (e.g. xauusd, btc, eurusd)")
	balanceFlag := flag.Float64("balance", 10000.0, "Portfolio Account Balance in USD")
	riskFlag := flag.Float64("risk", 1.0, "Risk percentage per trade (e.g. 1.0 for 1%)")
	flag.Parse()

	symbol := *symbolFlag
	balance := *balanceFlag
	risk := *riskFlag

	args := flag.Args()
	if len(args) >= 1 {
		symbol = args[0]
	}
	if len(args) >= 2 {
		if b, err := strconv.ParseFloat(args[1], 64); err == nil {
			balance = b
		}
	}
	if len(args) >= 3 {
		if r, err := strconv.ParseFloat(args[2], 64); err == nil {
			risk = r
		}
	}

	normSym := tvspoof.NormalizeDeskSymbol(symbol)

	fmt.Print("\033[?25l") // Hide cursor
	fmt.Printf("%s[⚡ ZTERM HEDGE FUND DESK]%s Menjalankan kalkulasi kuantitatif untuk %s%s%s...\n", cBold+cCyan, cReset, cBold+cYellow, normSym, cReset)

	client := tvspoof.NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	briefing, err := client.AnalyzeDesk(ctx, normSym, balance, risk)
	fmt.Print("\033[?25h") // Restore cursor

	if err != nil {
		fmt.Printf("\n%s[ERROR]%s Gagal menghasilkan Desk Briefing: %v\n", cRed+cBold, cReset, err)
		os.Exit(1)
	}

	renderDeskBriefing(briefing, balance, risk)
}

func renderDeskBriefing(b *tvspoof.DeskBriefing, balance, risk float64) {
	p := b.Precision
	w := 96 // inner width

	fmt.Println()
	// Top Border
	fmt.Println(cCyan + "╔" + strings.Repeat("═", w) + "╗" + cReset)

	// Header Title Bar
	timeStr := fmt.Sprintf("%s UTC", b.Timestamp.Format("2006-01-02 15:04:05"))
	headerText := fmt.Sprintf("  🏛️  %sZTERM PROPRIETARY DESK%s  •  EXECUTIVE INTELLIGENCE BRIEFING      %s%s%s  ",
		cBold, cReset, cGray, timeStr, cReset)
	printBoxRow(headerText, w)
	fmt.Println(cCyan + "╠" + strings.Repeat("═", w) + "╣" + cReset)

	// Symbol & Price Ribbon
	chgSign := "+"
	chgColor := cGreen
	if b.DayChange < 0 {
		chgSign = ""
		chgColor = cRed
	}
	symPriceText := fmt.Sprintf("  TARGET ASSET: %s%s%s  |  PRICE: %s%s%s  |  DAY CHG: %s%s%.2f (%s%.2f%%)%s",
		cBold+cYellow, b.Symbol, cReset,
		cBold+chgColor, tvspoof.FormatDeskPrice(b.CurrentPrice, p), cReset,
		cBold+chgColor, chgSign, b.DayChange, chgSign, b.DayChangePerc, cReset,
	)
	printBoxRow(symPriceText, w)

	// Conviction Gauge
	gaugeBar := renderGauge(b.ConvictionScore, 18)
	badgeColor := cBgGreen
	if b.ConvictionScore < 50 {
		badgeColor = cBgRed
	} else if b.ConvictionScore < 65 {
		badgeColor = cBgYellow
	}
	convictionText := fmt.Sprintf("  EXECUTIVE BIAS: %s %s %s  |  CONVICTION: %s %d%%  |  GRADE: %s %s %s",
		badgeColor, b.ExecutiveBias, cReset,
		gaugeBar, b.ConvictionScore,
		cBold+cYellow, b.ConvictionGrade, cReset,
	)
	printBoxRow(convictionText, w)
	fmt.Println(cCyan + "╠" + strings.Repeat("═", w) + "╣" + cReset)

	// Section 1: Intermarket Macro Drivers
	printBoxRow(cBold+cBlue+"  🌐 1. INTERMARKET MACRO RADAR & REGIME"+cReset, w)
	printBoxRow(fmt.Sprintf("     • Macro Regime : %s", b.Intermarket.MacroRegime), w)

	if b.Intermarket.DXY != nil {
		d := b.Intermarket.DXY
		printBoxRow(fmt.Sprintf("     • %-16s : Price: %-7.2f (Chg: %+.2f%%) ➔ %s",
			d.Name, d.Price, d.ChangePerc, d.ImpactOnSym), w)
	}
	if b.Intermarket.US10Y != nil {
		u := b.Intermarket.US10Y
		printBoxRow(fmt.Sprintf("     • %-16s : Yield: %-7.3f%% (Chg: %+.2f%%) ➔ %s",
			u.Name, u.Price, u.ChangePerc, u.ImpactOnSym), w)
	}
	if b.Intermarket.SPX != nil {
		s := b.Intermarket.SPX
		printBoxRow(fmt.Sprintf("     • %-16s : Price: %-7.2f (Chg: %+.2f%%) ➔ %s",
			s.Name, s.Price, s.ChangePerc, s.ImpactOnSym), w)
	}
	if b.Intermarket.Silver != nil {
		ag := b.Intermarket.Silver
		printBoxRow(fmt.Sprintf("     • %-16s : Price: %-7.2f (Chg: %+.2f%%) ➔ %s",
			ag.Name, ag.Price, ag.ChangePerc, ag.ImpactOnSym), w)
	}
	fmt.Println(cCyan + "╟" + strings.Repeat("─", w) + "╢" + cReset)

	// Section 2: Volume Profile & Institutional Liquidity
	printBoxRow(cBold+cMagenta+"  📊 2. VOLUME PROFILE (VPVR) & INSTITUTIONAL LIQUIDITY MAP"+cReset, w)
	vpLine := fmt.Sprintf("     • POC (Highest Vol): %s%s%s  |  VAH (Value High): %s%s%s  |  VAL (Value Low): %s%s%s",
		cBold+cYellow, tvspoof.FormatDeskPrice(b.VolumeProfile.POC, p), cReset,
		cBold+cRed, tvspoof.FormatDeskPrice(b.VolumeProfile.VAH, p), cReset,
		cBold+cGreen, tvspoof.FormatDeskPrice(b.VolumeProfile.VAL, p), cReset,
	)
	printBoxRow(vpLine, w)

	asiaSweptStr := cDarkGray + "NO" + cReset
	if b.Liquidity.AsiaSweptLow {
		asiaSweptStr = cBold + cGreen + "YES (BULLISH REVERSAL SWEEP)" + cReset
	}
	printBoxRow(fmt.Sprintf("     • Asian Range High/Low : %s - %s  |  Asia Low Swept: %s",
		tvspoof.FormatDeskPrice(b.Liquidity.AsiaHigh, p), tvspoof.FormatDeskPrice(b.Liquidity.AsiaLow, p), asiaSweptStr), w)

	printBoxRow(fmt.Sprintf("     • Previous Day Range   : %s (PDH) - %s (PDL)  |  Daily Open: %s",
		tvspoof.FormatDeskPrice(b.Liquidity.PDH, p), tvspoof.FormatDeskPrice(b.Liquidity.PDL, p), tvspoof.FormatDeskPrice(b.Liquidity.DailyOpen, p)), w)

	printBoxRow(fmt.Sprintf("     • SMC Pricing Zone     : %s", b.SMC.PricingZone), w)
	printBoxRow(fmt.Sprintf("     • Market Structure     : %s", b.SMC.MarketStructure), w)
	fmt.Println(cCyan + "╟" + strings.Repeat("─", w) + "╢" + cReset)

	// Section 3: Actionable Institutional Trade Playbook
	printBoxRow(cBold+cYellow+"  🎯 3. ACTIONABLE PROPRIETARY TRADE PLAYBOOK"+cReset, w)
	printBoxRow(fmt.Sprintf("     • Playbook Plan     : %s%s%s", cBold+cGreen, b.Playbook.PrimaryPlan, cReset), w)
	printBoxRow(fmt.Sprintf("     • Optimal Entry Zone: %s%s%s", cBold, b.Playbook.OptimalEntryZone, cReset), w)
	printBoxRow(fmt.Sprintf("     • Invalidation (SL) : %s%s%s (Structural Stop Loss below VAL / Asia Low)",
		cBold+cRed, tvspoof.FormatDeskPrice(b.Playbook.InvalidationPrice, p), cReset), w)
	printBoxRow(fmt.Sprintf("     • Target 1 (TP1)    : %s%s%s (Point of Control / Value Mean Reversion)",
		cBold+cGreen, tvspoof.FormatDeskPrice(b.Playbook.Target1Price, p), cReset), w)
	printBoxRow(fmt.Sprintf("     • Target 2 (TP2)    : %s%s%s (Day High / Liquidity Pool Expansion)",
		cBold+cGreen, tvspoof.FormatDeskPrice(b.Playbook.Target2Price, p), cReset), w)
	printBoxRow(fmt.Sprintf("     • Risk / Reward     : %s1 : %.2f%s  |  Recommended Position Size: %s%.2f Lots%s ($%.0f Account)",
		cBold+cYellow, b.Playbook.RiskRewardRatio, cReset,
		cBold+cCyan, b.Playbook.RecommendedLots, cReset, balance), w)
	printBoxRow(fmt.Sprintf("     • Max Risk Budget   : $%.2f (%.1f%%)  |  Expected Value (EV): %s+$%.2f%s",
		b.Playbook.RiskAmountUSD, risk, cBold+cGreen, b.Playbook.ExpectedValueUSD, cReset), w)
	fmt.Println(cCyan + "╟" + strings.Repeat("─", w) + "╢" + cReset)

	// Section 4: Economic Calendar Catalyst Watch
	printBoxRow(cBold+"  📅 4. UPCOMING CATALYST WATCH (TODAY)"+cReset, w)
	if len(b.UpcomingEvents) == 0 {
		printBoxRow("     • No immediate high-impact USD economic events scheduled for today.", w)
	} else {
		for i, ev := range b.UpcomingEvents {
			if i >= 3 {
				break
			}
			t := time.Unix(ev.Timestamp, 0).UTC().Format("15:04")
			printBoxRow(fmt.Sprintf("     • [%s UTC] %s (%s) - Forecast: %s | Prev: %s",
				t, ev.Title, ev.Impact, ev.Forecast, ev.Previous), w)
		}
	}
	fmt.Println(cCyan + "╟" + strings.Repeat("─", w) + "╢" + cReset)

	// Section 5: Registered Trade Plans Watchlist
	printBoxRow(cBold+cYellow+"  📋 5. REGISTERED DESK TRADE PLANS (tradeplans.json)"+cReset, w)
	plans, _ := tvspoof.LoadTradePlans("")
	if len(plans) == 0 {
		printBoxRow("     • No active trade plans saved. Manage plans in tradeplans.json", w)
	} else {
		for _, pl := range plans {
			stColor := cYellow
			if strings.ToUpper(pl.Status) == "RUNNING" {
				stColor = cGreen
			}
			printBoxRow(fmt.Sprintf("     • [%s%s%s] %s%s%s (%s) | Entry: %.2f | SL: %s%.2f%s | TP1: %s%.2f%s | RRR: %s",
				cBold+stColor, pl.Status, cReset,
				cBold, pl.Asset, cReset,
				pl.Direction, pl.Entry,
				cRed, pl.SL, cReset,
				cGreen, pl.TP1, cReset,
				pl.RRR,
			), w)
			if pl.Notes != "" {
				printBoxRow(fmt.Sprintf("       ↳ Note: %s", pl.Notes), w)
			}
		}
	}

	// Bottom Border
	fmt.Println(cCyan + "╚" + strings.Repeat("═", w) + "╝" + cReset)
	fmt.Printf(cGray+"  💡 Tip: Gunakan `go run ./cmd/desk [symbol] [balance] [risk%%]` untuk kustomisasi modal & risiko.\n\n"+cReset)
}

func renderGauge(score int, width int) string {
	filled := int((float64(score) / 100.0) * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	if score >= 75 {
		return cGreen + "[" + bar + "]" + cReset
	} else if score >= 50 {
		return cYellow + "[" + bar + "]" + cReset
	}
	return cRed + "[" + bar + "]" + cReset
}
