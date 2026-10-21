// Command shadow grades the advisor's paper trades.
//
//	go run ./cmd/shadow check   # replay M5 candles on open shadow trades, write lessons for resolved ones
//	go run ./cmd/shadow stats   # track record per setup / side / alignment
//	go run ./cmd/shadow list    # open (pending / active) shadow trades
//	go run ./cmd/shadow backfill xauusd   # replay the advisor over ~17 days of M5 and grade every scenario
//
// Append --json for machine output.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tvspoof "github.com/azdharsyahputra/tradingview-spoof"
)

func main() {
	cmd, jsonOut := "check", false
	var rest []string
	for _, a := range os.Args[1:] {
		switch {
		case a == "--json" || a == "-json":
			jsonOut = true
		case cmd == "check" && len(rest) == 0 && (a == "check" || a == "stats" || a == "list" || a == "backfill"):
			cmd = a
		default:
			rest = append(rest, a)
		}
	}
	if cmd == "backfill" {
		backfill(rest, jsonOut)
		return
	}
	trades, err := tvspoof.LoadShadowTrades(tvspoof.DefaultShadowPath)
	if err != nil {
		fail(err)
	}
	switch cmd {
	case "check":
		check(trades, jsonOut)
	case "stats":
		bf, _ := tvspoof.LoadShadowTrades("shadow_backfill.json")
		live, hist := tvspoof.ShadowStats(trades), tvspoof.ShadowStats(bf)
		emit(jsonOut, map[string]interface{}{"live": live, "backfill": hist}, func() {
			fmt.Println("LIVE shadow record:")
			printStats(live)
			if len(bf) > 0 {
				fmt.Println("\nBACKFILL record (historical replay, shadow_backfill.json):")
				printStats(hist)
			}
		})
	case "list":
		var open []tvspoof.ShadowTrade
		for _, t := range trades {
			if t.Open() {
				open = append(open, t)
			}
		}
		emit(jsonOut, open, func() { printTrades(open) })
	default:
		fail(fmt.Errorf("unknown command %q (check | stats | list)", cmd))
	}
}

func check(trades []tvspoof.ShadowTrade, jsonOut bool) {
	now := time.Now().UTC()
	earliest := map[string]int64{}
	for _, t := range trades {
		if t.Open() && (earliest[t.Symbol] == 0 || t.CreatedAt < earliest[t.Symbol]) {
			earliest[t.Symbol] = t.CreatedAt
		}
	}
	client := tvspoof.NewClient()
	m5 := map[string][]tvspoof.Bar{}
	for sym, from := range earliest {
		n := int(now.Unix()-from)/300 + 50
		if n > 5000 {
			n = 5000
		}
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		bars, err := client.GetHistory(ctx, sym, "5", n)
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", sym, err)
			continue
		}
		m5[sym] = bars
	}

	var resolved []tvspoof.ShadowTrade
	for i := range trades {
		t := &trades[i]
		bars, ok := m5[t.Symbol]
		if !t.Open() || !ok || len(bars) == 0 {
			continue
		}
		spec := tvspoof.ContractSpecFor(t.Symbol, bars[len(bars)-1].Close)
		tvspoof.EvaluateShadow(t, bars, now, spec.TypicalSpread)
		if !t.Open() {
			resolved = append(resolved, *t)
		}
	}

	// Lessons for resolved trades that actually triggered.
	stats := tvspoof.ShadowStats(trades)
	lessons, err := tvspoof.LoadLessons("")
	if err != nil {
		fail(err)
	}
	seq := 0
	for _, l := range lessons {
		if strings.HasPrefix(l.ID, "SHADOW-") {
			seq++
		}
	}
	var written []string
	for i := range trades {
		t := &trades[i]
		if t.Open() || !t.Triggered() || t.LessonID != "" {
			continue
		}
		seq++
		id := fmt.Sprintf("SHADOW-%03d", seq)
		if err := tvspoof.AppendLesson("", tvspoof.ShadowLesson(*t, id, tvspoof.StatFor(stats, tvspoof.ShadowKey(*t)))); err != nil {
			fail(err)
		}
		t.LessonID = id
		written = append(written, id)
	}
	if err := tvspoof.SaveShadowTrades(tvspoof.DefaultShadowPath, trades); err != nil {
		fail(err)
	}

	emit(jsonOut, map[string]interface{}{"resolved": resolved, "lessons_written": written, "stats": stats}, func() {
		fmt.Printf("SHADOW CHECK %s UTC — %d resolved, %d lessons written %v\n", now.Format("2006-01-02 15:04"), len(resolved), len(written), written)
		printTrades(resolved)
		var open []tvspoof.ShadowTrade
		for _, t := range trades {
			if t.Open() {
				open = append(open, t)
			}
		}
		fmt.Printf("\nOPEN (%d):\n", len(open))
		printTrades(open)
		fmt.Println()
		printStats(stats)
	})
}

