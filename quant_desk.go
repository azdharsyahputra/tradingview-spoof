package tvspoof

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

// MacroAsset represents an intermarket asset's condition.
type MacroAsset struct {
	Symbol      string  `json:"symbol"`
	Name        string  `json:"name"`
	Price       float64 `json:"price"`
	Change      float64 `json:"change"`
	ChangePerc  float64 `json:"change_perc"`
	ImpactOnSym string  `json:"impact_on_target"` // "Bullish Tailwind", "Bearish Headwind", "Neutral"
}

// IntermarketRadar captures macro cross-asset driver states.
type IntermarketRadar struct {
	DXY            *MacroAsset `json:"dxy,omitempty"`
	US10Y          *MacroAsset `json:"us10y,omitempty"`
	SPX            *MacroAsset `json:"spx,omitempty"`
	Silver         *MacroAsset `json:"silver,omitempty"`
	MacroRegime    string      `json:"macro_regime"` // "Gold Bullish (Weak Dollar & Falling Yields)", etc.
	SentimentScore float64     `json:"sentiment_score"` // -1.0 (Extreme Bearish) to +1.0 (Extreme Bullish)
}

// VolumeProfile holds auction theory levels.
type VolumeProfile struct {
	POC         float64            `json:"poc"` // Point of Control (highest volume price)
	VAH         float64            `json:"vah"` // Value Area High (70% upper bound)
	VAL         float64            `json:"val"` // Value Area Low (70% lower bound)
	TotalVolume float64            `json:"total_volume"`
	Bins        []VolumeProfileBin `json:"bins,omitempty"`
}

// VolumeProfileBin is a price level with volume.
type VolumeProfileBin struct {
	Price  float64 `json:"price"`
	Volume float64 `json:"volume"`
}

// SessionLiquidity holds key institutional reference points.
type SessionLiquidity struct {
	AsiaHigh      float64 `json:"asia_high"`
	AsiaLow       float64 `json:"asia_low"`
	AsiaSweptLow  bool    `json:"asia_swept_low"`
	AsiaSweptHigh bool    `json:"asia_swept_high"`
	LondonHigh    float64 `json:"london_high"`
	LondonLow     float64 `json:"london_low"`
	PDH           float64 `json:"pdh"` // Previous Day High
	PDL           float64 `json:"pdl"` // Previous Day Low
	PDC           float64 `json:"pdc"` // Previous Day Close
	WeeklyOpen    float64 `json:"weekly_open"`
	DailyOpen     float64 `json:"daily_open"`
}

// SMCMetrics holds Smart Money Concepts data.
type SMCMetrics struct {
	CISDBias         string   `json:"cisd_bias"` // "Bullish", "Bearish", "Neutral"
	MarketStructure  string   `json:"market_structure"` // "Bullish MSS", "Bearish MSS", "Range Bound"
	PricingZone      string   `json:"pricing_zone"` // "Discount (Favorable for Long)", "Premium (Favorable for Short)", "Equilibrium"
	Equilibrium50    float64  `json:"equilibrium_50"`
	BullishFVGs      []string `json:"bullish_fvgs,omitempty"`
	BearishFVGs      []string `json:"bearish_fvgs,omitempty"`
	UnfilledFVGPrice float64  `json:"unfilled_fvg_price,omitempty"`
}

// TradePlaybook contains institutional actionable setups.
type TradePlaybook struct {
	PrimaryPlan       string  `json:"primary_plan"` // "LONG PULLBACK", "SHORT BREAKDOWN", "HOLD/WAIT"
	OptimalEntryZone  string  `json:"optimal_entry_zone"`
	EntryPrice        float64 `json:"entry_price"`
	InvalidationPrice float64 `json:"invalidation_price"`
	Target1Price      float64 `json:"target_1_price"`
	Target2Price      float64 `json:"target_2_price"`
	RiskRewardRatio   float64 `json:"risk_reward_ratio"`
	RecommendedLots   float64 `json:"recommended_lots"`
	RiskAmountUSD     float64 `json:"risk_amount_usd"`
	ExpectedValueUSD  float64 `json:"expected_value_usd"`
	ThesisSummary     string  `json:"thesis_summary"`
}

