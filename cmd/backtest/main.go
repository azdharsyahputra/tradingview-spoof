// Command backtest replays the desk engine walk-forward over TradingView
// history, audits each score factor, and writes calibration.json so the
// desk's EstimatedWinRate is an out-of-sample frequency instead of a guess.
//
//	go run ./cmd/backtest xauusd                 # 10000 M15 bars, report only
//	go run ./cmd/backtest -write xauusd          # also update calibration.json (OOS segment)
//	go run ./cmd/backtest -cache /tmp/x.json xauusd   # reuse downloaded history
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tvspoof "github.com/azdharsyahputra/tradingview-spoof"
)

func main() {
	bars := flag.Int("bars", 10000, "M15 bars of history")
	step := flag.Int("step", 4, "M15 bars between decisions")
	hold := flag.Int("hold", 96, "max M15 bars per trade")
	horizon := flag.Int("horizon", 16, "M15 bars for factor forward return")
	oos := flag.Float64("oos", 0.4, "fraction of decisions (most recent) held out of sample")
	cache := flag.String("cache", "", "JSON file to load/save downloaded history")
	write := flag.Bool("write", false, "write calibration buckets (OOS segment) to calibration.json")
	calibAll := flag.Bool("calib-all", false, "calibrate on all decisions instead of OOS only")
	jsonOut := flag.Bool("json", false, "print JSON report")
	weights := flag.String("weights", "", "override score weights, e.g. asia_sweep=0,macro=20")
	band := flag.Int("band", -1, "override neutral band |score-50|")
	cacheDir := flag.String("cache-dir", "", "with several symbols: directory of <symbol>.json caches")
	flag.Parse()
	if err := applyWeights(*weights); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	if *band >= 0 {
		tvspoof.SetDeskNeutralBand(*band)
	}
	symbol := "xauusd"
	if flag.NArg() > 1 {
		runPooled(flag.Args(), *bars, *cacheDir, tvspoof.BacktestConfig{Step: *step, MaxHoldBars: *hold, Horizon: *horizon}, *oos)
		return
	}
	if flag.NArg() > 0 {
		symbol = flag.Arg(0)
	}

	data, err := loadData(symbol, *bars, *cache)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	trades := tvspoof.RunBacktest(data, tvspoof.BacktestConfig{Step: *step, MaxHoldBars: *hold, Horizon: *horizon})
	if len(trades) == 0 {
		fmt.Fprintln(os.Stderr, "error: not enough history")
		os.Exit(1)
	}
	split := int(float64(len(trades)) * (1 - *oos))
	is, os_ := trades[:split], trades[split:]

	report := map[string]interface{}{
		"symbol":         data.Symbol,
		"from":           ts(trades[0].Time),
		"to":             ts(trades[len(trades)-1].Time),
		"oos_from":       ts(os_[0].Time),
		"all_tradeable":  tvspoof.Stats(tradeable(trades)),
		"is_tradeable":   tvspoof.Stats(tradeable(is)),
		"oos_tradeable":  tvspoof.Stats(tradeable(os_)),
		"oos_all_leans":  tvspoof.Stats(os_),
		"factors_is":     tvspoof.FactorStats(is),
		"factors_oos":    tvspoof.FactorStats(os_),
		"calibration_oos": tvspoof.BuildSymbolCalibration(data.Symbol, os_),
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
	} else {
		printReport(data.Symbol, trades, is, os_)
	}

	if *write {
		src := os_
		if *calibAll {
			src = trades
		}
		sc := tvspoof.BuildSymbolCalibration(data.Symbol, src)
		sc.Settings = map[string]float64{"step": float64(*step), "hold": float64(*hold), "bars": float64(*bars)}
		c := tvspoof.LoadCalibration(tvspoof.DefaultCalibrationPath)
		if c == nil {
			c = &tvspoof.Calibration{}
		}
		c.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
		c.Upsert(sc)
		if err := tvspoof.SaveCalibration(tvspoof.DefaultCalibrationPath, c); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Printf("\ncalibration.json updated for %s (%d decisions, %s → %s)\n", data.Symbol, len(src), sc.From, sc.To)
	}
}

