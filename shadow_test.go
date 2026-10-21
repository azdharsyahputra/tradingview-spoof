package tvspoof

import (
	"testing"
	"time"
)

func m5(t0 int64, ohlc ...[4]float64) []Bar {
	out := make([]Bar, len(ohlc))
	for i, v := range ohlc {
		out[i] = Bar{Time: t0 + int64(i)*300, Open: v[0], High: v[1], Low: v[2], Close: v[3]}
	}
	return out
}

func TestShadowSweepRejectBothSides(t *testing.T) {
	// SELL at resistance 100: sweep to 100.4, bearish candle, then gap down (FVG), then dump to TP2.
	bars := m5(0,
		[4]float64{99, 100.4, 98.9, 99.5},  // sweep, close back below
		[4]float64{99.5, 99.6, 98.0, 98.2}, // bearish displacement
		[4]float64{98.2, 98.5, 97.5, 97.8}, // high 98.5 < low two bars back 98.9 -> bearish FVG, sell limit @98.5
		[4]float64{97.8, 98.6, 97.6, 98.0}, // retrace fills the limit
		[4]float64{98.0, 98.1, 95.0, 95.2},
		[4]float64{95.2, 95.3, 90.0, 90.5},
	)
	sell := ShadowTrade{Setup: "SWEEP_REJECT", Side: "SELL", Level: 100, ATR: 1, ExpiresAt: 1e9, Status: ShadowPending, PlannedTP1: 95, PlannedTP2: 91}
	EvaluateShadow(&sell, bars, time.Unix(1e6, 0), 0)
	if sell.Entry != 98.5 || sell.SL <= 100.4 || sell.Status != ShadowWin || !sell.RunnerHit {
		t.Fatalf("sell sweep-reject: %+v", sell)
	}

	// Mirror for BUY at support -100 (negate every price).
	neg := make([]Bar, len(bars))
	for i, b := range bars {
		neg[i] = Bar{Time: b.Time, Open: -b.Open, High: -b.Low, Low: -b.High, Close: -b.Close}
	}
	buy := ShadowTrade{Setup: "SWEEP_REJECT", Side: "BUY", Level: -100, ATR: 1, ExpiresAt: 1e9, Status: ShadowPending, PlannedTP1: -95, PlannedTP2: -91}
	EvaluateShadow(&buy, neg, time.Unix(1e6, 0), 0)
	if buy.Entry != -98.5 || buy.SL >= -100.4 || buy.Status != ShadowWin || !buy.RunnerHit {
		t.Fatalf("buy sweep-reject (mirror): %+v", buy)
	}
}

func TestShadowBreakRetestAndFakeout(t *testing.T) {
	ok := m5(0,
		[4]float64{99, 100.5, 98.8, 100.4},     // close > 100.1: break
		[4]float64{100.4, 100.6, 100.1, 100.5}, // retest low <= 100.25, close above: trigger
		[4]float64{100.5, 104, 100.4, 103.8},
	)
	tr := ShadowTrade{Setup: "BREAK_RETEST", Side: "BUY", Level: 100, ATR: 1, ExpiresAt: 1e9, Status: ShadowPending}
	EvaluateShadow(&tr, ok, time.Unix(1e6, 0), 0)
	if !tr.Triggered() || tr.Entry != 100.5 || !tr.TP1Hit {
		t.Fatalf("break-retest: %+v", tr)
	}

	fake := m5(0,
		[4]float64{99, 100.5, 98.8, 100.4},
		[4]float64{100.4, 100.5, 99.0, 99.5}, // close < 99.7 before retest
	)
	tr = ShadowTrade{Setup: "BREAK_RETEST", Side: "BUY", Level: 100, ATR: 1, ExpiresAt: 1e9, Status: ShadowPending}
	EvaluateShadow(&tr, fake, time.Unix(1e6, 0), 0)
	if tr.Status != ShadowInvalidated {
		t.Fatalf("fake breakout should invalidate: %+v", tr)
	}
}

func TestShadowIgnoresCandlesBeforeCreationAndForming(t *testing.T) {
	bars := m5(0,
		[4]float64{99, 100.4, 98.9, 99.5},
		[4]float64{99.5, 99.6, 98.0, 98.2},
		[4]float64{98.2, 98.5, 97.5, 97.8},
	)
	tr := ShadowTrade{Setup: "SWEEP_REJECT", Side: "SELL", Level: 100, ATR: 1, CreatedAt: 900, ExpiresAt: 1e9, Status: ShadowPending}
	EvaluateShadow(&tr, bars, time.Unix(1e6, 0), 0)
	if tr.Triggered() || tr.Status != ShadowPending {
		t.Fatal("must not trigger on candles before the scenario existed")
	}
	tr = ShadowTrade{Setup: "SWEEP_REJECT", Side: "SELL", Level: 100, ATR: 1, ExpiresAt: 1e9, Status: ShadowPending}
	EvaluateShadow(&tr, bars, time.Unix(700, 0), 0) // third candle still forming
	if tr.Triggered() || tr.Status != ShadowPending {
		t.Fatalf("must not act on a forming candle: %+v", tr)
	}
	// FVG confirmed but limit not filled yet: stays pending, not expired.
	EvaluateShadow(&tr, bars, time.Unix(1e6, 0), 0)
	if tr.Status != ShadowPending {
		t.Fatalf("unfilled limit with few candles must stay pending: %+v", tr)
	}
}

func TestKillzoneAndLevelTouches(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 10, 21, h, m, 0, 0, time.UTC) }
	if got := killzone(at(6, 35)); got != "London open 25m lagi (rawan spike/sweep)" {
		t.Fatalf("killzone = %q", got)
	}
	if got := killzone(at(14, 0)); got != "NY open 30m lalu (killzone aktif)" {
		t.Fatalf("killzone = %q", got)
	}
	base := at(1, 0).Unix()
	bars := m5(base,
		[4]float64{99, 99.5, 98, 99},        // away
		[4]float64{99, 100.05, 99, 99.5},    // touch 1
		[4]float64{99.5, 100, 99.4, 99.6},   // same episode
		[4]float64{99.6, 99.7, 99, 99.2},    // away
		[4]float64{99.2, 100.2, 99.1, 99.3}, // touch 2
	)
	lv := []LiquidityLevel{{Price: 100}, {Price: 105}}
	levelTouches(lv, bars, 1, time.Unix(base+5*300, 0))
	if lv[0].Touches != 2 || lv[0].LastTouchMin != 0 || lv[1].Touches != 0 || lv[1].LastTouchMin != -1 {
		t.Fatalf("touches = %+v", lv)
	}
}

func TestShadowResolvedAtIsExitCandle(t *testing.T) {
	ok := m5(0,
		[4]float64{99, 100.5, 98.8, 100.4},
		[4]float64{100.4, 100.6, 100.1, 100.5}, // trigger
		[4]float64{100.5, 100.6, 99.0, 99.2},   // stop hit here (t=600)
		[4]float64{99.2, 99.3, 98, 98.5},
		[4]float64{98.5, 98.6, 97, 97.5},
	)
	tr := ShadowTrade{Setup: "BREAK_RETEST", Side: "BUY", Level: 100, ATR: 1, ExpiresAt: 1e9, Status: ShadowPending}
	EvaluateShadow(&tr, ok, time.Unix(1e6, 0), 0)
	if tr.Status != ShadowLoss || tr.ResolvedAt != 600 {
		t.Fatalf("want LOSS resolved at 600, got %s at %d", tr.Status, tr.ResolvedAt)
	}
}
