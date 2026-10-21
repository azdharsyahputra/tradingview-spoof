package tvspoof

import (
	"fmt"
	"math"
	"time"
)

// addCandidateFactors adds research factors. They are audited by cmd/backtest
// and only move the score when deskScoreWeights gives them a non-zero weight.
func addCandidateFactors(si scoreInputs, add func(name string, signal float64, detail string)) {
	px, atr := si.CurrentPrice, si.ATR
	if atr <= 0 {
		return
	}

	// Fresh liquidity sweep: one of the last 4 closed M15 candles wicked
	// through PDH/PDL (or the Asian extreme once Asia has closed) and closed
	// back inside — a failed breakout.
	if n := len(si.Closed15M); n >= 4 {
		hour := time.Unix(si.Closed15M[n-1].Time, 0).UTC().Hour()
		lows, highs := []float64{si.Liq.PDL}, []float64{si.Liq.PDH}
		if hour >= 8 {
			lows = append(lows, si.Liq.AsiaLow)
			highs = append(highs, si.Liq.AsiaHigh)
		}
		sig := 0.0
		for _, b := range si.Closed15M[n-4:] {
			for _, l := range lows {
				if l > 0 && b.Low < l && b.Close > l {
					sig = 1
				}
			}
			for _, h := range highs {
				if h > 0 && b.High > h && b.Close < h {
					sig = -1
				}
			}
		}
		add("fresh_sweep", sig, "PDH/PDL/Asia wick-and-reclaim in last 4 M15 candles")
	}

	if ema := calculateEMA(si.Closed4H, 50); ema > 0 {
		add("h4_trend", math.Copysign(1, px-ema), fmt.Sprintf("EMA50 H4 %.2f", ema))
	}
	if ema := calculateEMA(si.ClosedD, 20); ema > 0 {
		add("d1_trend", math.Copysign(1, px-ema), fmt.Sprintf("EMA20 D1 %.2f", ema))
	}
	if n := len(si.Closed15M); n > 16 {
		mom := (si.Closed15M[n-1].Close - si.Closed15M[n-17].Close) / (2 * atr)
		add("momentum_4h", mom, "16-bar M15 return / 2 ATR")
	}
	if si.VP.POC > 0 {
		add("poc_reversion", (si.VP.POC-px)/(2*atr), "distance to POC / 2 ATR")
	}
}

// DeskScoreWeights returns a copy of the conviction-score weights.
func DeskScoreWeights() map[string]float64 {
	out := make(map[string]float64, len(deskScoreWeights))
	for k, v := range deskScoreWeights {
		out[k] = v
	}
	return out
}

// SetDeskScoreWeights overrides weights (used by cmd/backtest experiments).
func SetDeskScoreWeights(w map[string]float64) {
	for k, v := range w {
		deskScoreWeights[k] = v
	}
}

// SetDeskNeutralBand overrides the no-trade band (used by cmd/backtest).
func SetDeskNeutralBand(band int) { deskNeutralBand = band }
