package tvspoof

import (
	"fmt"
	"math"
	"time"
)

// DeltaBiasType indicates the aggressive order flow pressure.
type DeltaBiasType string

const (
	DeltaBullishAggression DeltaBiasType = "🟢 BULLISH AGGRESSION"
	DeltaBearishAggression DeltaBiasType = "🔴 BEARISH AGGRESSION"
	DeltaAbsorption        DeltaBiasType = "🛡️ INSTITUTIONAL ABSORPTION"
	DeltaNeutral           DeltaBiasType = "⚪ NEUTRAL BALANCED"
)

// DeltaBar holds individual candlestick volume delta metrics.
type DeltaBar struct {
	Time       time.Time     `json:"time"`
	Open       float64       `json:"open"`
	High       float64       `json:"high"`
	Low        float64       `json:"low"`
	Close      float64       `json:"close"`
	Volume     float64       `json:"volume"`
	BuyVolume  float64       `json:"buy_volume"`
	SellVolume float64       `json:"sell_volume"`
	Delta      float64       `json:"delta"`
	CVD        float64       `json:"cvd"`
	DeltaRatio float64       `json:"delta_ratio"` // Buy Vol / Total Vol (0.0 to 1.0)
	Bias       DeltaBiasType `json:"bias"`
}

// CVDSummary holds high-level Cumulative Volume Delta telemetry and divergence signals.
type CVDSummary struct {
	TotalVolume           float64   `json:"total_volume"`
	TotalBuyVolume        float64   `json:"total_buy_volume"`
	TotalSellVolume       float64   `json:"total_sell_volume"`
	NetDelta              float64   `json:"net_delta"`
	CurrentCVD            float64   `json:"current_cvd"`
	BuyerDominancePct     float64   `json:"buyer_dominance_pct"`
	SellerDominancePct    float64   `json:"seller_dominance_pct"`
	DivergenceSignal      string    `json:"divergence_signal"`      // "🟢 BULLISH ABSORPTION DIVERGENCE", "🔴 BEARISH EXHAUSTION DIVERGENCE", "BALANCED"
	DivergenceDescription string    `json:"divergence_description"` // Detailed rationale
	InstitutionalDirective string   `json:"institutional_directive"`
	Bars                  []DeltaBar `json:"bars,omitempty"`
}

// CalculateCVD computes intra-bar Delta, Cumulative Volume Delta (CVD), and detects divergences.
func CalculateCVD(bars []Bar) CVDSummary {
	var deltaBars []DeltaBar
	if len(bars) == 0 {
		return CVDSummary{
			DivergenceSignal: "BALANCED",
		}
	}

	runningCVD := 0.0
	totVol := 0.0
	totBuy := 0.0
	totSell := 0.0

	for _, b := range bars {
		cRange := b.High - b.Low
		buyFrac := 0.5
		if cRange > 0 {
			// Price action & displacement volume delta model
			buyFrac = ((b.Close - b.Low) + (b.High - b.Open)) / (2.0 * cRange)
			if buyFrac < 0.05 {
				buyFrac = 0.05
			} else if buyFrac > 0.95 {
				buyFrac = 0.95
			}
		}

		bVol := b.Volume * buyFrac
		sVol := b.Volume - bVol
		delta := bVol - sVol
		runningCVD += delta

		totVol += b.Volume
		totBuy += bVol
		totSell += sVol

		ratio := 0.5
		if b.Volume > 0 {
			ratio = bVol / b.Volume
		}

		var bias DeltaBiasType
		isBullCandle := b.Close >= b.Open
		if ratio >= 0.65 {
			bias = DeltaBullishAggression
		} else if ratio <= 0.35 {
			bias = DeltaBearishAggression
		} else if (!isBullCandle && delta > 0) || (isBullCandle && delta < 0) {
			bias = DeltaAbsorption
		} else {
			bias = DeltaNeutral
		}

		deltaBars = append(deltaBars, DeltaBar{
			Time:       time.Unix(b.Time, 0).UTC(),
			Open:       b.Open,
			High:       b.High,
			Low:        b.Low,
			Close:      b.Close,
			Volume:     b.Volume,
			BuyVolume:  bVol,
			SellVolume: sVol,
			Delta:      delta,
			CVD:        runningCVD,
			DeltaRatio: ratio,
			Bias:       bias,
		})
	}

	buyPct := 50.0
	sellPct := 50.0
	if totVol > 0 {
		buyPct = (totBuy / totVol) * 100.0
		sellPct = (totSell / totVol) * 100.0
	}

	divSignal, divDesc, directive := detectCVDDivergence(deltaBars)

	return CVDSummary{
		TotalVolume:           totVol,
		TotalBuyVolume:        totBuy,
		TotalSellVolume:       totSell,
		NetDelta:              totBuy - totSell,
		CurrentCVD:            runningCVD,
		BuyerDominancePct:     buyPct,
		SellerDominancePct:    sellPct,
		DivergenceSignal:      divSignal,
		DivergenceDescription: divDesc,
		InstitutionalDirective: directive,
		Bars:                  deltaBars,
	}
}