func printTrades(ts []tvspoof.ShadowTrade) {
	for _, t := range ts {
		line := fmt.Sprintf("  %s %-12s %-12s %-4s %-9s @ %.5g %s", t.ID, t.Status, t.Setup, t.Side, t.Priority, t.Level, t.LevelName)
		if t.Triggered() {
			line += fmt.Sprintf(" | entry %.5g SL %.5g (%.0f pips) TP1 %.5g (%.0f) TP2 %.5g (%.0f) | %+.1f pips $%+.2f@0.01", t.Entry, t.SL, t.SLPips, t.TP1, t.TP1Pips, t.TP2, t.TP2Pips, t.Pips, t.USD)
		}
		if t.Reason != "" {
			line += " | " + t.Reason
		}
		fmt.Println(line)
	}
}

func printStats(stats []tvspoof.ShadowSetupStat) {
	fmt.Printf("%-34s %6s %6s %5s %5s %5s %6s %6s %10s %11s\n", "setup", "logged", "trig", "win", "loss", "inval", "expir", "win%", "avg pips", "$ @0.01lot")
	for _, s := range stats {
		fmt.Printf("%-34s %6d %6d %5d %5d %5d %6d %5.0f%% %+10.1f %+11.2f\n", s.Key, s.Logged, s.Triggered, s.Wins, s.Losses, s.Invalidated, s.Expired, s.WinRate*100, s.AvgPips, s.TotalUSD)
	}
}

func emit(jsonOut bool, v interface{}, text func()) {
	if !jsonOut {
		text()
		return
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

// backfill grades the advisor's historical calls; results go to
// shadow_backfill.json (never into lessons.json, so history does not flood it).
func backfill(args []string, jsonOut bool) {
	var symbols []string
	cacheDir, out := "", "shadow_backfill.json"
	for _, a := range args {
		k, v, isFlag := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if !strings.HasPrefix(a, "-") || !isFlag {
			symbols = append(symbols, a)
			continue
		}
		f, _ := strconv.ParseFloat(v, 64)
		switch k {
		case "cache-dir":
			cacheDir = v
		case "out":
			out = v
		case "sweep-sl":
			tvspoof.ShadowParams.SweepSLBufATR = f
		case "break-sl":
			tvspoof.ShadowParams.BreakSLBufATR = f
		case "min-risk":
			tvspoof.ShadowParams.MinRiskATR = f
		case "tp1":
			tvspoof.ShadowParams.TP1MinR = f
		case "tp2":
			tvspoof.ShadowParams.TP2AddR = f
		case "level-targets":
			tvspoof.ShadowParams.UseLevelTargets = v != "false"
		}
	}
	if len(symbols) == 0 {
		symbols = []string{"xauusd"}
	}
	var all []tvspoof.ShadowTrade
	for _, sym := range symbols {
		h, err := loadScenarioHistory(sym, cacheDir)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		j := tvspoof.BackfillShadow(h, 12, 12*time.Hour)
		all = append(all, j...)
		if !jsonOut {
			fmt.Printf("%s: %d scenarios logged (%s → %s)\n", h.Symbol, len(j),
				time.Unix(h.M5[0].Time, 0).UTC().Format("2006-01-02"), time.Unix(h.M5[len(h.M5)-1].Time, 0).UTC().Format("2006-01-02"))
		}
	}
	if err := tvspoof.SaveShadowTrades(out, all); err != nil {
		fail(err)
	}
	stats := tvspoof.ShadowStats(all)
	emit(jsonOut, stats, func() { fmt.Println(); printStats(stats) })
}

func loadScenarioHistory(sym, cacheDir string) (*tvspoof.ScenarioHistory, error) {
	path := ""
	if cacheDir != "" {
		path = filepath.Join(cacheDir, "scen_"+strings.ToLower(sym)+".json")
		if raw, err := os.ReadFile(path); err == nil {
			var h tvspoof.ScenarioHistory
			if json.Unmarshal(raw, &h) == nil && len(h.M5) > 0 {
				return &h, nil
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	h, err := tvspoof.NewClient().FetchScenarioHistory(ctx, sym, 5000)
	if err != nil {
		return nil, err
	}
	if path != "" {
		if raw, err := json.Marshal(h); err == nil {
			_ = os.WriteFile(path, raw, 0644)
		}
	}
	return h, nil
}
