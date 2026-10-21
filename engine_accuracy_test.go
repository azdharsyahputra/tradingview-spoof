package tvspoof

import (
	"math"
	"strings"
	"testing"
	"time"
)

func approx(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestContractSpecMatchesExnessCFD(t *testing.T) {
	gold := ContractSpecFor("OANDA:XAUUSD", 4320)
	if gold.USDPerPointPerLot != 100 || !approx(gold.MarginPerLot(4320)*0.01, 21.60, 0.01) {
		t.Fatalf("gold spec wrong: %+v margin=%.2f", gold, gold.MarginPerLot(4320)*0.01)
	}
	btc := ContractSpecFor("BINANCE:BTCUSDT", 86250)
	if btc.USDPerPointPerLot != 1 || !approx(btc.MarginPerLot(86250)*0.01, 4.31, 0.01) {
		t.Fatalf("btc spec wrong: %+v", btc)
	}
	oil := ContractSpecFor("TVC:USOIL", 90.2)
	if !approx(oil.MarginPerLot(90.2)*0.01, 4.51, 0.01) {
		t.Fatalf("oil margin = %.2f", oil.MarginPerLot(90.2)*0.01)
	}
	jpy := ContractSpecFor("FX:USDJPY", 150)
	if !approx(jpy.USDPerPointPerLot, 666.67, 0.01) || !approx(jpy.MarginPerLot(150), 500, 1e-9) {
		t.Fatalf("usdjpy spec wrong: %+v", jpy)
	}
}

func TestNormalizeCFDSymbols(t *testing.T) {
	for in, want := range map[string]string{"btcusd": "BINANCE:BTCUSDT", "usoil": "TVC:USOIL", "xagusd": "OANDA:XAGUSD", "us30": "TVC:DJI"} {
		if got := NormalizeDeskSymbol(in); got != want {
			t.Errorf("NormalizeDeskSymbol(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitFormingDropsLiveCandle(t *testing.T) {
	bars := []Bar{{Time: 0}, {Time: 900}}
	closed, forming := splitForming(bars, 15*time.Minute, time.Unix(1000, 0))
	if len(closed) != 1 || forming == nil || forming.Time != 900 {
		t.Fatalf("expected live bar split off, got %d closed, forming=%v", len(closed), forming)
	}
	closed, forming = splitForming(bars, 15*time.Minute, time.Unix(1800, 0))
	if len(closed) != 2 || forming != nil {
		t.Fatalf("expected all closed")
	}
}

func flatBars(n int, px float64) []Bar {
	bars := make([]Bar, n)
	for i := range bars {
		bars[i] = Bar{Time: int64(i * 900), Open: px, High: px + 1, Low: px - 1, Close: px, Volume: 100}
	}
	return bars
}

func TestMSSFiresOnClosedBreakout(t *testing.T) {
	bars := flatBars(20, 100)
	bars[19] = Bar{Time: 19 * 900, Open: 100, High: 104, Low: 99.5, Close: 103}
	smc := calculateSMC(bars, nil, nil, 103, 2)
	if smc.CISDBias != "Bullish" {
		t.Fatalf("expected bullish MSS, got %q (%s)", smc.CISDBias, smc.MarketStructure)
	}
}

func TestFilledFVGIsNotReported(t *testing.T) {
	bars := flatBars(15, 100)
	bars[5] = Bar{Time: 5 * 900, Open: 100, High: 101, Low: 99, Close: 100.5}
	bars[6] = Bar{Time: 6 * 900, Open: 101, High: 106, Low: 100.8, Close: 105.5}
	bars[7] = Bar{Time: 7 * 900, Open: 105.5, High: 107, Low: 103, Close: 106}
	for i := 8; i < 15; i++ {
		bars[i] = Bar{Time: int64(i * 900), Open: 106, High: 107, Low: 105, Close: 106}
	}
	smc := calculateSMC(bars, nil, nil, 106, 2)
	if len(smc.BullishFVGs) != 1 || smc.UnfilledFVGPrice != 101 {
		t.Fatalf("expected one unfilled FVG at 101, got %v / %.2f", smc.BullishFVGs, smc.UnfilledFVGPrice)
	}
	bars[12].Low = 100.5 // trade back through the gap bottom
	smc = calculateSMC(bars, nil, nil, 106, 2)
	if len(smc.BullishFVGs) != 0 {
		t.Fatalf("filled FVG still reported: %v", smc.BullishFVGs)
	}
}

func TestSPXRiskOnIsHeadwindForGold(t *testing.T) {
	spx := []Bar{{Open: 5000, Close: 5000}, {Open: 5000, Close: 5050}}
	r := calculateIntermarketRadar("OANDA:XAUUSD", nil, nil, nil, spx, nil)
	if r.SentimentScore >= 0 {
		t.Fatalf("risk-on should be bearish for gold, got %.2f", r.SentimentScore)
	}
	r = calculateIntermarketRadar("BINANCE:BTCUSDT", nil, nil, nil, spx, nil)
	if r.SentimentScore <= 0 {
		t.Fatalf("risk-on should be bullish for BTC, got %.2f", r.SentimentScore)
	}
	dxyUp := []Bar{{Open: 100, Close: 100}, {Open: 100, Close: 100.5}}
	if r := calculateIntermarketRadar("FX:USDJPY", dxyUp, nil, nil, nil, nil); r.SentimentScore <= 0 {
		t.Fatalf("strong USD should be bullish for USDJPY, got %.2f", r.SentimentScore)
	}
}

func TestPlaybookSizingAndLevels(t *testing.T) {
	vp := VolumeProfile{POC: 4330, VAH: 4340, VAL: 4300}
	liq := SessionLiquidity{AsiaLow: 4305, AsiaHigh: 4335, PDH: 4360, PDL: 4280}
	var smc SMCMetrics
	smc.StructureSeq.LastLow.Price = 4312
	pb := buildPlaybook("OANDA:XAUUSD", 4320, 2, 70, vp, liq, smc, 4, 10000, 0.1, nil)
	if pb.Direction != "LONG" {
		t.Fatalf("direction = %s", pb.Direction)
	}
	// nearest protective level 4312 (8 pts) + 1 pt buffer = 9 pts, inside [4, 12].
	if !approx(pb.InvalidationPrice, 4311, 1e-9) {
		t.Fatalf("SL = %.2f, want 4311", pb.InvalidationPrice)
	}
	if !(pb.InvalidationPrice < pb.EntryPrice && pb.EntryPrice < pb.Target1Price && pb.Target1Price < pb.Target2Price) {
		t.Fatalf("level ordering broken: %+v", pb)
	}
	// $10 budget / (9 pts * $100/pt/lot) = 0.0111 -> floor 0.01, never above budget.
	if pb.RecommendedLots != 0.01 || pb.RiskAmountUSD > pb.RiskBudgetUSD+1e-9 {
		t.Fatalf("lots=%.2f risk=%.2f budget=%.2f", pb.RecommendedLots, pb.RiskAmountUSD, pb.RiskBudgetUSD)
	}
	if !strings.Contains(pb.WinRateSource, "baseline") {
		t.Fatalf("uncalibrated playbook must say so: %q", pb.WinRateSource)
	}
	// Random-walk baseline expectancy is zero before costs.
	r1 := (pb.Target1Price - pb.EntryPrice) / 9
	r2 := (pb.Target2Price - pb.EntryPrice) / 9
	if !approx(managedExpectancyR(1/(1+r1), r1/r2, r1, r2), 0, 1e-9) {
		t.Fatal("baseline expectancy should be zero")
	}

	wait := buildPlaybook("OANDA:XAUUSD", 4320, 2, 53, vp, liq, smc, 4, 10000, 0.1, nil)
	if wait.Direction != "WAIT" {
		t.Fatalf("score inside neutral band must WAIT, got %s", wait.Direction)
	}
}

func TestCalibrationOverridesBaseline(t *testing.T) {
	c := &Calibration{Symbols: []SymbolCalibration{{Symbol: "OANDA:XAUUSD", Buckets: []CalibrationBucket{
		{Direction: "SHORT", MinEdge: 15, MaxEdge: 24, Samples: 100, TP1Rate: 0.45, RunnerRate: 0.5},
		{Direction: "LONG", MinEdge: 15, MaxEdge: 24, Samples: 5, TP1Rate: 0.9},
	}}}}
	if b := c.Lookup("OANDA:XAUUSD", "SHORT", 20); b == nil || b.TP1Rate != 0.45 {
		t.Fatal("expected SHORT bucket")
	}
	if c.Lookup("OANDA:XAUUSD", "LONG", 20) != nil {
		t.Fatal("under-sampled bucket must be ignored")
	}
	var nilCal *Calibration
	if nilCal.Lookup("x", "LONG", 1) != nil {
		t.Fatal("nil calibration lookup must be safe")
	}
}

func TestAsOfRebuildsFormingHTFWithoutLookahead(t *testing.T) {
	h1 := []Bar{{Time: 0, Open: 1, High: 2, Low: 0.5, Close: 1.5}, {Time: 3600, Open: 1.5, High: 99, Low: 0.1, Close: 50}}
	m15 := []Bar{
		{Time: 3600, Open: 1.5, High: 1.8, Low: 1.4, Close: 1.7, Volume: 1},
		{Time: 4500, Open: 1.7, High: 1.9, Low: 1.6, Close: 1.65, Volume: 2},
	}
	got := asOf(h1, time.Hour, m15, time.Unix(5400, 0))
	last := got[len(got)-1]
	if len(got) != 2 || last.High != 1.9 || last.Low != 1.4 || last.Close != 1.65 || last.Open != 1.5 || last.Volume != 3 {
		t.Fatalf("forming H1 leaked or rebuilt wrong: %+v", last)
	}
}

func TestSimulateManagedTrade(t *testing.T) {
	// Ambiguous bar touching both SL and TP1 counts as a loss.
	tr := BacktestTrade{Entry: 100, SL: 99, TP1: 101.5, TP2: 103}
	simulateManagedTrade(&tr, true, []Bar{{High: 102, Low: 98.5, Close: 100}}, 10, 0)
	if !tr.StoppedOut || tr.R != -1 {
		t.Fatalf("expected -1R stop, got %+v", tr)
	}
	// TP1 then back to entry: +0.75R banked, runner flat.
	tr = BacktestTrade{Entry: 100, SL: 99, TP1: 101.5, TP2: 103}
	simulateManagedTrade(&tr, true, []Bar{{High: 101.6, Low: 99.8, Close: 101}, {High: 101, Low: 99.9, Close: 100}}, 10, 0)
	if !tr.TP1Hit || tr.RunnerHit || !approx(tr.R, 0.75, 1e-9) {
		t.Fatalf("expected +0.75R, got %+v", tr)
	}
	// Short: TP1 then TP2 = 0.5*1.5 + 0.5*3 = 2.25R.
	tr = BacktestTrade{Entry: 100, SL: 101, TP1: 98.5, TP2: 97}
	simulateManagedTrade(&tr, false, []Bar{{High: 100.2, Low: 98.4, Close: 98.6}, {High: 98.9, Low: 96.9, Close: 97.2}}, 10, 0)
	if !tr.RunnerHit || !approx(tr.R, 2.25, 1e-9) {
		t.Fatalf("expected +2.25R, got %+v", tr)
	}
}