// detectCVDDivergence checks for Price vs CVD Divergences across the last 30 bars.
func detectCVDDivergence(d桿 []DeltaBar) (string, string, string) {
	n := len(d桿)
	if n < 10 {
		return "⚪ BALANCED", "Data candle belum cukup untuk konfirmasi divergensi volume delta.", "Pantau pembentukan baseline volume."
	}

	lookback := 30
	if n < lookback {
		lookback = n
	}
	recentBars := d桿[n-lookback:]

	// Find local swing lows and highs in recent window
	type swing struct {
		idx   int
		price float64
		cvd   float64
	}

	var priceLows []swing
	var priceHighs []swing

	for i := 2; i < len(recentBars)-2; i++ {
		b := recentBars[i]
		// Swing Low
		if b.Low < recentBars[i-1].Low && b.Low < recentBars[i-2].Low &&
			b.Low < recentBars[i+1].Low && b.Low < recentBars[i+2].Low {
			priceLows = append(priceLows, swing{idx: i, price: b.Low, cvd: b.CVD})
		}
		// Swing High
		if b.High > recentBars[i-1].High && b.High > recentBars[i-2].High &&
			b.High > recentBars[i+1].High && b.High > recentBars[i+2].High {
			priceHighs = append(priceHighs, swing{idx: i, price: b.High, cvd: b.CVD})
		}
	}

	// 1. Check Bullish Absorption Divergence (Price Lower Low or Equal Low, but CVD Higher Low)
	if len(priceLows) >= 2 {
		s1 := priceLows[len(priceLows)-2]
		s2 := priceLows[len(priceLows)-1]
		if s2.price <= s1.price && s2.cvd > s1.cvd {
			return "🟢 BULLISH ABSORPTION DIVERGENCE",
				"Harga mencetak Lower Low/Double Bottom, TETAPI Cumulative Volume Delta mencetak Higher Low. Pembeli institusional menyerap pasokan jual dengan Limit Buy masif.",
				"DILARANG SHORT! Prioritaskan Buy Limit di area demand atau tunggu trigger breakout."
		}
	}

	// 2. Check Bearish Exhaustion Divergence (Price Higher High or Equal High, but CVD Lower High)
	if len(priceHighs) >= 2 {
		s1 := priceHighs[len(priceHighs)-2]
		s2 := priceHighs[len(priceHighs)-1]
		if s2.price >= s1.price && s2.cvd < s1.cvd {
			return "🔴 BEARISH EXHAUSTION DIVERGENCE",
				"Harga mencetak Higher High/Double Top, TETAPI Cumulative Volume Delta mencetak Lower High. Agresi pembeli mengering di puncak resisten.",
				"Waspada Fakeout/Reversal! Amankan profit posisi Buy dan hindari kejar harga di resisten."
		}
	}

	// 3. Trend Alignment
	lastCVD := recentBars[len(recentBars)-1].CVD
	firstCVD := recentBars[0].CVD
	if lastCVD > firstCVD {
		return "🟢 BULLISH DELTA MOMENTUM",
			"Cumulative Volume Delta konsisten meningkat seiring waktu. Agresi beli pasar mendominasi aliran order.",
			"Kondisi lelang sehat untuk kelanjutan tren bullish."
	} else if lastCVD < firstCVD {
		return "🔴 BEARISH DELTA PRESSURE",
			"Cumulative Volume Delta konsisten menurun. Agresi jual pasar mendominasi aliran order.",
			"Waspada tekanan jual institusional."
	}

	return "⚪ BALANCED ORDER FLOW",
		"Volume Beli dan Jual berada dalam ekuilibrium seimbang tanpa divergensi ekstrim.",
		"Trading dalam batas Value Area (POC reversion playbook)."
}

// FormatDeltaDisplay returns a colored terminal representation of delta.
func FormatDeltaDisplay(delta float64) string {
	if delta > 0 {
		return "+ " + formatFloat(delta)
	} else if delta < 0 {
		return "- " + formatFloat(math.Abs(delta))
	}
	return "  " + formatFloat(0)
}

func formatFloat(v float64) string {
	if v >= 1000000 {
		return formatNumber(v/1000000) + "M"
	} else if v >= 1000 {
		return formatNumber(v/1000) + "k"
	}
	return formatNumber(v)
}

func formatNumber(v float64) string {
	return formatDouble(v, 1)
}

func formatDouble(v float64, decimals int) string {
	shift := math.Pow10(decimals)
	return fmt.Sprintf("%.*f", decimals, math.Round(v*shift)/shift)
}
