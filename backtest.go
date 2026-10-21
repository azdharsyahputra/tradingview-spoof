package tvspoof

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// BacktestData is the full history replayed by RunBacktest.
type BacktestData struct {
	Symbol                                string
	M15, H1, H4, D                        []Bar
	DXY, US10Y, RealYield, SPX, Silver    []Bar
}

// BacktestConfig controls the walk-forward replay.
type BacktestConfig struct {
	Step        int     // M15 bars between decisions (default 4 = hourly)
	MaxHoldBars int     // M15 bars before a trade is closed at market (default 96 = 24h)
	Horizon     int     // M15 bars for the factor forward-return audit (default 16 = 4h)
	Spread      float64 // round-trip cost in price units (default: contract typical spread)
}

// BacktestTrade is one hypothetical desk decision and its simulated outcome.
type BacktestTrade struct {
	Time       int64            `json:"time"`
	Score      int              `json:"score"`
	Direction  string           `json:"direction"` // engine lean: LONG / SHORT
	Tradeable  bool             `json:"tradeable"` // outside neutral band
	Entry      float64          `json:"entry"`
	SL         float64          `json:"sl"`
	TP1        float64          `json:"tp1"`
	TP2        float64          `json:"tp2"`
	TP1Hit     bool             `json:"tp1_hit"`
	RunnerHit  bool             `json:"runner_hit"`
	StoppedOut bool             `json:"stopped_out"`
	R          float64          `json:"r"`           // realised R after costs
	Exited     bool             `json:"-"`           // stop or final target hit (not time-out)
	ExitTime   int64            `json:"exit_time,omitempty"`
	FwdATR     float64          `json:"fwd_atr"`     // close[t+Horizon]-close[t] in ATR units
	Components []ScoreComponent `json:"components"`
}

// FetchBacktestData downloads ~m15Bars of M15 history plus matching HTF and
// intermarket series.
func (c *Client) FetchBacktestData(ctx context.Context, symbol string, m15Bars int) (*BacktestData, error) {
	sym := NormalizeDeskSymbol(symbol)
	h1 := m15Bars/4 + 200
	d := &BacktestData{Symbol: sym}
	type job struct {
		dst   *[]Bar
		sym   string
		tf    string
		n     int
		fatal bool
	}
	jobs := []job{
		{&d.M15, sym, "15", m15Bars, true},
		{&d.H1, sym, "60", h1, false},
		{&d.H4, sym, "240", h1/4 + 150, false},
		{&d.D, sym, "D", h1/24 + 150, false},
		{&d.DXY, "TVC:DXY", "60", h1, false},
		{&d.US10Y, "TVC:US10Y", "60", h1, false},
		{&d.SPX, "TVC:SPX", "60", h1, false},
		{&d.Silver, "OANDA:XAGUSD", "60", h1, false},
	}
	var wg sync.WaitGroup
	errs := make([]error, len(jobs))
	for i, j := range jobs {
		wg.Add(1)
		go func(i int, j job) {
			defer wg.Done()
			*j.dst, errs[i] = c.GetHistory(ctx, j.sym, j.tf, j.n)
		}(i, j)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.RealYield, _ = fetchTenYearRealYield(ctx, h1/24+60)
	}()
	wg.Wait()
	for i, j := range jobs {
		if errs[i] != nil && j.fatal {
			return nil, fmt.Errorf("fetch %s %s: %w", j.sym, j.tf, errs[i])
		}
	}
	return d, nil
}

// RunBacktest replays BuildDeskBriefing over history without lookahead and
// simulates the playbook with the TP1-partial / BEP-runner exit model.
func RunBacktest(data *BacktestData, cfg BacktestConfig) []BacktestTrade {
	if cfg.Step <= 0 {
		cfg.Step = 4
	}
	if cfg.MaxHoldBars <= 0 {
		cfg.MaxHoldBars = 96
	}
	if cfg.Horizon <= 0 {
		cfg.Horizon = 16
	}
	m15 := data.M15
	if len(m15) < 300 {
		return nil
	}
	if cfg.Spread <= 0 {
		cfg.Spread = ContractSpecFor(data.Symbol, m15[len(m15)-1].Close).TypicalSpread
	}

	const window15 = 150 // AnalyzeDesk fetches 150 M15 bars
	var trades []BacktestTrade
	for i := window15; i < len(m15)-cfg.Horizon; i += cfg.Step {
		now := time.Unix(m15[i].Time, 0).Add(15 * time.Minute)
		ltf := m15[:i+1]
		in := DeskInputs{
			Symbol:    data.Symbol,
			Balance:   10000,
			RiskPerc:  1,
			Now:       now,
			Bars15M:   ltf[len(ltf)-window15:],
			Bars1H:    tail(asOf(data.H1, time.Hour, ltf, now), 120),
			Bars4H:    tail(asOf(data.H4, 4*time.Hour, ltf, now), 100),
			BarsD:     tail(asOf(data.D, 24*time.Hour, ltf, now), 100),
			DXY:       tail(closedAsOf(data.DXY, time.Hour, now), 100),
			US10Y:     tail(closedAsOf(data.US10Y, time.Hour, now), 100),
			SPX:       tail(closedAsOf(data.SPX, time.Hour, now), 100),
			Silver:    tail(closedAsOf(data.Silver, time.Hour, now), 100),
			RealYield: tail(closedAsOf(data.RealYield, 48*time.Hour, now), 40), // FRED publishes T+1
		}
		b := BuildDeskBriefing(in)
		pb := b.Playbook
		long := pb.Direction == "LONG" || (pb.Direction == "WAIT" && b.ConvictionScore >= 50)
		dir := "SHORT"
		if long {
			dir = "LONG"
		}
		t := BacktestTrade{
			Time: m15[i].Time, Score: b.ConvictionScore, Direction: dir, Tradeable: pb.Direction != "WAIT",
			Entry: pb.EntryPrice, SL: pb.InvalidationPrice, TP1: pb.Target1Price, TP2: pb.Target2Price,
			Components: b.ScoreBreakdown,
		}
		atr := calculateATR(in.Bars15M, 14)
		if atr > 0 {
			t.FwdATR = (m15[i+cfg.Horizon].Close - m15[i].Close) / atr
		}
		simulateManagedTrade(&t, long, m15[i+1:], cfg.MaxHoldBars, cfg.Spread)
		trades = append(trades, t)
	}
	return trades
}

