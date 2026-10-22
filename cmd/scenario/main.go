// Command scenario is the market-reaction & liquidity advisor: structure
// (HH/HL/LH/LL, BOS/CHoCH), channel, liquidity pools, active FVGs, recent
// reactions, and if-then scenarios at the nearest levels. Non-FAR scenarios
// are logged to shadow_trades.json so cmd/shadow can grade the calls.
//
//	go run ./cmd/scenario xauusd            # plain text for the agent
//	go run ./cmd/scenario xauusd --json     # full JSON
//	go run ./cmd/scenario xauusd --no-shadow
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	tvspoof "github.com/azdharsyahputra/tradingview-spoof"
)

func main() {
	symbol, jsonOut, shadow := "xauusd", false, true
	expiry := 12 * time.Hour
	for _, a := range os.Args[1:] {
		switch {
		case a == "--json" || a == "-json":
			jsonOut = true
		case a == "--no-shadow" || a == "-no-shadow":
			shadow = false
		case strings.HasPrefix(a, "--expiry="):
			if d, err := time.ParseDuration(strings.TrimPrefix(a, "--expiry=")); err == nil {
				expiry = d
			}
		default:
			symbol = a
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rep, err := tvspoof.NewClient().AnalyzeScenarios(ctx, symbol)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	var logged []tvspoof.ShadowTrade
	if shadow {
		existing, err := tvspoof.LoadShadowTrades(tvspoof.DefaultShadowPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "shadow journal:", err)
		} else {
			logged = tvspoof.NewShadowTrades(rep, existing, expiry, false)
			if len(logged) > 0 {
				if err := tvspoof.SaveShadowTrades(tvspoof.DefaultShadowPath, append(existing, logged...)); err != nil {
					fmt.Fprintln(os.Stderr, "shadow journal:", err)
				}
			}
		}
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]interface{}{"report": rep, "shadow_logged": logged})
		return
	}
	printText(rep, logged)
}