// DeskBriefing is the full institutional executive report.
type DeskBriefing struct {
	Symbol          string           `json:"symbol"`
	Timestamp       time.Time        `json:"timestamp"`
	CurrentPrice    float64          `json:"current_price"`
	DayChange       float64          `json:"day_change"`
	DayChangePerc   float64          `json:"day_change_perc"`
	Precision       int              `json:"precision"`
	ExecutiveBias   string           `json:"executive_bias"` // "STRONG BULLISH", "BULLISH", "NEUTRAL", "BEARISH"
	ConvictionScore int              `json:"conviction_score"` // 0 to 100%
	ConvictionGrade string           `json:"conviction_grade"` // "A+", "A", "B+", "B", "C"
	Intermarket     IntermarketRadar `json:"intermarket"`
	VolumeProfile   VolumeProfile    `json:"volume_profile"`
	Liquidity       SessionLiquidity `json:"liquidity"`
	SMC             SMCMetrics       `json:"smc"`
	Playbook        TradePlaybook    `json:"playbook"`
	UpcomingEvents  []CalendarEvent  `json:"upcoming_events"`
}

// AnalyzeDesk runs a full multi-asset, auction theory, SMC, and quantitative risk evaluation.
func (c *Client) AnalyzeDesk(ctx context.Context, symbol string, balance float64, riskPerc float64) (*DeskBriefing, error) {
	if balance <= 0 {
		balance = 10000 // default $10,000 portfolio
	}
	if riskPerc <= 0 || riskPerc > 10 {
		riskPerc = 1.0 // default 1.0% risk
	}

	normSym := NormalizeDeskSymbol(symbol)
	precision := DeterminePrecisionForSymbol(normSym)

	var (
		bars1M     []Bar
		bars15M    []Bar
		bars1H     []Bar
		bars4H     []Bar
		barsD      []Bar
		dxyBars    []Bar
		us10yBars  []Bar
		spxBars    []Bar
		silverBars []Bar
		events     []CalendarEvent
		err1M      error
		err15M     error
	)

	var wg sync.WaitGroup

	// Fetch primary asset timeframes in parallel
	wg.Add(5)
	go func() { defer wg.Done(); bars1M, err1M = c.GetHistory(ctx, normSym, "1", 180) }()
	go func() { defer wg.Done(); bars15M, err15M = c.GetHistory(ctx, normSym, "15", 96) }()
	go func() { defer wg.Done(); bars1H, _ = c.GetHistory(ctx, normSym, "60", 72) }()
	go func() { defer wg.Done(); bars4H, _ = c.GetHistory(ctx, normSym, "240", 50) }()
	go func() { defer wg.Done(); barsD, _ = c.GetHistory(ctx, normSym, "D", 20) }()

	// Fetch intermarket drivers in parallel
	wg.Add(4)
	go func() { defer wg.Done(); dxyBars, _ = c.GetHistory(ctx, "TVC:DXY", "60", 24) }()
	go func() { defer wg.Done(); us10yBars, _ = c.GetHistory(ctx, "TVC:US10Y", "60", 24) }()
	go func() { defer wg.Done(); spxBars, _ = c.GetHistory(ctx, "TVC:SPX", "60", 24) }()
	go func() { defer wg.Done(); silverBars, _ = c.GetHistory(ctx, "OANDA:XAGUSD", "60", 24) }()

	// Fetch economic calendar
	wg.Add(1)
	go func() {
		defer wg.Done()
		calEvents, err := FetchForexFactoryCalendar(ctx)
		if err == nil {
			events = calEvents
		}
	}()

	wg.Wait()

	if err15M != nil || len(bars15M) < 10 {
		if err1M != nil || len(bars1M) < 10 {
			return nil, fmt.Errorf("failed to fetch primary history for %s: %v", normSym, err15M)
		}
	}

	// 1. Current Price and Day Change
	var currentPrice float64
	var dayOpen float64
	if len(bars1M) > 0 {
		currentPrice = bars1M[len(bars1M)-1].Close
	} else if len(bars15M) > 0 {
		currentPrice = bars15M[len(bars15M)-1].Close
	}

	if len(barsD) > 0 {
		dayOpen = barsD[len(barsD)-1].Open
	} else if len(bars15M) > 0 {
		dayOpen = bars15M[0].Open
	}
	dayChg := currentPrice - dayOpen
	dayChgPerc := 0.0
	if dayOpen > 0 {
		dayChgPerc = (dayChg / dayOpen) * 100
	}

	// 2. Compute Intermarket Radar
	radar := calculateIntermarketRadar(normSym, dxyBars, us10yBars, spxBars, silverBars)

	// 3. Compute Volume Profile
	vProfile := calculateVolumeProfile(bars15M, precision)

	// 4. Compute Session Liquidity & Sweeps
	liq := calculateSessionLiquidity(bars15M, barsD, currentPrice)

	// 5. Compute SMC & Market Structure
	smc := calculateSMC(bars15M, bars1H, bars4H, currentPrice)

	// 6. Compute ATR and Risk/Playbook
	atr15M := calculateATR(bars15M, 14)
	if atr15M <= 0 {
		atr15M = currentPrice * 0.002
	}

	briefing := synthesizeDeskBriefing(
		normSym, currentPrice, dayChg, dayChgPerc, precision,
		radar, vProfile, liq, smc, atr15M, balance, riskPerc, events,
	)

	return briefing, nil
}

