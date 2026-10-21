package tvspoof

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ScenarioHistory is the candle history replayed by BackfillShadow.
type ScenarioHistory struct {
	Symbol             string
	M5, M15, H1, H4, D []Bar
	DXY                []Bar // M15, optional
}

// FetchScenarioHistory downloads ~m5Bars of M5 plus covering HTF history.
func (c *Client) FetchScenarioHistory(ctx context.Context, symbol string, m5Bars int) (*ScenarioHistory, error) {
	h := &ScenarioHistory{Symbol: NormalizeDeskSymbol(symbol)}
	jobs := []struct {
		dst *[]Bar
		tf  string
		n   int
	}{
		{&h.M5, "5", m5Bars}, {&h.M15, "15", m5Bars/3 + 200}, {&h.H1, "60", m5Bars/12 + 150},
		{&h.H4, "240", m5Bars/48 + 120}, {&h.D, "D", m5Bars/288 + 40},
	}
	if usdExposure(h.Symbol) != 0 {
		jobs = append(jobs, struct {
			dst *[]Bar
			tf  string
			n   int
		}{&h.DXY, "15", m5Bars/3 + 200})
	}
	var wg sync.WaitGroup
	errs := make([]error, len(jobs))
	for i, j := range jobs {
		wg.Add(1)
		go func(i int, dst *[]Bar, tf string, n int) {
			defer wg.Done()
			sym := h.Symbol
			if dst == &h.DXY {
				sym = "TVC:DXY"
			}
			*dst, errs[i] = c.GetHistory(ctx, sym, tf, n)
		}(i, j.dst, j.tf, j.n)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil && jobs[i].dst != &h.DXY {
			return nil, fmt.Errorf("fetch %s %s: %w", h.Symbol, jobs[i].tf, err)
		}
	}
	return h, nil
}

// BackfillShadow replays the advisor every `step` M5 bars without lookahead,
// logs its non-FAR scenarios as shadow trades exactly like the live flow, and
// grades them on the following candles.
func BackfillShadow(h *ScenarioHistory, step int, expiry time.Duration) []ShadowTrade {
	if step <= 0 {
		step = 12
	}
	m5 := h.M5
	spread := 0.0
	if len(m5) > 0 {
		spread = ContractSpecFor(h.Symbol, m5[len(m5)-1].Close).TypicalSpread
	}
	var journal []ShadowTrade
	for i := ScenarioWarmup; i < len(m5); i += step {
		now := time.Unix(m5[i].Time, 0).Add(5 * time.Minute)
		known := m5[:i+1]
		for k := range journal {
			EvaluateShadow(&journal[k], known, now, spread)
		}
		rep := h.ReportAt(i)
		journal = append(journal, NewShadowTrades(rep, journal, expiry, false)...)
	}
	end := time.Unix(m5[len(m5)-1].Time, 0).Add(5 * time.Minute)
	for k := range journal {
		EvaluateShadow(&journal[k], m5, end, spread)
	}
	return journal
}

// ScenarioWarmup is the number of M5 candles the advisor needs.
const ScenarioWarmup = 300

// ReportAt builds the advisor report as it would have looked right after M5
// candle i closed (no lookahead: HTF candles still forming are rebuilt from M5).
func (h *ScenarioHistory) ReportAt(i int) *ScenarioReport {
	now := h.CloseTime(i)
	known := h.M5[:i+1]
	return BuildScenarioReport(h.Symbol,
		tail(known, ScenarioWarmup),
		tail(asOf(h.M15, 15*time.Minute, known, now), 150),
		tail(asOf(h.H1, time.Hour, known, now), 120),
		tail(asOf(h.H4, 4*time.Hour, known, now), 100),
		tail(asOf(h.D, 24*time.Hour, known, now), 60),
		tail(closedAsOf(h.DXY, 15*time.Minute, now), 40),
		now)
}

// CloseTime is the close time of M5 candle i.
func (h *ScenarioHistory) CloseTime(i int) time.Time {
	return time.Unix(h.M5[i].Time, 0).UTC().Add(5 * time.Minute)
}

// BarsAsOf returns the last n candles of tf ("M5" or "M15") known after M5 candle i closed.
func (h *ScenarioHistory) BarsAsOf(tf string, i, n int) []Bar {
	known := h.M5[:i+1]
	if tf == "M15" {
		return tail(asOf(h.M15, 15*time.Minute, known, h.CloseTime(i)), n)
	}
	return tail(known, n)
}
