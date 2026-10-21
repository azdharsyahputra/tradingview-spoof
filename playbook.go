package tvspoof

import (
	"fmt"
	"math"
)

// buildPlaybook converts the conviction score into an executable CFD plan.
//
// Exit model (Lesson-008): 50% closed at TP1, stop moved to breakeven, runner
// held to TP2. EstimatedWinRate is P(TP1 before SL). Without a backtest
// calibration it falls back to the driftless random-walk barrier probability,
// which by construction has zero expectancy before costs — the honest prior
// when no out-of-sample evidence exists.
func buildPlaybook(symbol string, currentPrice float64, precision int, score int,
	vp VolumeProfile, liq SessionLiquidity, smc SMCMetrics, atr float64,
	balance, riskPerc float64, calib *Calibration) TradePlaybook {

	long := score >= 50
	direction := "SHORT"
	if long {
		direction = "LONG"
	}
	edge := int(math.Abs(float64(score - 50)))
	tradeable := edge >= deskNeutralBand

	entry := currentPrice
	sl, tp1, tp2 := planLevels(long, entry, vp, liq, smc, atr)
	slDist := math.Abs(entry - sl)
	r1 := math.Abs(tp1-entry) / slDist
	r2 := math.Abs(tp2-entry) / slDist

	spec := ContractSpecFor(symbol, currentPrice)
	riskBudget := balance * (riskPerc / 100.0)
	lots := math.Floor(riskBudget/(slDist*spec.USDPerPointPerLot)*100) / 100
	exceeds := false
	if lots < 0.01 {
		lots = 0.01
		exceeds = true
	}
	actualRisk := lots * slDist * spec.USDPerPointPerLot

	p1, p21 := 1/(1+r1), r1/r2
	source := "random-walk baseline (no backtest calibration)"
	samples := 0
	if b := calib.Lookup(symbol, direction, edge); b != nil {
		p1, p21 = b.TP1Rate, b.RunnerRate
		samples = b.Samples
		source = fmt.Sprintf("backtest calibration (n=%d, edge %d-%d)", b.Samples, b.MinEdge, b.MaxEdge)
	}
	expR := managedExpectancyR(p1, p21, r1, r2) - spec.TypicalSpread/slDist

	pb := TradePlaybook{
		EntryPrice:         entry,
		InvalidationPrice:  sl,
		Target1Price:       tp1,
		Target2Price:       tp2,
		RiskRewardRatio:    math.Round(r2*100) / 100,
		EstimatedWinRate:   math.Round(p1*1000) / 10,
		RecommendedLots:    lots,
		RiskAmountUSD:      actualRisk,
		ExpectedValueUSD:   expR * actualRisk,
		Direction:          direction,
		RiskBudgetUSD:      riskBudget,
		RiskExceedsBudget:  exceeds,
		MarginRequiredUSD:  lots * spec.MarginPerLot(currentPrice),
		RunnerProbability:  math.Round(p21*1000) / 10,
		ExpectancyR:        math.Round(expR*100) / 100,
		WinRateSource:      source,
		CalibrationSamples: samples,
	}

	if long {
		pb.PrimaryPlan = "LONG PULLBACK / DISCOUNT ACCUMULATION"
		pb.OptimalEntryZone = fmt.Sprintf("%.*f – %.*f", precision, sl+(entry-sl)*0.5, precision, entry)
		pb.ThesisSummary = fmt.Sprintf("Long dengan invalidasi struktural %s, TP1 %s (%.2fR) lalu runner ke %s (%.2fR).",
			FormatDeskPrice(sl, precision), FormatDeskPrice(tp1, precision), r1, FormatDeskPrice(tp2, precision), r2)
	} else {
		pb.PrimaryPlan = "SHORT BREAKDOWN / PREMIUM DISTRIBUTION"
		pb.OptimalEntryZone = fmt.Sprintf("%.*f – %.*f", precision, entry, precision, sl-(sl-entry)*0.5)
		pb.ThesisSummary = fmt.Sprintf("Short dengan invalidasi struktural %s, TP1 %s (%.2fR) lalu runner ke %s (%.2fR).",
			FormatDeskPrice(sl, precision), FormatDeskPrice(tp1, precision), r1, FormatDeskPrice(tp2, precision), r2)
	}
	if !tradeable {
		pb.Direction = "WAIT"
		pb.PrimaryPlan = fmt.Sprintf("WAIT / NO EDGE (lean %s, score %d inside neutral band ±%d)", direction, score, deskNeutralBand)
		pb.ThesisSummary = "Tidak ada edge statistik: " + pb.ThesisSummary
	} else if expR <= 0 {
		pb.PrimaryPlan += " — NEGATIVE EXPECTANCY, SKIP"
	}
	return pb
}

// planLevels picks a structural stop beyond the nearest protective swing /
// session extreme / value edge (+0.25 ATR buffer), with the stop distance
// clamped to [1, 3] ATR, then targets value reversion (TP1, min 1.5R) and the
// prior-day liquidity pool (TP2, min TP1 + 1.5R).
func planLevels(long bool, entry float64, vp VolumeProfile, liq SessionLiquidity, smc SMCMetrics, atr float64) (sl, tp1, tp2 float64) {
	dir := 1.0
	candidates := []float64{smc.StructureSeq.LastLow.Price, liq.AsiaLow, vp.VAL}
	if !long {
		dir = -1
		candidates = []float64{smc.StructureSeq.LastHigh.Price, liq.AsiaHigh, vp.VAH}
	}

	// Nearest protective level at least 0.5 ATR away on the stop side.
	best := 0.0
	for _, c := range candidates {
		if c <= 0 {
			continue
		}
		d := (entry - c) * dir
		if d >= 0.5*atr && (best == 0 || d < best) {
			best = d
		}
	}
	slDist := 1.5 * atr
	if best > 0 {
		slDist = best + 0.25*atr
	}
	slDist = math.Max(atr, math.Min(3*atr, slDist))
	sl = entry - dir*slDist

	tp1Dist := 1.5 * slDist
	if vp.POC > 0 {
		if d := (vp.POC - entry) * dir; d > tp1Dist {
			tp1Dist = d
		}
	}
	tp1 = entry + dir*tp1Dist

	tp2Dist := tp1Dist + 1.5*slDist
	pool := liq.PDH
	if !long {
		pool = liq.PDL
	}
	if pool > 0 {
		if d := (pool - entry) * dir; d > tp2Dist {
			tp2Dist = d
		}
	}
	tp2 = entry + dir*tp2Dist
	return sl, tp1, tp2
}

// managedExpectancyR is the per-trade expectancy in R of the TP1-partial /
// breakeven-runner exit model.
func managedExpectancyR(p1, p21, r1, r2 float64) float64 {
	return p1*(0.5*r1+0.5*p21*r2) - (1 - p1)
}