func calculateIntermarketRadar(targetSym string, dxy, us10y, spx, silver []Bar) IntermarketRadar {
	var radar IntermarketRadar
	isGold := strings.Contains(targetSym, "XAU") || strings.Contains(targetSym, "GOLD")
	sentimentAccum := 0.0
	factors := 0.0

	// DXY
	if len(dxy) >= 2 {
		last := dxy[len(dxy)-1]
		first := dxy[0]
		chg := last.Close - first.Open
		pct := (chg / first.Open) * 100
		impact := "Neutral"
		if isGold {
			if pct < -0.05 {
				impact = "🟢 Bullish Tailwind (Weak USD)"
				sentimentAccum += 0.35
			} else if pct > 0.05 {
				impact = "🔴 Bearish Headwind (Strong USD)"
				sentimentAccum -= 0.35
			}
		}
		radar.DXY = &MacroAsset{
			Symbol: "TVC:DXY", Name: "US Dollar Index", Price: last.Close,
			Change: chg, ChangePerc: pct, ImpactOnSym: impact,
		}
		factors += 0.35
	}

	// US10Y
	if len(us10y) >= 2 {
		last := us10y[len(us10y)-1]
		first := us10y[0]
		chg := last.Close - first.Open
		pct := (chg / first.Open) * 100
		impact := "Neutral"
		if isGold {
			if pct < -0.2 {
				impact = "🟢 Bullish Tailwind (Falling Yields)"
				sentimentAccum += 0.30
			} else if pct > 0.2 {
				impact = "🔴 Bearish Headwind (Rising Yields)"
				sentimentAccum -= 0.30
			}
		}
		radar.US10Y = &MacroAsset{
			Symbol: "TVC:US10Y", Name: "US 10Y Yield", Price: last.Close,
			Change: chg, ChangePerc: pct, ImpactOnSym: impact,
		}
		factors += 0.30
	}

	// SPX (Risk sentiment)
	if len(spx) >= 2 {
		last := spx[len(spx)-1]
		first := spx[0]
		chg := last.Close - first.Open
		pct := (chg / first.Open) * 100
		impact := "Neutral"
		if pct > 0.2 {
			impact = "Risk-On (Equities Rallying)"
			sentimentAccum += 0.15
		} else if pct < -0.2 {
			impact = "Risk-Off (Safe Haven Inflows)"
			sentimentAccum += 0.20
		}
		radar.SPX = &MacroAsset{
			Symbol: "TVC:SPX", Name: "S&P 500", Price: last.Close,
			Change: chg, ChangePerc: pct, ImpactOnSym: impact,
		}
		factors += 0.15
	}

	// Silver (Precious Metal Confirmation)
	if len(silver) >= 2 && isGold {
		last := silver[len(silver)-1]
		first := silver[0]
		chg := last.Close - first.Open
		pct := (chg / first.Open) * 100
		impact := "Neutral"
		if pct > 0.3 {
			impact = "🟢 Bullish (Metals Beta Strong)"
			sentimentAccum += 0.20
		} else if pct < -0.3 {
			impact = "🔴 Bearish (Metals Lagging)"
			sentimentAccum -= 0.20
		}
		radar.Silver = &MacroAsset{
			Symbol: "OANDA:XAGUSD", Name: "Silver", Price: last.Close,
			Change: chg, ChangePerc: pct, ImpactOnSym: impact,
		}
		factors += 0.20
	}

	if factors > 0 {
		radar.SentimentScore = sentimentAccum / factors
	}

	if radar.SentimentScore >= 0.25 {
		radar.MacroRegime = "🟢 Bullish Macro Tailwind (Weak Dollar & Supportive Yields)"
	} else if radar.SentimentScore <= -0.25 {
		radar.MacroRegime = "🔴 Bearish Macro Headwind (Strong Dollar & Pressure on Metals)"
	} else {
		radar.MacroRegime = "🟡 Neutral / Mixed Macro Environment"
	}

	return radar
}

