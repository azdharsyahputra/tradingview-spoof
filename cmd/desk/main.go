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
	winRateColor := cGreen
	if b.Playbook.EstimatedWinRate < 50 {
		winRateColor = cYellow
	}
	convictionText := fmt.Sprintf("  EXECUTIVE BIAS: %s %s %s  |  EST. WINRATE: %s%.1f%%%s  |  CONVICTION: %s %d%%  |  GRADE: %s %s %s",
		badgeColor, b.ExecutiveBias, cReset,
		cBold+winRateColor, b.Playbook.EstimatedWinRate, cReset,
		gaugeBar, b.ConvictionScore,
		cBold+cYellow, b.ConvictionGrade, cReset,
	)
	printBoxRow(convictionText, w)
	fmt.Println(cCyan + "╠" + strings.Repeat("═", w) + "╣" + cReset)

	// Section 1: Intermarket Macro Drivers
	printBoxRow(cBold+cBlue+"  🌐 1. INTERMARKET MACRO RADAR & REGIME"+cReset, w)
	printBoxRow(fmt.Sprintf("     • Macro Regime : %s", b.Intermarket.MacroRegime), w)
	printBoxRow(fmt.Sprintf("     • Rates Signal : %s", b.Intermarket.RatesSignalSource), w)

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
	if b.Intermarket.US10YReal != nil {
		r := b.Intermarket.US10YReal
		printBoxRow(fmt.Sprintf("     • %-16s : Yield: %-7.3f%% (Δ %+.1f bp, %s, as of %s) ➔ %s",
			r.Name, r.Price, r.ChangeBps, r.Lookback, r.AsOf, r.ImpactOnSym), w)
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

	// Section 2: Multi-Timeframe Matrix (M1, M5, M15, H1, H4, D1)
	printBoxRow(cBold+cCyan+"  ⏱️  2. MULTI-TIMEFRAME (MTF) CONFLUENCE & STRUCTURE MATRIX"+cReset, w)
	printBoxRow(cGray+"     TF            | Bias         | Structure / Key Flow          | RSI(14) | EMA(21)   | Candle Flow"+cReset, w)
	printBoxRow(cDarkGray+"     ──────────────┼──────────────┼───────────────────────────────┼─────────┼───────────┼───────────────────"+cReset, w)
	for _, f := range b.MTF.Frames {
		biasColor := cYellow
		if strings.Contains(f.Bias, "BULL") {
			biasColor = cGreen
		} else if strings.Contains(f.Bias, "BEAR") {
			biasColor = cRed
		}
		row := fmt.Sprintf("     %-13s | %s%-12s%s | %-29s | %-7.1f | %-9.2f | %s",
			f.TF, biasColor, f.Bias, cReset, f.Structure, f.RSI, f.EMA21, f.Candle)
		printBoxRow(row, w)
	}
	printBoxRow(cDarkGray+"     ───────────────────────────────────────────────────────────────────────────────────────────"+cReset, w)
	printBoxRow(fmt.Sprintf("     • MTF Confluence : %s%s%s", cBold+cYellow, b.MTF.AlignmentSummary, cReset), w)
	printBoxRow(cBold+cBlue+"     • Candle Anatomy & Morphology Breakdown (Recent Bars):"+cReset, w)
	for _, f := range b.MTF.Frames {
		if f.TF == "M1 (1-Min)" || f.TF == "M5 (5-Min)" || f.TF == "M15 (15-Min)" || f.TF == "H1 (1-Hour)" {
			tfShort := strings.Split(f.TF, " ")[0]
			for idx, cm := range f.RecentCandles {
				barOffset := idx - (len(f.RecentCandles) - 1)
				barLabel := "Live"
				if barOffset < 0 {
					barLabel = fmt.Sprintf("-%d", -barOffset)
				}
				chgColor := cGreen
				chgSign := "+"
				if cm.Change < 0 {
					chgColor = cRed
					chgSign = ""
				}
				row := fmt.Sprintf("       [%-3s %-4s %s UTC] %s ➔ %s (%s%s%.2f%s) ➔ %s",
					tfShort, barLabel, cm.TimeStr,
					tvspoof.FormatDeskPrice(cm.Open, p), tvspoof.FormatDeskPrice(cm.Close, p),
					chgColor, chgSign, cm.Change, cReset, cm.Summary)
				printBoxRow(row, w)
			}
		}
	}
	fmt.Println(cCyan + "╟" + strings.Repeat("─", w) + "╢" + cReset)

	// Section 3: Volume Profile & Institutional Liquidity
	printBoxRow(cBold+cMagenta+"  📊 3. VOLUME PROFILE (VPVR) & INSTITUTIONAL LIQUIDITY MAP"+cReset, w)
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
	if b.SMC.StructureSeq.LastHigh.Price > 0 || b.SMC.StructureSeq.LastLow.Price > 0 {
		swingLine := fmt.Sprintf("     • Swing Points (HH/LL) : %s%s%s (High: %s @ %s | Low: %s @ %s)",
			cBold+cYellow, b.SMC.StructureSeq.SequenceFlow, cReset,
			b.SMC.StructureSeq.LastHigh.Type, tvspoof.FormatDeskPrice(b.SMC.StructureSeq.LastHigh.Price, p),
			b.SMC.StructureSeq.LastLow.Type, tvspoof.FormatDeskPrice(b.SMC.StructureSeq.LastLow.Price, p))
		printBoxRow(swingLine, w)
	}

	// FVG display
	fvgLine := fmt.Sprintf("     • Fair Value Gaps      : Bullish FVG: %d  |  Bearish FVG: %d", len(b.SMC.BullishFVGs), len(b.SMC.BearishFVGs))
	if b.SMC.UnfilledFVGPrice > 0 {
		fvgLine += fmt.Sprintf("  |  Nearest Unfilled: %s", tvspoof.FormatDeskPrice(b.SMC.UnfilledFVGPrice, p))
	}
	printBoxRow(fvgLine, w)

	// Order Block display
	obLine := "     • Order Blocks         :"
	if b.SMC.BullishOB != "" {
		obLine += fmt.Sprintf(" %s🟢 Bullish OB [%s]%s", cBold+cGreen, b.SMC.BullishOB, cReset)
	}
	if b.SMC.BearishOB != "" {
		if b.SMC.BullishOB != "" {
			obLine += "  |"
		}
		obLine += fmt.Sprintf(" %s🔴 Bearish OB [%s]%s", cBold+cRed, b.SMC.BearishOB, cReset)
	}
	if b.SMC.BullishOB == "" && b.SMC.BearishOB == "" {
		obLine += " None detected"
	}
	printBoxRow(obLine, w)

	// BOS display
	bosLine := fmt.Sprintf("     • Break of Structure   : %s", b.SMC.BOSDirection)
	if b.SMC.BOSLevel > 0 {
		bosLine += fmt.Sprintf(" @ %s", tvspoof.FormatDeskPrice(b.SMC.BOSLevel, p))
	}
	printBoxRow(bosLine, w)

	// ChoCH display
	if b.SMC.ChoCHDetected {
		chochLine := fmt.Sprintf("     • Change of Character  : %s%s @ %s%s",
			cBold+cYellow, b.SMC.ChoCHDirection, tvspoof.FormatDeskPrice(b.SMC.ChoCHLevel, p), cReset)
		printBoxRow(chochLine, w)
	}

	fmt.Println(cCyan + "╟" + strings.Repeat("─", w) + "╢" + cReset)

	// Section 4: Institutional Support & Resistance (S/R) & Key Levels
	printBoxRow(cBold+cCyan+"  📐 4. INSTITUTIONAL SUPPORT & RESISTANCE (S/R) & KEY LEVELS"+cReset, w)
	rangeText := fmt.Sprintf("     • Today Range       : %s%s%s (High) - %s%s%s (Low) | Spread: %s%.2f pts%s",
		cBold+cRed, tvspoof.FormatDeskPrice(b.Levels.TodayHigh, p), cReset,
		cBold+cGreen, tvspoof.FormatDeskPrice(b.Levels.TodayLow, p), cReset,
		cBold+cYellow, b.Levels.TodayRange, cReset)
	printBoxRow(rangeText, w)

	pivotText := fmt.Sprintf("     • Daily Pivot Point : %s%s%s  |  Daily Open: %s%s%s",
		cBold+cYellow, tvspoof.FormatDeskPrice(b.Levels.DailyPivot, p), cReset,
		cBold+cCyan, tvspoof.FormatDeskPrice(b.Levels.DailyOpen, p), cReset)
	printBoxRow(pivotText, w)

	// Resistances line
	rLine := "     • Major Resistances : "
	for i, r := range b.Levels.Resistances {
		if i > 0 {
			rLine += " | "
		}
		rLine += fmt.Sprintf("%s: %s%s%s (%s+%.1f pts%s)",
			strings.Split(r.Level, " ")[0],
			cBold+cRed, tvspoof.FormatDeskPrice(r.Price, p), cReset,
			cRed, r.DistancePts, cReset)
	}
	printBoxRow(rLine, w)

	// Supports line
	sLine := "     • Major Supports    : "
	for i, s := range b.Levels.Supports {
		if i > 0 {
			sLine += " | "
		}
		sLine += fmt.Sprintf("%s: %s%s%s (%s-%.1f pts%s)",
			strings.Split(s.Level, " ")[0],
			cBold+cGreen, tvspoof.FormatDeskPrice(s.Price, p), cReset,
			cGreen, s.DistancePts, cReset)
	}
	printBoxRow(sLine, w)

	fmt.Println(cCyan + "╟" + strings.Repeat("─", w) + "╢" + cReset)

	// Section 5: Institutional Supply & Demand Zones
	printBoxRow(cBold+cMagenta+"  ⚡ 5. INSTITUTIONAL SUPPLY & DEMAND ZONES (ORDER FLOW MAP)"+cReset, w)
	printBoxRow(cGray+"     Zone Type     | Price Range               | Distance  | Strength              | Status"+cReset, w)
	printBoxRow(cDarkGray+"     ──────────────┼───────────────────────────┼───────────┼───────────────────────┼──────────────────────────"+cReset, w)

	// Supply Zones
	for idx, sz := range b.Levels.SupplyZones {
		distStr := fmt.Sprintf("+%.2f pts", sz.DistancePts)
		if sz.DistancePts == 0 {
			distStr = "In Zone"
		}
		row := fmt.Sprintf("     %s🔴 Supply #%d%s  | %-25s | %-9s | %-21s | %s",
			cBold+cRed, idx+1, cReset,
			fmt.Sprintf("%s - %s", tvspoof.FormatDeskPrice(sz.BottomPrice, p), tvspoof.FormatDeskPrice(sz.TopPrice, p)),
			distStr, sz.Strength, sz.Status)
		printBoxRow(row, w)
	}

	// Demand Zones
	for idx, dz := range b.Levels.DemandZones {
		distStr := fmt.Sprintf("-%.2f pts", dz.DistancePts)
		if dz.DistancePts == 0 {
			distStr = "In Zone"
		}
		row := fmt.Sprintf("     %s🟢 Demand #%d%s  | %-25s | %-9s | %-21s | %s",
			cBold+cGreen, idx+1, cReset,
			fmt.Sprintf("%s - %s", tvspoof.FormatDeskPrice(dz.BottomPrice, p), tvspoof.FormatDeskPrice(dz.TopPrice, p)),
			distStr, dz.Strength, dz.Status)
		printBoxRow(row, w)
	}

	fmt.Println(cCyan + "╟" + strings.Repeat("─", w) + "╢" + cReset)

	// Section 6: Actionable Institutional Trade Playbook
	printBoxRow(cBold+cYellow+"  🎯 6. ACTIONABLE PROPRIETARY TRADE PLAYBOOK"+cReset, w)
	printBoxRow(fmt.Sprintf("     • Playbook Plan     : %s%s%s", cBold+cGreen, b.Playbook.PrimaryPlan, cReset), w)
	printBoxRow(fmt.Sprintf("     • Optimal Entry Zone: %s%s%s", cBold, b.Playbook.OptimalEntryZone, cReset), w)
	printBoxRow(fmt.Sprintf("     • Invalidation (SL) : %s%s%s (Structural Stop Loss below VAL / Asia Low)",
		cBold+cRed, tvspoof.FormatDeskPrice(b.Playbook.InvalidationPrice, p), cReset), w)
	printBoxRow(fmt.Sprintf("     • Target 1 (TP1)    : %s%s%s (Point of Control / Value Mean Reversion)",
		cBold+cGreen, tvspoof.FormatDeskPrice(b.Playbook.Target1Price, p), cReset), w)
	printBoxRow(fmt.Sprintf("     • Target 2 (TP2)    : %s%s%s (Day High / Liquidity Pool Expansion)",
		cBold+cGreen, tvspoof.FormatDeskPrice(b.Playbook.Target2Price, p), cReset), w)
	printBoxRow(fmt.Sprintf("     • Risk / Reward     : %s1 : %.2f%s  |  Est. Win Rate: %s%.1f%%%s  |  Size: %s%.2f Lots%s ($%.0f Account)",
		cBold+cYellow, b.Playbook.RiskRewardRatio, cReset,
		cBold+cGreen, b.Playbook.EstimatedWinRate, cReset,
		cBold+cCyan, b.Playbook.RecommendedLots, cReset, balance), w)
	printBoxRow(fmt.Sprintf("     • Max Risk Budget   : $%.2f (%.1f%%)  |  Expected Value (EV): %s+$%.2f%s  |  Edge: %s+EV Mathematical Edge%s",
		b.Playbook.RiskAmountUSD, risk, cBold+cGreen, b.Playbook.ExpectedValueUSD, cReset, cBold+cGreen, cReset), w)
	fmt.Println(cCyan + "╟" + strings.Repeat("─", w) + "╢" + cReset)

	// Section 7: Economic Calendar Catalyst Watch
	printBoxRow(cBold+"  📅 7. UPCOMING CATALYST WATCH (TODAY)"+cReset, w)
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

	// Section 8: Registered Trade Plans Watchlist
	printBoxRow(cBold+cYellow+"  📋 8. REGISTERED DESK TRADE PLANS (tradeplans.json)"+cReset, w)
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