// simulateManagedTrade walks forward bar by bar. Prices are bid; the spread
// is charged once on entry. When a bar touches both stop and target the stop
// is assumed first (conservative).
func simulateManagedTrade(t *BacktestTrade, long bool, future []Bar, maxHold int, spread float64) {
	dir := 1.0
	if !long {
		dir = -1
	}
	risk := math.Abs(t.Entry - t.SL)
	if risk <= 0 {
		return
	}
	cost := spread / risk
	stop := t.SL
	banked := 0.0
	open := 1.0
	r := func(px float64) float64 { return (px - t.Entry) * dir / risk }
	adverse := func(b Bar) float64 {
		if long {
			return b.Low
		}
		return b.High
	}
	favour := func(b Bar) float64 {
		if long {
			return b.High
		}
		return b.Low
	}
	hit := func(px, level float64, toward bool) bool {
		if toward == long {
			return px >= level
		}
		return px <= level
	}
	n := maxHold
	if n > len(future) {
		n = len(future)
	}
	for k := 0; k < n; k++ {
		b := future[k]
		if hit(adverse(b), stop, false) {
			if !t.TP1Hit {
				t.StoppedOut = true
			}
			t.R = banked + open*r(stop) - cost
			t.Exited, t.ExitTime = true, b.Time
			return
		}
		if !t.TP1Hit && hit(favour(b), t.TP1, true) {
			t.TP1Hit = true
			banked += 0.5 * r(t.TP1)
			open = 0.5
			stop = t.Entry // Lesson-008: trail to BEP
			continue
		}
		if t.TP1Hit && hit(favour(b), t.TP2, true) {
			t.RunnerHit = true
			t.R = banked + open*r(t.TP2) - cost
			t.Exited, t.ExitTime = true, b.Time
			return
		}
	}
	if n > 0 {
		t.R = banked + open*r(future[n-1].Close) - cost
		t.ExitTime = future[n-1].Time
	}
}

// asOf returns the HTF bars known at `now`: closed HTF candles plus the
// forming candle rebuilt from LTF bars (never the final OHLC, which would leak
// the future).
func asOf(htf []Bar, dur time.Duration, ltf []Bar, now time.Time) []Bar {
	cut := sort.Search(len(htf), func(i int) bool { return htf[i].Time > now.Unix() })
	known := htf[:cut]
	if len(known) == 0 {
		return nil
	}
	last := known[len(known)-1]
	if !time.Unix(last.Time, 0).Add(dur).After(now) {
		return known
	}
	partial := Bar{Time: last.Time}
	started := false
	for j := len(ltf) - 1; j >= 0 && ltf[j].Time >= last.Time; j-- {
		b := ltf[j]
		if !started {
			partial.Close, partial.High, partial.Low, started = b.Close, b.High, b.Low, true
		}
		partial.Open = b.Open
		partial.High = math.Max(partial.High, b.High)
		partial.Low = math.Min(partial.Low, b.Low)
		partial.Volume += b.Volume
	}
	out := append([]Bar(nil), known[:len(known)-1]...)
	if started {
		out = append(out, partial)
	}
	return out
}

func closedAsOf(bars []Bar, dur time.Duration, now time.Time) []Bar {
	cut := sort.Search(len(bars), func(i int) bool {
		return time.Unix(bars[i].Time, 0).Add(dur).After(now)
	})
	return bars[:cut]
}

func tail(bars []Bar, n int) []Bar {
	if len(bars) > n {
		return bars[len(bars)-n:]
	}
	return bars
}