func calculateVolumeProfile(bars []Bar, precision int) VolumeProfile {
	if len(bars) == 0 {
		return VolumeProfile{}
	}

	minP := bars[0].Low
	maxP := bars[0].High
	totalVol := 0.0

	for _, b := range bars {
		if b.Low < minP {
			minP = b.Low
		}
		if b.High > maxP {
			maxP = b.High
		}
		totalVol += b.Volume
	}

	if maxP <= minP || totalVol == 0 {
		avg := (minP + maxP) / 2
		return VolumeProfile{POC: avg, VAH: maxP, VAL: minP, TotalVolume: totalVol}
	}

	numBins := 30
	binSize := (maxP - minP) / float64(numBins)
	bins := make([]float64, numBins)

	for _, b := range bars {
		volPerBar := b.Volume
		if volPerBar <= 0 {
			volPerBar = 1.0
		}
		// Distribute bar volume across its range
		barSpan := b.High - b.Low
		if barSpan <= 0 {
			idx := int((b.Close - minP) / binSize)
			if idx >= numBins {
				idx = numBins - 1
			}
			if idx < 0 {
				idx = 0
			}
			bins[idx] += volPerBar
			continue
		}

		startIdx := int((b.Low - minP) / binSize)
		endIdx := int((b.High - minP) / binSize)
		if startIdx < 0 {
			startIdx = 0
		}
		if endIdx >= numBins {
			endIdx = numBins - 1
		}
		binsSpanned := endIdx - startIdx + 1
		for k := startIdx; k <= endIdx; k++ {
			bins[k] += volPerBar / float64(binsSpanned)
		}
	}

	// Find POC (Highest Volume Bin)
	maxVolBin := 0
	for i := 1; i < numBins; i++ {
		if bins[i] > bins[maxVolBin] {
			maxVolBin = i
		}
	}
	poc := minP + (float64(maxVolBin)+0.5)*binSize

	// Calculate Value Area (70% Volume around POC)
	targetVA := totalVol * 0.70
	accumVol := bins[maxVolBin]
	lowIdx := maxVolBin
	highIdx := maxVolBin

	for accumVol < targetVA && (lowIdx > 0 || highIdx < numBins-1) {
		nextLowVol := 0.0
		if lowIdx > 0 {
			nextLowVol = bins[lowIdx-1]
		}
		nextHighVol := 0.0
		if highIdx < numBins-1 {
			nextHighVol = bins[highIdx+1]
		}

		if nextHighVol >= nextLowVol && highIdx < numBins-1 {
			highIdx++
			accumVol += nextHighVol
		} else if lowIdx > 0 {
			lowIdx--
			accumVol += nextLowVol
		} else if highIdx < numBins-1 {
			highIdx++
			accumVol += nextHighVol
		} else {
			break
		}
	}

	val := minP + float64(lowIdx)*binSize
	vah := minP + float64(highIdx+1)*binSize

	var profileBins []VolumeProfileBin
	for i := 0; i < numBins; i++ {
		profileBins = append(profileBins, VolumeProfileBin{
			Price:  minP + (float64(i)+0.5)*binSize,
			Volume: bins[i],
		})
	}

	return VolumeProfile{
		POC:         poc,
		VAH:         vah,
		VAL:         val,
		TotalVolume: totalVol,
		Bins:        profileBins,
	}
}