func printText(r *tvspoof.ScenarioReport, logged []tvspoof.ShadowTrade) {
	p := r.Precision
	f := func(v float64) string { return fmt.Sprintf("%.*f", p, v) }
	fmt.Printf("SCENARIO %s  price %s  %s UTC  session %s  ATR M15 %s / M5 %s\n", r.Symbol, f(r.Price), r.Time.Format("2006-01-02 15:04"), r.Session, f(r.ATR15), f(r.ATR5))
	fmt.Println("SUMMARY:", r.Summary)

	fmt.Println("\nSTRUCTURE (closed candles):")
	for _, s := range r.Structure {
		fmt.Printf("  %-4s %-16s %s\n", s.TF, s.Trend, s.Sequence)
		if s.LastEvent != "" {
			fmt.Printf("       last: %s\n", s.LastEvent)
		}
		if s.FlipNote != "" {
			fmt.Printf("       flip: %s\n", s.FlipNote)
		}
	}
	if c := r.Channel; c != nil {
		fmt.Printf("CHANNEL M15: %s %s  upper %s  lower %s  pos %.0f%%\n", c.Kind, c.State, f(c.Upper), f(c.Lower), c.Position)
	}
	x := r.Context
	fmt.Printf("CONTEXT: move 1h %+.1f ATR / 4h %+.1f ATR | streak M5 %+d | body ratio %.2f | rel vol %.1fx\n", x.Move1hATR, x.Move4hATR, x.StreakM5, x.BodyRatio, x.RelVolLast)
	fmt.Printf("         ADR %s, today %s (%.0f%% used) | D1 %s (%+.1f ATR D1 vs EMA20) | H4 %s (EMA50) | %s\n", f(x.ADR), f(x.TodayRange), x.ADRUsedPct, x.D1Trend, x.D1DistATR, x.H4Trend, x.Killzone)
	if x.DXYNote != "" {
		fmt.Println("         " + x.DXYNote)
	}
	fmt.Printf("FILTER:  %s (vol M15 %.1fx) | lokasi %s (%+.1f ATR vs EMA20 M15) | swing bias %s (EMA20 D1 %+.2f ATR/3d, mom 24h %+.1f)\n",
		activityWord(x.Activity), x.RelVolM15, locationWord(x.EMA15DistATR), x.EMA15DistATR, x.SwingBias, x.D1SlopeATR, x.Mom24h)
	if x.Box5State != "" {
		fmt.Printf("         box 5 hari %s–%s → %s\n", f(x.Box5Low), f(x.Box5High), boxWord(x.Box5State))
	}

	fmt.Println("\nLIQUIDITY ABOVE (buy-side / atap):")
	for _, l := range r.Above {
		fmt.Printf("  %s  +%.1f ATR  x%d  %s%s%s\n", f(l.Price), l.DistATR, l.Strength, strings.Join(l.Names, " + "), swept(l.Swept), touches(l))
	}
	fmt.Println("LIQUIDITY BELOW (sell-side / lantai):")
	for _, l := range r.Below {
		fmt.Printf("  %s  -%.1f ATR  x%d  %s%s%s\n", f(l.Price), l.DistATR, l.Strength, strings.Join(l.Names, " + "), swept(l.Swept), touches(l))
	}

	if len(r.FVGs) > 0 {
		fmt.Println("\nACTIVE FVG:")
		for _, z := range r.FVGs {
			kind := "BEAR"
			if z.Bull {
				kind = "BULL"
			}
			fmt.Printf("  %s %s %s-%s (%s UTC)\n", z.TF, kind, f(z.Bottom), f(z.Top), time.Unix(z.Time, 0).UTC().Format("Jan02 15:04"))
		}
	}
	if len(r.Reactions) > 0 {
		fmt.Println("\nRECENT REACTIONS (last 12 M5):")
		for _, e := range r.Reactions {
			fmt.Printf("  %s %-10s %s %s\n", time.Unix(e.Time, 0).UTC().Format("15:04"), e.Kind, f(e.Price), e.Level)
		}
	}

	fmt.Println("\nSCENARIOS (if-then):")
	for _, s := range r.Scenarios {
		fmt.Printf("  [%s] %s %s %s @ %s (%s) — %s, %.1f ATR\n", s.ID, s.Priority, s.Setup, s.Side, f(s.Level), s.LevelName, s.Status, s.DistATR)
		fmt.Printf("      trigger : %s\n", s.Trigger)
		spec := tvspoof.ContractSpecFor(r.Symbol, r.Price)
		pp := func(a, b float64) float64 { return spec.Pips(math.Abs(a - b)) }
		fmt.Printf("      SL est  : %s (%.0f pips) | TP1 %s (%.0f pips) | TP2 %s (%.0f pips) (%s)\n", f(s.EstSL), pp(s.EstSL, s.Level), f(s.Target1), pp(s.Target1, s.Level), f(s.Target2), pp(s.Target2, s.Level), s.TargetNote)
		fmt.Printf("      invalid : %s\n", s.Invalid)
	}
	if len(logged) > 0 {
		var ids []string
		for _, t := range logged {
			ids = append(ids, t.ID+" "+t.ScenarioID)
		}
		fmt.Println("\nSHADOW LOGGED:", strings.Join(ids, ", "))
	}
}

func swept(b bool) string {
	if b {
		return "  (swept today)"
	}
	return ""
}

func touches(l tvspoof.LiquidityLevel) string {
	if l.Touches == 0 {
		return "  [FRESH]"
	}
	return fmt.Sprintf("  [tested %dx, last %dm ago]", l.Touches, l.LastTouchMin)
}

func activityWord(a string) string {
	switch a {
	case "SIAGA":
		return "⚡ SIAGA — gerak besar ≤1 jam ~80%"
	case "SEPI":
		return "💤 SEPI — gerak besar ≤1 jam ~40%, jangan entry"
	}
	return "normal"
}

func locationWord(d float64) string {
	if math.Abs(d) >= 1.5 {
		return "TEPI"
	}
	return "TENGAH (WAIT)"
}

func boxWord(s string) string {
	switch s {
	case "ABOVE":
		return "DI ATAS box (jangan fade/sell counter)"
	case "BELOW":
		return "DI BAWAH box (jangan fade/buy counter)"
	}
	return "di dalam box (fade di tepi boleh)"
}