// BacktestStats aggregates trades.
type BacktestStats struct {
	Trades      int     `json:"trades"`
	TP1Rate     float64 `json:"tp1_rate"`
	RunnerRate  float64 `json:"runner_rate"`
	StopRate    float64 `json:"stop_rate"`
	ExpectancyR float64 `json:"expectancy_r"`
	TotalR      float64 `json:"total_r"`
	ProfitFact  float64 `json:"profit_factor"`
	MaxDDR      float64 `json:"max_drawdown_r"`
	DirHitRate  float64 `json:"directional_hit_rate"` // sign(fwd return) == lean
}

// FactorStat is the directional hit rate of one score component.
type FactorStat struct {
	Name    string  `json:"name"`
	Active  int     `json:"active"`   // decisions with non-zero signal
	HitRate float64 `json:"hit_rate"` // sign(signal) == sign(forward return)
	MeanFwd float64 `json:"mean_signed_fwd_atr"`
	TStat   float64 `json:"t_stat"`
}

// Stats summarises trades (in time order; drawdown is sequential).
func Stats(trades []BacktestTrade) BacktestStats {
	var s BacktestStats
	var win, loss, peak, eq float64
	var runners, tp1s, dirHits int
	for _, t := range trades {
		s.Trades++
		s.TotalR += t.R
		if t.R > 0 {
			win += t.R
		} else {
			loss -= t.R
		}
		eq += t.R
		peak = math.Max(peak, eq)
		s.MaxDDR = math.Max(s.MaxDDR, peak-eq)
		if t.TP1Hit {
			tp1s++
			if t.RunnerHit {
				runners++
			}
		}
		if t.StoppedOut {
			s.StopRate++
		}
		if (t.FwdATR > 0) == (t.Direction == "LONG") && t.FwdATR != 0 {
			dirHits++
		}
	}
	if s.Trades == 0 {
		return s
	}
	n := float64(s.Trades)
	s.TP1Rate = float64(tp1s) / n
	if tp1s > 0 {
		s.RunnerRate = float64(runners) / float64(tp1s)
	}
	s.StopRate /= n
	s.ExpectancyR = s.TotalR / n
	s.DirHitRate = float64(dirHits) / n
	if loss > 0 {
		s.ProfitFact = win / loss
	}
	return s
}

// FactorStats audits each score component against the forward return.
func FactorStats(trades []BacktestTrade) []FactorStat {
	type acc struct {
		n, hits    int
		sum, sumSq float64
	}
	m := map[string]*acc{}
	var order []string
	for _, t := range trades {
		for _, c := range t.Components {
			a, ok := m[c.Name]
			if !ok {
				a = &acc{}
				m[c.Name] = a
				order = append(order, c.Name)
			}
			if c.Signal == 0 || t.FwdATR == 0 {
				continue
			}
			x := math.Copysign(1, c.Signal) * t.FwdATR
			a.n++
			if x > 0 {
				a.hits++
			}
			a.sum += x
			a.sumSq += x * x
		}
	}
	var out []FactorStat
	for _, name := range order {
		a := m[name]
		fs := FactorStat{Name: name, Active: a.n}
		if a.n > 1 {
			n := float64(a.n)
			mean := a.sum / n
			sd := math.Sqrt(math.Max(0, (a.sumSq-n*mean*mean)/(n-1)))
			fs.HitRate = float64(a.hits) / n
			fs.MeanFwd = mean
			if sd > 0 {
				fs.TStat = mean / (sd / math.Sqrt(n))
			}
		}
		out = append(out, fs)
	}
	return out
}

// calibrationEdges are the |score-50| bucket boundaries.
var calibrationEdges = [][2]int{{0, 7}, {8, 14}, {15, 24}, {25, 50}}

// BuildSymbolCalibration buckets trades by direction and conviction edge.
func BuildSymbolCalibration(symbol string, trades []BacktestTrade) SymbolCalibration {
	sc := SymbolCalibration{Symbol: symbol}
	if len(trades) > 0 {
		sc.From = time.Unix(trades[0].Time, 0).UTC().Format("2006-01-02")
		sc.To = time.Unix(trades[len(trades)-1].Time, 0).UTC().Format("2006-01-02")
	}
	for _, dir := range []string{"LONG", "SHORT"} {
		for _, e := range calibrationEdges {
			var bucket []BacktestTrade
			for _, t := range trades {
				edge := int(math.Abs(float64(t.Score - 50)))
				if t.Direction == dir && edge >= e[0] && edge <= e[1] {
					bucket = append(bucket, t)
				}
			}
			st := Stats(bucket)
			sc.Buckets = append(sc.Buckets, CalibrationBucket{
				Direction: dir, MinEdge: e[0], MaxEdge: e[1], Samples: st.Trades,
				TP1Rate: st.TP1Rate, RunnerRate: st.RunnerRate, ExpectancyR: st.ExpectancyR,
			})
		}
	}
	return sc
}