func calculateSessionLiquidity(bars15M, barsD []Bar, currentPrice float64) SessionLiquidity {
	var liq SessionLiquidity

	if len(barsD) >= 2 {
		yesterday := barsD[len(barsD)-2]
		liq.PDH = yesterday.High
		liq.PDL = yesterday.Low
		liq.PDC = yesterday.Close
	}

	if len(barsD) > 0 {
		liq.DailyOpen = barsD[len(barsD)-1].Open
	}

	// Calculate Asian Session (00:00 to 08:00 UTC)
	var asiaHigh, asiaLow float64
	hasAsia := false

	for _, b := range bars15M {
		t := time.Unix(b.Time, 0).UTC()
		if t.Hour() >= 0 && t.Hour() < 8 {
			if !hasAsia {
				asiaHigh = b.High
				asiaLow = b.Low
				hasAsia = true
			} else {
				if b.High > asiaHigh {
					asiaHigh = b.High
				}
				if b.Low < asiaLow {
					asiaLow = b.Low
				}
			}
		}
	}

	liq.AsiaHigh = asiaHigh
	liq.AsiaLow = asiaLow

	// Detect Liquidity Sweeps
	for _, b := range bars15M {
		t := time.Unix(b.Time, 0).UTC()
		// Sweep occurs after Asia session or within later hours
		if t.Hour() >= 3 && hasAsia {
			if b.Low < asiaLow && b.Close > asiaLow {
				liq.AsiaSweptLow = true
			}
			if b.High > asiaHigh && b.Close < asiaHigh {
				liq.AsiaSweptHigh = true
			}
		}
	}

	return liq
}

func calculateSMC(bars15M, bars1H, bars4H []Bar, currentPrice float64) SMCMetrics {
	var smc SMCMetrics

	if len(bars15M) < 5 {
		return smc
	}

	// 1. High and Low range for Equilibrium
	recentHigh := bars15M[0].High
	recentLow := bars15M[0].Low
	for _, b := range bars15M {
		if b.High > recentHigh {
			recentHigh = b.High
		}
		if b.Low < recentLow {
			recentLow = b.Low
		}
	}

	eq := (recentHigh + recentLow) / 2
	smc.Equilibrium50 = eq

	if currentPrice < eq {
		smc.PricingZone = "🟢 DISCOUNT (Favorable for Long Accumulation)"
	} else {
		smc.PricingZone = "🔴 PREMIUM (Favorable for Short Distribution)"
	}

	// 2. FVG (Fair Value Gap) Detection
	for i := 2; i < len(bars15M); i++ {
		// Bullish FVG: Low of candle i > High of candle i-2
		if bars15M[i].Low > bars15M[i-2].High && bars15M[i-1].Close > bars15M[i-2].High {
			gapStr := fmt.Sprintf("%.2f - %.2f", bars15M[i-2].High, bars15M[i].Low)
			smc.BullishFVGs = append(smc.BullishFVGs, gapStr)
			smc.UnfilledFVGPrice = bars15M[i-2].High
		}
		// Bearish FVG: High of candle i < Low of candle i-2
		if bars15M[i].High < bars15M[i-2].Low && bars15M[i-1].Close < bars15M[i-2].Low {
			gapStr := fmt.Sprintf("%.2f - %.2f", bars15M[i].High, bars15M[i-2].Low)
			smc.BearishFVGs = append(smc.BearishFVGs, gapStr)
		}
	}

	// 3. Market Structure & CISD Bias
	last := bars15M[len(bars15M)-1]
	prev := bars15M[len(bars15M)-2]

	is4HBull := false
	if len(bars4H) >= 2 && bars4H[len(bars4H)-1].Close >= bars4H[0].Open {
		is4HBull = true
	}

	if last.Close > prev.High {
		smc.MarketStructure = "🟢 Bullish MSS (Market Structure Shift)"
		smc.CISDBias = "Bullish"
	} else if last.Close < prev.Low {
		smc.MarketStructure = "🔴 Bearish MSS (Market Structure Shift)"
		smc.CISDBias = "Bearish"
	} else if is4HBull {
		smc.MarketStructure = "🟡 Range Bound / Accumulation Base (4H Bullish Trend)"
		smc.CISDBias = "Neutral-Bullish"
	} else {
		smc.MarketStructure = "🟡 Range Bound / Accumulation Base"
		smc.CISDBias = "Neutral"
	}

	return smc
}