func loadData(symbol string, bars int, cache string) (*tvspoof.BacktestData, error) {
	if cache != "" {
		if raw, err := os.ReadFile(cache); err == nil {
			var d tvspoof.BacktestData
			if err := json.Unmarshal(raw, &d); err == nil && len(d.M15) > 0 {
				return &d, nil
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	d, err := tvspoof.NewClient().FetchBacktestData(ctx, symbol, bars)
	if err != nil {
		return nil, err
	}
	if cache != "" {
		if raw, err := json.Marshal(d); err == nil {
			_ = os.WriteFile(cache, raw, 0644)
		}
	}
	return d, nil
}

func tradeable(trades []tvspoof.BacktestTrade) []tvspoof.BacktestTrade {
	var out []tvspoof.BacktestTrade
	for _, t := range trades {
		if t.Tradeable {
			out = append(out, t)
		}
	}
	return out
}

func ts(unix int64) string { return time.Unix(unix, 0).UTC().Format("2006-01-02 15:04") }

func printReport(symbol string, all, is, oos []tvspoof.BacktestTrade) {
	fmt.Printf("WALK-FORWARD BACKTEST  %s  %s → %s  (OOS from %s)\n", symbol, ts(all[0].Time), ts(all[len(all)-1].Time), ts(oos[0].Time))
	fmt.Println("exit model: 50% at TP1, SL→BEP, runner to TP2; spread charged; SL assumed first on ambiguous bars")
	fmt.Printf("\n%-22s %6s %7s %7s %7s %8s %8s %6s %7s %7s\n", "segment", "n", "TP1%", "run%", "stop%", "E[R]", "sumR", "PF", "maxDD", "dir%")
	row := func(name string, tr []tvspoof.BacktestTrade) {
		s := tvspoof.Stats(tr)
		fmt.Printf("%-22s %6d %6.1f%% %6.1f%% %6.1f%% %+8.3f %+8.1f %6.2f %7.1f %6.1f%%\n", name, s.Trades,
			s.TP1Rate*100, s.RunnerRate*100, s.StopRate*100, s.ExpectancyR, s.TotalR, s.ProfitFact, s.MaxDDR, s.DirHitRate*100)
	}
	row("in-sample tradeable", tradeable(is))
	row("OOS tradeable", tradeable(oos))
	row("OOS all leans", oos)

	fmt.Printf("\nOOS calibration buckets (|score-50|):\n%-6s %-7s %6s %7s %7s %8s\n", "dir", "edge", "n", "TP1%", "run%", "E[R]")
	for _, b := range tvspoof.BuildSymbolCalibration(symbol, oos).Buckets {
		fmt.Printf("%-6s %2d-%-4d %6d %6.1f%% %6.1f%% %+8.3f\n", b.Direction, b.MinEdge, b.MaxEdge, b.Samples, b.TP1Rate*100, b.RunnerRate*100, b.ExpectancyR)
	}

	fmt.Printf("\nFactor audit (sign(signal) vs %s forward return):\n%-14s %22s   %22s\n", "horizon", "factor", "IN-SAMPLE n/hit/t", "OOS n/hit/t")
	isF, oosF := tvspoof.FactorStats(is), tvspoof.FactorStats(oos)
	for i, f := range isF {
		o := tvspoof.FactorStat{}
		if i < len(oosF) {
			o = oosF[i]
		}
		fmt.Printf("%-14s %6d %6.1f%% %+6.2f   %6d %6.1f%% %+6.2f\n", f.Name, f.Active, f.HitRate*100, f.TStat, o.Active, o.HitRate*100, o.TStat)
	}
}

func applyWeights(spec string) error {
	if spec == "" {
		return nil
	}
	w := map[string]float64{}
	for _, kv := range strings.Split(spec, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(kv), "=")
		if !ok {
			return fmt.Errorf("bad weight %q", kv)
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("bad weight %q: %v", kv, err)
		}
		w[k] = f
	}
	tvspoof.SetDeskScoreWeights(w)
	return nil
}

// runPooled backtests several symbols and pools results so factor verdicts
// do not hinge on one asset's regime.
func runPooled(symbols []string, bars int, cacheDir string, cfg tvspoof.BacktestConfig, oos float64) {
	var isAll, oosAll []tvspoof.BacktestTrade
	fmt.Printf("%-18s %6s %8s %8s %8s %8s\n", "symbol", "n", "IS E[R]", "OOS E[R]", "IS dir%", "OOS dir%")
	for _, sym := range symbols {
		cache := ""
		if cacheDir != "" {
			cache = filepath.Join(cacheDir, sym+".json")
		}
		data, err := loadData(sym, bars, cache)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", sym, err)
			continue
		}
		trades := tvspoof.RunBacktest(data, cfg)
		split := int(float64(len(trades)) * (1 - oos))
		is, os_ := trades[:split], trades[split:]
		isAll = append(isAll, is...)
		oosAll = append(oosAll, os_...)
		a, b := tvspoof.Stats(tradeable(is)), tvspoof.Stats(tradeable(os_))
		fmt.Printf("%-18s %6d %+8.3f %+8.3f %7.1f%% %7.1f%%\n", data.Symbol, len(trades), a.ExpectancyR, b.ExpectancyR, a.DirHitRate*100, b.DirHitRate*100)
	}
	sort.SliceStable(isAll, func(i, j int) bool { return isAll[i].Time < isAll[j].Time })
	sort.SliceStable(oosAll, func(i, j int) bool { return oosAll[i].Time < oosAll[j].Time })
	a, b := tvspoof.Stats(tradeable(isAll)), tvspoof.Stats(tradeable(oosAll))
	fmt.Printf("%-18s %6d %+8.3f %+8.3f %7.1f%% %7.1f%%   (tradeable IS n=%d, OOS n=%d, OOS PF %.2f, TP1 %.1f%%)\n",
		"POOLED", len(isAll)+len(oosAll), a.ExpectancyR, b.ExpectancyR, a.DirHitRate*100, b.DirHitRate*100, a.Trades, b.Trades, b.ProfitFact, b.TP1Rate*100)
	fmt.Printf("\nPooled factor audit:\n%-14s %22s   %22s\n", "factor", "IN-SAMPLE n/hit/t", "OOS n/hit/t")
	isF, oosF := tvspoof.FactorStats(isAll), tvspoof.FactorStats(oosAll)
	for i, f := range isF {
		o := tvspoof.FactorStat{}
		if i < len(oosF) {
			o = oosF[i]
		}
		fmt.Printf("%-14s %6d %6.1f%% %+6.2f   %6d %6.1f%% %+6.2f\n", f.Name, f.Active, f.HitRate*100, f.TStat, o.Active, o.HitRate*100, o.TStat)
	}
}