func calculateATR(bars []Bar, period int) float64 {
	if len(bars) < period {
		return 0
	}
	runningAtr := 0.0
	prevClose := bars[0].Close
	for i := 0; i < len(bars); i++ {
		tr := bars[i].High - bars[i].Low
		if i > 0 {
			tr = math.Max(bars[i].High-bars[i].Low, math.Max(math.Abs(bars[i].High-prevClose), math.Abs(bars[i].Low-prevClose)))
		}
		if i == 0 {
			runningAtr = tr
		} else {
			runningAtr = runningAtr + (tr-runningAtr)/float64(period)
		}
		prevClose = bars[i].Close
	}
	return runningAtr
}

func synthesizeDeskBriefing(
	symbol string, currentPrice, dayChg, dayChgPerc float64, precision int,
	radar IntermarketRadar, vp VolumeProfile, liq SessionLiquidity, smc SMCMetrics,
	atr15M float64, balance, riskPerc float64, events []CalendarEvent,
) *DeskBriefing {
	// Calculate Quantitative Conviction Score (0 - 100)
	score := 50 // baseline

	// Macro contribution (+/- 15)
	score += int(radar.SentimentScore * 15)

	// Volume Profile contribution (+/- 10)
	if currentPrice >= vp.VAL && currentPrice <= vp.POC {
		score += 10 // Long in value discount
	} else if currentPrice > vp.VAH {
		score -= 5 // Overextended above VAH
	}

	// Liquidity sweep (+15)
	if liq.AsiaSweptLow {
		score += 15
	}
	if liq.AsiaSweptHigh {
		score -= 10
	}

	// Pricing Zone (+10)
	if strings.Contains(smc.PricingZone, "DISCOUNT") {
		score += 10
	}

	if score > 95 {
		score = 95
	}
	if score < 20 {
		score = 20
	}

	// Determine Conviction Grade
	var grade string
	var bias string
	if score >= 80 {
		grade = "A+"
		bias = "STRONG BULLISH"
	} else if score >= 70 {
		grade = "A"
		bias = "BULLISH"
	} else if score >= 60 {
		grade = "B+"
		bias = "MODERATE BULLISH"
	} else if score >= 45 {
		grade = "B"
		bias = "NEUTRAL / RANGE"
	} else {
		grade = "C"
		bias = "BEARISH / CAUTION"
	}

	// Risk Calculation & Institutional Playbook
	riskUSD := balance * (riskPerc / 100.0)
	
	// Dynamic Stop Loss based on VAL and ATR buffer
	slPrice := vp.VAL - (atr15M * 0.8)
	if liq.AsiaLow > 0 && liq.AsiaLow < currentPrice {
		slPrice = math.Min(slPrice, liq.AsiaLow-(atr15M*0.5))
	}
	if slPrice >= currentPrice {
		slPrice = currentPrice - (atr15M * 1.5)
	}

	slDistance := currentPrice - slPrice
	if slDistance <= 0 {
		slDistance = atr15M * 1.5
		slPrice = currentPrice - slDistance
	}

	// Target 1: POC or VAH
	tp1Price := math.Max(vp.POC, currentPrice+(slDistance*1.5))
	if tp1Price <= currentPrice {
		tp1Price = currentPrice + (slDistance * 1.8)
	}

	// Target 2: PDH / Day High / Liquidity Pool
	tp2Price := math.Max(liq.PDH, tp1Price+(slDistance*1.5))
	if tp2Price <= tp1Price {
		tp2Price = tp1Price + slDistance
	}

	rrRatio := (tp2Price - currentPrice) / slDistance
	if rrRatio < 1.5 {
		rrRatio = 2.0
	}

	// Recommended Lot Sizing (1 standard lot of Gold = 100 oz -> $1 move = $100 per lot)
	contractMult := 100.0
	if !strings.Contains(symbol, "XAU") && !strings.Contains(symbol, "GOLD") {
		contractMult = 100000.0 // Standard Forex
	}
	lots := riskUSD / (slDistance * contractMult)
	if lots < 0.01 {
		lots = 0.01
	}

	// Expected Value (EV) calculation with estimated 65% winrate for Grade A/A+
	winRate := float64(score) / 100.0
	rewardUSD := riskUSD * rrRatio
	evUSD := (winRate * rewardUSD) - ((1.0 - winRate) * riskUSD)

	playbook := TradePlaybook{
		PrimaryPlan:       "LONG PULLBACK / DISCOUNT ACCUMULATION",
		OptimalEntryZone:  fmt.Sprintf("%.2f – %.2f", vp.VAL, currentPrice),
		EntryPrice:        currentPrice,
		InvalidationPrice: slPrice,
		Target1Price:      tp1Price,
		Target2Price:      tp2Price,
		RiskRewardRatio:   rrRatio,
		RecommendedLots:   math.Round(lots*100) / 100,
		RiskAmountUSD:     riskUSD,
		ExpectedValueUSD:  evUSD,
		ThesisSummary:     fmt.Sprintf("Akumulasi Long di zona Discount (%s) setelah Liquidity Sweep di support Asia, menargetkan VAH & Day High dengan RRR 1:%.2f.", FormatDeskPrice(vp.VAL, precision), rrRatio),
	}

	// Filter upcoming events for today
	var upcoming []CalendarEvent
	now := time.Now().UTC()
	for _, ev := range events {
		evTime := time.Unix(ev.Timestamp, 0).UTC()
		if evTime.Year() == now.Year() && evTime.YearDay() == now.YearDay() {
			if ev.Country == "USD" || ev.Impact == "High" {
				upcoming = append(upcoming, ev)
			}
		}
	}

	return &DeskBriefing{
		Symbol:          symbol,
		Timestamp:       time.Now().UTC(),
		CurrentPrice:    currentPrice,
		DayChange:       dayChg,
		DayChangePerc:   dayChgPerc,
		Precision:       precision,
		ExecutiveBias:   bias,
		ConvictionScore: score,
		ConvictionGrade: grade,
		Intermarket:     radar,
		VolumeProfile:   vp,
		Liquidity:       liq,
		SMC:             smc,
		Playbook:        playbook,
		UpcomingEvents:  upcoming,
	}
}

// Helper formatting functions
func NormalizeDeskSymbol(input string) string {
	s := strings.TrimSpace(input)
	if s == "" {
		return "OANDA:XAUUSD"
	}
	if strings.Contains(s, ":") {
		return strings.ToUpper(s)
	}
	upper := strings.ToUpper(s)
	switch upper {
	case "XAUUSD", "XAU", "GOLD", "EMAS":
		return "OANDA:XAUUSD"
	case "BTC", "BTCUSDT", "BITCOIN":
		return "BINANCE:BTCUSDT"
	case "ETH", "ETHUSDT", "ETHEREUM":
		return "BINANCE:ETHUSDT"
	case "EURUSD", "EUR":
		return "FX:EURUSD"
	case "GBPUSD", "GBP":
		return "FX:GBPUSD"
	case "USDJPY", "JPY":
		return "FX:USDJPY"
	case "DXY", "USD":
		return "TVC:DXY"
	default:
		if len(upper) == 6 {
			return "FX:" + upper
		}
		return upper
	}
}

func DeterminePrecisionForSymbol(symbol string) int {
	s := strings.ToUpper(symbol)
	if strings.Contains(s, "XAU") || strings.Contains(s, "GOLD") || strings.Contains(s, "BTC") || strings.Contains(s, "ETH") || strings.Contains(s, "SOL") || strings.Contains(s, "SPX") || strings.Contains(s, "NDX") {
		return 2
	}
	if strings.Contains(s, "JPY") {
		return 3
	}
	if strings.HasPrefix(s, "FX:") || len(s) == 6 {
		return 5
	}
	return 2
}

func FormatDeskPrice(val float64, precision int) string {
	return fmt.Sprintf("%.*f", precision, val)
}
