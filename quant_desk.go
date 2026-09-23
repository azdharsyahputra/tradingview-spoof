package tvspoof

import (
	"context"
	"fmt"
	"math"
	"os"
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
	ChangeBps   float64 `json:"change_bps,omitempty"`
	Lookback    string  `json:"lookback,omitempty"`
	AsOf        string  `json:"as_of,omitempty"`
	ImpactOnSym string  `json:"impact_on_target"` // "Bullish Tailwind", "Bearish Headwind", "Neutral"
}

// IntermarketRadar captures macro cross-asset driver states.
type IntermarketRadar struct {
	DXY               *MacroAsset `json:"dxy,omitempty"`
	US10Y             *MacroAsset `json:"us10y,omitempty"`
	US10YReal         *MacroAsset `json:"us10y_real,omitempty"`
	SPX               *MacroAsset `json:"spx,omitempty"`
	Silver            *MacroAsset `json:"silver,omitempty"`
	RatesSignalSource string      `json:"rates_signal_source"`
	MacroRegime       string      `json:"macro_regime"`    // "Gold Bullish (Weak Dollar & Falling Yields)", etc.
	SentimentScore    float64     `json:"sentiment_score"` // -1.0 (Extreme Bearish) to +1.0 (Extreme Bullish)
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

// StructureSwing represents an individual Higher High, Lower High, Higher Low, or Lower Low.
type StructureSwing struct {
	Type        string  `json:"type"`        // "HH", "LH", "HL", "LL"
	Label       string  `json:"label"`       // "Higher High", "Lower High", "Higher Low", "Lower Low"
	Price       float64 `json:"price"`
	TimeStr     string  `json:"time_str"`
	DistancePts float64 `json:"distance_pts"`// Distance from current price
}

// MarketStructureSequence holds the chronological sequence of recent HH/HL/LH/LL.
type MarketStructureSequence struct {
	TrendBias    string         `json:"trend_bias"`    // "Bullish Structure (HH + HL)", "Bearish Structure (LH + LL)", "Range / Mixed"
	LastHigh     StructureSwing `json:"last_high"`     // Most recent swing high (HH or LH)
	PrevHigh     StructureSwing `json:"prev_high"`     // Previous swing high
	LastLow      StructureSwing `json:"last_low"`      // Most recent swing low (HL or LL)
	PrevLow      StructureSwing `json:"prev_low"`      // Previous swing low
	SequenceFlow string         `json:"sequence_flow"` // e.g. "LH @ 4342.93 ➔ LL @ 4315.28"
}

// SMCMetrics holds Smart Money Concepts data.
type SMCMetrics struct {
	CISDBias         string                  `json:"cisd_bias"`         // "Bullish", "Bearish", "Neutral"
	MarketStructure  string                  `json:"market_structure"`  // "Bullish MSS", "Bearish MSS", "Range Bound"
	StructureSeq     MarketStructureSequence `json:"structure_seq"`     // HH / HL / LH / LL details
	PricingZone      string                  `json:"pricing_zone"`      // "Discount (Favorable for Long)", "Premium (Favorable for Short)", "Equilibrium"
	Equilibrium50    float64                 `json:"equilibrium_50"`
	BullishFVGs      []string                `json:"bullish_fvgs,omitempty"`
	BearishFVGs      []string                `json:"bearish_fvgs,omitempty"`
	UnfilledFVGPrice float64                 `json:"unfilled_fvg_price,omitempty"`
	// Order Block (OB) — last opposing candle before impulsive move
	BullishOB        string                  `json:"bullish_ob,omitempty"`  // e.g. "4348.00 - 4352.00"
	BearishOB        string                  `json:"bearish_ob,omitempty"`  // e.g. "4390.00 - 4395.00"
	// Break of Structure (BOS) — trend continuation via swing break
	BOSDirection     string                  `json:"bos_direction"`         // "Bullish BOS", "Bearish BOS", "None"
	BOSLevel         float64                 `json:"bos_level,omitempty"`   // The swing level that was broken
	// Change of Character (ChoCH) — trend reversal signal
	ChoCHDetected    bool                    `json:"choch_detected"`
	ChoCHDirection   string                  `json:"choch_direction"`       // "Bullish ChoCH", "Bearish ChoCH", ""
	ChoCHLevel       float64                 `json:"choch_level,omitempty"` // The swing level where ChoCH occurred
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
	EstimatedWinRate  float64 `json:"estimated_win_rate"` // Estimated statistical win probability (e.g. 74.0 for 74%)
	RecommendedLots   float64 `json:"recommended_lots"`
	RiskAmountUSD     float64 `json:"risk_amount_usd"`
	ExpectedValueUSD  float64 `json:"expected_value_usd"`
	ThesisSummary     string  `json:"thesis_summary"`
}

// CandleMorphology holds exact mathematical proportions and geometric shape of a candle.
type CandleMorphology struct {
	TimeStr       string  `json:"time_str"`
	Open          float64 `json:"open"`
	High          float64 `json:"high"`
	Low           float64 `json:"low"`
	Close         float64 `json:"close"`
	Volume        float64 `json:"volume"`
	Change        float64 `json:"change"`
	Range         float64 `json:"range"`
	BodySize      float64 `json:"body_size"`
	UpperWick     float64 `json:"upper_wick"`
	LowerWick     float64 `json:"lower_wick"`
	BodyPercent   float64 `json:"body_percent"`
	UpperWickPerc float64 `json:"upper_wick_percent"`
	LowerWickPerc float64 `json:"lower_wick_percent"`
	Pattern       string  `json:"pattern"` // e.g. "Upper Wick Rejection (Shooting Star)", "Bullish Marubozu", "Dragonfly Doji"
	Summary       string  `json:"summary"`
}

// TimeframeAnalysis holds indicators, momentum, and structural flow for a single timeframe.
type TimeframeAnalysis struct {
	TF            string             `json:"tf"`        // e.g. "M1 (1-Min)", "M5 (5-Min)", "M15 (15-Min)", "H1 (1-Hour)", "H4 (4-Hour)", "D1 (Daily)"
	Bias          string             `json:"bias"`      // "BULLISH", "BEARISH", "NEUTRAL"
	Structure     string             `json:"structure"` // "Lower Highs / Rejection", "Higher Highs / Bullish Flow", "Range Bound"
	RSI           float64            `json:"rsi"`
	EMA21         float64            `json:"ema21"`
	Candle        string             `json:"candle"`    // "🟢 Bull (+1.20)", "🔴 Bear (-2.40)"
	LatestCandle  CandleMorphology   `json:"latest_candle"`
	RecentCandles []CandleMorphology `json:"recent_candles,omitempty"`
}

// MTFMatrix aggregates multi-timeframe trend and momentum alignment across M1 to D1.
type MTFMatrix struct {
	Frames           []TimeframeAnalysis `json:"frames"`
	AlignmentSummary string              `json:"alignment_summary"`
	BullishCount     int                 `json:"bullish_count"`
	BearishCount     int                 `json:"bearish_count"`
	NeutralCount     int                 `json:"neutral_count"`
}

// KeyLevel represents a specific significant reference level with distance and type.
type KeyLevel struct {
	Name        string  `json:"name"`        // e.g. "Today High", "Today Low", "Daily Open", "Asian High", "Asian Low", "PDH", "PDL", "PDC", "Weekly Open"
	Price       float64 `json:"price"`
	Distance    float64 `json:"distance"`    // price - currentPrice (signed)
	DistancePts float64 `json:"distance_pts"`// math.Abs(price - currentPrice)
	Type        string  `json:"type"`        // "Resistance", "Support", "Benchmark"
	Description string  `json:"description"`
}

// SRLevel represents an institutional Support or Resistance level.
type SRLevel struct {
	Level       string  `json:"level"`       // "R3", "R2", "R1", "Pivot", "S1", "S2", "S3"
	Price       float64 `json:"price"`
	DistancePts float64 `json:"distance_pts"`// math.Abs(price - currentPrice)
	Strength    string  `json:"strength"`    // "Major (Confluence)", "Moderate", "Minor"
	TestCount   int     `json:"test_count"`  // times tested
	Source      string  `json:"source"`      // "Classic Pivot", "Swing Cluster", "Value Area VAH/VAL"
}

// SupplyDemandZone represents an institutional supply or demand order flow zone.
type SupplyDemandZone struct {
	Type        string  `json:"type"`        // "DEMAND" or "SUPPLY"
	ZoneType    string  `json:"zone_type"`   // "Extreme / Liquidity Base", "Breaker / Order Block", "FVG Mitigation Base"
	TopPrice    float64 `json:"top_price"`
	BottomPrice float64 `json:"bottom_price"`
	MidPrice    float64 `json:"mid_price"`
	Status      string  `json:"status"`      // "FRESH (Untested)", "TESTED (Active)", "SWEPT / REJECTED"
	Strength    string  `json:"strength"`    // "Institutional Extreme", "High Confluence", "Moderate"
	DistancePts float64 `json:"distance_pts"`// Distance from currentPrice to closest edge
	Timeframe   string  `json:"timeframe"`   // e.g. "M15", "H1", "H4", "D1"
	Summary     string  `json:"summary"`
}

// InstitutionalLevelsMap holds all key market reference points, S/R, and Supply/Demand zones.
type InstitutionalLevelsMap struct {
	TodayHigh   float64            `json:"today_high"`
	TodayLow    float64            `json:"today_low"`
	TodayRange  float64            `json:"today_range"`
	DailyOpen   float64            `json:"daily_open"`
	DailyPivot  float64            `json:"daily_pivot"`
	Resistances []SRLevel          `json:"resistances"`
	Supports    []SRLevel          `json:"supports"`
	DemandZones []SupplyDemandZone `json:"demand_zones"`
	SupplyZones []SupplyDemandZone `json:"supply_zones"`
	KeyLevels   []KeyLevel         `json:"key_levels"`
}

// DeskBriefing is the full institutional executive report.
type DeskBriefing struct {
	Symbol          string                 `json:"symbol"`
	Timestamp       time.Time              `json:"timestamp"`
	CurrentPrice    float64                `json:"current_price"`
	DayChange       float64                `json:"day_change"`
	DayChangePerc   float64                `json:"day_change_perc"`
	Precision       int                    `json:"precision"`
	ExecutiveBias   string                 `json:"executive_bias"` // "STRONG BULLISH", "BULLISH", "NEUTRAL", "BEARISH"
	ConvictionScore int                    `json:"conviction_score"` // 0 to 100%
	ConvictionGrade string                 `json:"conviction_grade"` // "A+", "A", "B+", "B", "C"
	Intermarket     IntermarketRadar       `json:"intermarket"`
	MTF             MTFMatrix              `json:"mtf"`
	VolumeProfile   VolumeProfile          `json:"volume_profile"`
	Liquidity       SessionLiquidity       `json:"liquidity"`
	SMC             SMCMetrics             `json:"smc"`
	Levels          InstitutionalLevelsMap `json:"levels"`
	Playbook        TradePlaybook          `json:"playbook"`
	UpcomingEvents  []CalendarEvent        `json:"upcoming_events"`
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
		bars1M        []Bar
		bars5M        []Bar
		bars15M       []Bar
		bars1H        []Bar
		bars4H        []Bar
		barsD         []Bar
		dxyBars       []Bar
		us10yBars     []Bar
		realYieldBars []Bar
		realYieldErr  error
		spxBars       []Bar
		silverBars    []Bar
		events        []CalendarEvent
		err1M         error
		err5M         error
		err15M        error
	)

	var wg sync.WaitGroup

	// Fetch primary asset timeframes in parallel (M1, M5, M15, H1, H4, D1) — all >= 100 bars
	var err1H, err4H, errD error
	wg.Add(6)
	go func() { defer wg.Done(); bars1M, err1M = c.GetHistory(ctx, normSym, "1", 200) }()
	go func() { defer wg.Done(); bars5M, err5M = c.GetHistory(ctx, normSym, "5", 150) }()
	go func() { defer wg.Done(); bars15M, err15M = c.GetHistory(ctx, normSym, "15", 150) }()
	go func() { defer wg.Done(); bars1H, err1H = c.GetHistory(ctx, normSym, "60", 120) }()
	go func() { defer wg.Done(); bars4H, err4H = c.GetHistory(ctx, normSym, "240", 100) }()
	go func() { defer wg.Done(); barsD, errD = c.GetHistory(ctx, normSym, "D", 100) }()

	// Fetch intermarket drivers in parallel (all >= 100 bars)
	wg.Add(5)
	go func() { defer wg.Done(); dxyBars, _ = c.GetHistory(ctx, "TVC:DXY", "60", 100) }()
	go func() { defer wg.Done(); us10yBars, _ = c.GetHistory(ctx, "TVC:US10Y", "60", 100) }()
	go func() { defer wg.Done(); realYieldBars, realYieldErr = fetchTenYearRealYield(ctx, 40) }()
	go func() { defer wg.Done(); spxBars, _ = c.GetHistory(ctx, "TVC:SPX", "60", 100) }()
	go func() { defer wg.Done(); silverBars, _ = c.GetHistory(ctx, "OANDA:XAGUSD", "60", 100) }()

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
	if realYieldErr != nil {
		fmt.Fprintf(os.Stderr, "[WARN] 10Y TIPS real-yield data unavailable; nominal US10Y will be used: %v\n", realYieldErr)
	}

	if err15M != nil || len(bars15M) < 10 {
		if err1M != nil || len(bars1M) < 10 {
			return nil, fmt.Errorf("failed to fetch primary history for %s: %v", normSym, err15M)
		}
	}

	// Log and handle secondary timeframe fetch failures gracefully
	if err5M != nil {
		bars5M = nil
	}
	if err1H != nil {
		fmt.Fprintf(os.Stderr, "[WARN] H1 data fetch failed for %s: %v\n", normSym, err1H)
		bars1H = nil
	}
	if err4H != nil {
		fmt.Fprintf(os.Stderr, "[WARN] H4 data fetch failed for %s: %v\n", normSym, err4H)
		bars4H = nil
	}
	if errD != nil {
		fmt.Fprintf(os.Stderr, "[WARN] Daily data fetch failed for %s: %v\n", normSym, errD)
		barsD = nil
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
	radar := calculateIntermarketRadar(normSym, dxyBars, us10yBars, realYieldBars, spxBars, silverBars)

	// 3. Compute Multi-Timeframe Matrix (M1, M5, M15, H1, H4, D1)
	mtf := calculateMTFMatrix(bars1M, bars5M, bars15M, bars1H, bars4H, barsD, precision)

	// 4. Compute Volume Profile
	vProfile := calculateVolumeProfile(bars15M, precision)

	// 5. Compute Session Liquidity & Sweeps
	liq := calculateSessionLiquidity(bars15M, barsD, currentPrice)

	// 6. Compute SMC & Market Structure
	smc := calculateSMC(bars15M, bars1H, bars4H, currentPrice)

	// 7. Compute Institutional S/R & Supply/Demand Levels
	levels := calculateInstitutionalLevels(bars1M, bars15M, bars1H, bars4H, barsD, currentPrice, vProfile, liq, smc, precision)

	// 8. Compute ATR and Risk/Playbook
	atr15M := calculateATR(bars15M, 14)
	if atr15M <= 0 {
		atr15M = currentPrice * 0.002
	}

	// 9. Compute EMA 50 on H1 for trend filter
	ema50H1 := calculateEMA(bars1H, 50)

	// 10. Compute RSI 14 on M15 for momentum filter
	rsi14M15 := calculateRSI(bars15M, 14)

	briefing := synthesizeDeskBriefing(
		normSym, currentPrice, dayChg, dayChgPerc, precision,
		radar, mtf, vProfile, liq, smc, levels, atr15M, balance, riskPerc, events,
		ema50H1, rsi14M15,
	)

	return briefing, nil
}

func calculateIntermarketRadar(targetSym string, dxy, us10y, realYield, spx, silver []Bar) IntermarketRadar {
	var radar IntermarketRadar
	isGold := strings.Contains(targetSym, "XAU") || strings.Contains(targetSym, "GOLD")
	useRealYield := len(realYield) >= 21
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
			if useRealYield {
				impact = "Reference only (real yield used in macro score)"
			} else if pct < -0.2 {
				impact = "🟢 Bullish Tailwind (Falling Yields)"
				sentimentAccum += 0.30
			} else if pct > 0.2 {
				impact = "🔴 Bearish Headwind (Rising Yields)"
				sentimentAccum -= 0.30
			}
		}
		radar.US10Y = &MacroAsset{
			Symbol: "TVC:US10Y", Name: "US 10Y Nominal Yield", Price: last.Close,
			Change: chg, ChangePerc: pct, ImpactOnSym: impact,
		}
		if !useRealYield {
			factors += 0.30
		}
	}

	// Prefer the daily 10Y TIPS real yield as the scored rates input. Use the
	// latest 20 observations (about one trading month) and measure its move in
	// basis points; percentage changes are misleading for rates near zero.
	if useRealYield {
		last := realYield[len(realYield)-1]
		lookback := len(realYield) - 1
		if lookback > 20 {
			lookback = 20
		}
		first := realYield[len(realYield)-1-lookback]
		chg := last.Close - first.Close // yields are quoted in percentage points
		chgBps := chg * 100
		pct := 0.0
		if first.Close != 0 {
			pct = (chg / math.Abs(first.Close)) * 100
		}
		impact := "Neutral (real yields broadly stable)"
		if isGold {
			// A 10 bp monthly move is treated as a macro regime shift, not an
			// entry trigger. Smaller moves do not add directional conviction.
			if chgBps <= -10 {
				impact = "🟢 Bullish Tailwind (Falling Real Yields)"
				sentimentAccum += 0.30
			} else if chgBps >= 10 {
				impact = "🔴 Bearish Headwind (Rising Real Yields)"
				sentimentAccum -= 0.30
			}
		}
		asOf := ""
		if last.Time > 0 {
			asOf = time.Unix(last.Time, 0).UTC().Format("2006-01-02")
		}
		radar.US10YReal = &MacroAsset{
			Symbol: "FRED:DFII10", Name: "US 10Y TIPS Real Yield", Price: last.Close,
			Change: chg, ChangePerc: pct, ChangeBps: chgBps,
			Lookback: fmt.Sprintf("%d daily observations", lookback), AsOf: asOf, ImpactOnSym: impact,
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
	if useRealYield {
		radar.RatesSignalSource = "10Y TIPS real yield (20 daily observations)"
	} else if len(us10y) >= 2 {
		radar.RatesSignalSource = "Nominal US10Y fallback (TIPS history unavailable or insufficient)"
	} else {
		radar.RatesSignalSource = "Unavailable (insufficient yield history)"
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

	// Determine current trading day from the latest bar
	var currentDay int
	if len(bars15M) > 0 {
		currentDay = time.Unix(bars15M[len(bars15M)-1].Time, 0).UTC().YearDay()
	}

	// Calculate Asian Session (00:00 to 08:00 UTC of current day)
	var asiaHigh, asiaLow float64
	hasAsia := false

	for _, b := range bars15M {
		t := time.Unix(b.Time, 0).UTC()
		if t.YearDay() == currentDay && t.Hour() >= 0 && t.Hour() < 8 {
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

	// Detect Asia Liquidity Sweeps (only on current day after Asia session closes at 08:00 UTC)
	for _, b := range bars15M {
		t := time.Unix(b.Time, 0).UTC()
		if t.YearDay() == currentDay && t.Hour() >= 8 && hasAsia {
			if b.Low < asiaLow && b.Close > asiaLow {
				liq.AsiaSweptLow = true
			}
			if b.High > asiaHigh && b.Close < asiaHigh {
				liq.AsiaSweptHigh = true
			}
		}
	}

	// Calculate London Session (08:00 to 16:00 UTC)
	var londonHigh, londonLow float64
	hasLondon := false
	for _, b := range bars15M {
		t := time.Unix(b.Time, 0).UTC()
		if t.Hour() >= 8 && t.Hour() < 16 {
			if !hasLondon {
				londonHigh = b.High
				londonLow = b.Low
				hasLondon = true
			} else {
				if b.High > londonHigh {
					londonHigh = b.High
				}
				if b.Low < londonLow {
					londonLow = b.Low
				}
			}
		}
	}
	liq.LondonHigh = londonHigh
	liq.LondonLow = londonLow

	return liq
}

func calculateSMC(bars15M, bars1H, bars4H []Bar, currentPrice float64) SMCMetrics {
	var smc SMCMetrics

	if len(bars15M) < 10 {
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

	// 2. FVG (Fair Value Gap) Detection on M15
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

	// 3. Order Block (OB) Detection
	// Bullish OB: Last bearish candle before a strong bullish impulse (3+ candle move up)
	// Bearish OB: Last bullish candle before a strong bearish impulse (3+ candle move down)
	for i := 1; i < len(bars15M)-2; i++ {
		isBearishCandle := bars15M[i].Close < bars15M[i].Open
		isBullishCandle := bars15M[i].Close > bars15M[i].Open

		// Check for bullish impulse after a bearish candle
		if isBearishCandle {
			impulseUp := bars15M[i+1].Close > bars15M[i].High && bars15M[i+2].Close > bars15M[i+1].High
			if impulseUp {
				smc.BullishOB = fmt.Sprintf("%.2f - %.2f", bars15M[i].Low, bars15M[i].High)
			}
		}

		// Check for bearish impulse after a bullish candle
		if isBullishCandle {
			impulseDown := bars15M[i+1].Close < bars15M[i].Low && bars15M[i+2].Close < bars15M[i+1].Low
			if impulseDown {
				smc.BearishOB = fmt.Sprintf("%.2f - %.2f", bars15M[i].Low, bars15M[i].High)
			}
		}
	}

	// 4. Swing Point Detection for BOS & ChoCH
	// Detect swing highs and swing lows using a 3-bar pivot method
	type SwingPoint struct {
		Price     float64
		Index     int
		IsHigh    bool
	}
	var swings []SwingPoint

	for i := 2; i < len(bars15M)-2; i++ {
		// Swing High: bar[i].High > both neighbors' highs
		if bars15M[i].High > bars15M[i-1].High && bars15M[i].High > bars15M[i-2].High &&
			bars15M[i].High > bars15M[i+1].High && bars15M[i].High > bars15M[i+2].High {
			swings = append(swings, SwingPoint{Price: bars15M[i].High, Index: i, IsHigh: true})
		}
		// Swing Low: bar[i].Low < both neighbors' lows
		if bars15M[i].Low < bars15M[i-1].Low && bars15M[i].Low < bars15M[i-2].Low &&
			bars15M[i].Low < bars15M[i+1].Low && bars15M[i].Low < bars15M[i+2].Low {
			swings = append(swings, SwingPoint{Price: bars15M[i].Low, Index: i, IsHigh: false})
		}
	}

	// 5. BOS & ChoCH Detection from Swing Points
	// BOS = Break of Structure (trend continuation):
	//   Bullish BOS: current price breaks above a recent swing high (Higher High)
	//   Bearish BOS: current price breaks below a recent swing low (Lower Low)
	// ChoCH = Change of Character (trend reversal):
	//   After a series of HH/HL, price breaks below the last swing low → Bearish ChoCH
	//   After a series of LL/LH, price breaks above the last swing high → Bullish ChoCH
	smc.BOSDirection = "None"
	smc.ChoCHDirection = ""

	if len(swings) >= 4 {
		// Determine prevailing trend from last 4 swing points
		var lastSwingHighs []SwingPoint
		var lastSwingLows []SwingPoint
		for _, s := range swings {
			if s.IsHigh {
				lastSwingHighs = append(lastSwingHighs, s)
			} else {
				lastSwingLows = append(lastSwingLows, s)
			}
		}

		// Determine if prevailing trend was bullish (HH + HL) or bearish (LH + LL)
		prevailingBull := false
		prevailingBear := false
		if len(lastSwingHighs) >= 2 && len(lastSwingLows) >= 2 {
			h1 := lastSwingHighs[len(lastSwingHighs)-2]
			h2 := lastSwingHighs[len(lastSwingHighs)-1]
			l1 := lastSwingLows[len(lastSwingLows)-2]
			l2 := lastSwingLows[len(lastSwingLows)-1]

			if h2.Price > h1.Price && l2.Price > l1.Price {
				prevailingBull = true // Higher Highs + Higher Lows
			} else if h2.Price < h1.Price && l2.Price < l1.Price {
				prevailingBear = true // Lower Highs + Lower Lows
			}
		}

		// Populate Structure Sequence (HH, HL, LH, LL)
		if len(lastSwingHighs) >= 1 {
			lastH := lastSwingHighs[len(lastSwingHighs)-1]
			hType := "LH"
			hLabel := "Lower High"
			if len(lastSwingHighs) >= 2 {
				prevH := lastSwingHighs[len(lastSwingHighs)-2]
				if lastH.Price >= prevH.Price {
					hType = "HH"
					hLabel = "Higher High"
				}
				smc.StructureSeq.PrevHigh = StructureSwing{
					Type:        "SH",
					Label:       "Prior High",
					Price:       prevH.Price,
					TimeStr:     time.Unix(bars15M[prevH.Index].Time, 0).UTC().Format("15:04"),
					DistancePts: math.Abs(prevH.Price - currentPrice),
				}
			}
			smc.StructureSeq.LastHigh = StructureSwing{
				Type:        hType,
				Label:       hLabel,
				Price:       lastH.Price,
				TimeStr:     time.Unix(bars15M[lastH.Index].Time, 0).UTC().Format("15:04"),
				DistancePts: math.Abs(lastH.Price - currentPrice),
			}
		}

		if len(lastSwingLows) >= 1 {
			lastL := lastSwingLows[len(lastSwingLows)-1]
			lType := "LL"
			lLabel := "Lower Low"
			if len(lastSwingLows) >= 2 {
				prevL := lastSwingLows[len(lastSwingLows)-2]
				if lastL.Price >= prevL.Price {
					lType = "HL"
					lLabel = "Higher Low"
				}
				smc.StructureSeq.PrevLow = StructureSwing{
					Type:        "SL",
					Label:       "Prior Low",
					Price:       prevL.Price,
					TimeStr:     time.Unix(bars15M[prevL.Index].Time, 0).UTC().Format("15:04"),
					DistancePts: math.Abs(currentPrice - prevL.Price),
				}
			}
			smc.StructureSeq.LastLow = StructureSwing{
				Type:        lType,
				Label:       lLabel,
				Price:       lastL.Price,
				TimeStr:     time.Unix(bars15M[lastL.Index].Time, 0).UTC().Format("15:04"),
				DistancePts: math.Abs(currentPrice - lastL.Price),
			}
		}

		if smc.StructureSeq.LastHigh.Type == "HH" && smc.StructureSeq.LastLow.Type == "HL" {
			smc.StructureSeq.TrendBias = "🟢 Bullish Structure (Higher Highs + Higher Lows)"
		} else if smc.StructureSeq.LastHigh.Type == "LH" && smc.StructureSeq.LastLow.Type == "LL" {
			smc.StructureSeq.TrendBias = "🔴 Bearish Structure (Lower Highs + Lower Lows)"
		} else if smc.StructureSeq.LastHigh.Type == "LH" && smc.StructureSeq.LastLow.Type == "HL" {
			smc.StructureSeq.TrendBias = "🟡 Contracting Symmetrical Range (LH + HL)"
		} else {
			smc.StructureSeq.TrendBias = "🟡 Expanding / Transition Structure"
		}

		if smc.StructureSeq.LastHigh.Price > 0 && smc.StructureSeq.LastLow.Price > 0 {
			smc.StructureSeq.SequenceFlow = fmt.Sprintf("%s (%.2f) ➔ %s (%.2f)",
				smc.StructureSeq.LastHigh.Type, smc.StructureSeq.LastHigh.Price,
				smc.StructureSeq.LastLow.Type, smc.StructureSeq.LastLow.Price)
		}

		// Check BOS: does current price break last swing high (bullish) or low (bearish)?
		if len(lastSwingHighs) > 0 {
			lastSH := lastSwingHighs[len(lastSwingHighs)-1]
			if currentPrice > lastSH.Price {
				if prevailingBull {
					// Continuation → BOS
					smc.BOSDirection = "🟢 Bullish BOS (Break of Structure)"
					smc.BOSLevel = lastSH.Price
				} else if prevailingBear {
					// Reversal → ChoCH
					smc.ChoCHDetected = true
					smc.ChoCHDirection = "🟢 Bullish ChoCH (Change of Character)"
					smc.ChoCHLevel = lastSH.Price
				} else {
					smc.BOSDirection = "🟢 Bullish BOS (Break of Structure)"
					smc.BOSLevel = lastSH.Price
				}
			}
		}
		if len(lastSwingLows) > 0 {
			lastSL := lastSwingLows[len(lastSwingLows)-1]
			if currentPrice < lastSL.Price {
				if prevailingBear {
					// Continuation → BOS
					smc.BOSDirection = "🔴 Bearish BOS (Break of Structure)"
					smc.BOSLevel = lastSL.Price
				} else if prevailingBull {
					// Reversal → ChoCH
					smc.ChoCHDetected = true
					smc.ChoCHDirection = "🔴 Bearish ChoCH (Change of Character)"
					smc.ChoCHLevel = lastSL.Price
				} else {
					smc.BOSDirection = "🔴 Bearish BOS (Break of Structure)"
					smc.BOSLevel = lastSL.Price
				}
			}
		}
	}

	// 6. Market Structure & CISD Bias using Swing High/Low Breaks
	lookback := 5
	if lookback > len(bars15M)-2 {
		lookback = len(bars15M) - 2
	}

	recentSwingHigh := bars15M[len(bars15M)-1].High
	recentSwingLow := bars15M[len(bars15M)-1].Low
	for i := len(bars15M) - lookback; i < len(bars15M); i++ {
		if bars15M[i].High > recentSwingHigh {
			recentSwingHigh = bars15M[i].High
		}
		if bars15M[i].Low < recentSwingLow {
			recentSwingLow = bars15M[i].Low
		}
	}

	// Check H1 structure direction
	is1HBull := false
	is1HBear := false
	if len(bars1H) >= 3 {
		h1Last := bars1H[len(bars1H)-1]
		h1Prev := bars1H[len(bars1H)-2]
		if h1Last.Close > h1Prev.High {
			is1HBull = true
		} else if h1Last.Close < h1Prev.Low {
			is1HBear = true
		}
	}

	// Check 4H trend direction
	is4HBull := false
	is4HBear := false
	if len(bars4H) >= 2 {
		if bars4H[len(bars4H)-1].Close >= bars4H[0].Open {
			is4HBull = true
		} else {
			is4HBear = true
		}
	}

	// Determine MSS: price breaking above recent swing high = bullish, below swing low = bearish
	last := bars15M[len(bars15M)-1]
	if last.Close > recentSwingHigh && recentSwingHigh > recentSwingLow {
		smc.MarketStructure = "🟢 Bullish MSS (Market Structure Shift)"
		smc.CISDBias = "Bullish"
	} else if last.Close < recentSwingLow && recentSwingLow < recentSwingHigh {
		smc.MarketStructure = "🔴 Bearish MSS (Market Structure Shift)"
		smc.CISDBias = "Bearish"
	} else if is1HBull || is4HBull {
		smc.MarketStructure = "🟡 Range Bound / Accumulation Base (HTF Bullish)"
		smc.CISDBias = "Neutral-Bullish"
	} else if is1HBear || is4HBear {
		smc.MarketStructure = "🟡 Range Bound / Distribution (HTF Bearish)"
		smc.CISDBias = "Neutral-Bearish"
	} else {
		smc.MarketStructure = "🟡 Range Bound / No Clear Structure"
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

// calculateEMA computes the Exponential Moving Average for the given period using Close prices.
func calculateEMA(bars []Bar, period int) float64 {
	if len(bars) < period || period <= 0 {
		return 0
	}
	// Seed EMA with SMA of first 'period' bars
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += bars[i].Close
	}
	ema := sum / float64(period)
	multiplier := 2.0 / float64(period+1)
	for i := period; i < len(bars); i++ {
		ema = (bars[i].Close-ema)*multiplier + ema
	}
	return ema
}

// calculateRSI computes the Relative Strength Index for the given period.
func calculateRSI(bars []Bar, period int) float64 {
	if len(bars) < period+1 || period <= 0 {
		return 50 // neutral default
	}
	avgGain := 0.0
	avgLoss := 0.0
	// Initial average gain/loss
	for i := 1; i <= period; i++ {
		change := bars[i].Close - bars[i-1].Close
		if change > 0 {
			avgGain += change
		} else {
			avgLoss += math.Abs(change)
		}
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)

	// Smoothed RSI (Wilder's method)
	for i := period + 1; i < len(bars); i++ {
		change := bars[i].Close - bars[i-1].Close
		if change > 0 {
			avgGain = (avgGain*float64(period-1) + change) / float64(period)
			avgLoss = (avgLoss * float64(period-1)) / float64(period)
		} else {
			avgGain = (avgGain * float64(period-1)) / float64(period)
			avgLoss = (avgLoss*float64(period-1) + math.Abs(change)) / float64(period)
		}
	}

	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - (100 / (1 + rs))
}

func synthesizeDeskBriefing(
	symbol string, currentPrice, dayChg, dayChgPerc float64, precision int,
	radar IntermarketRadar, mtf MTFMatrix, vp VolumeProfile, liq SessionLiquidity, smc SMCMetrics,
	levels InstitutionalLevelsMap,
	atr15M float64, balance, riskPerc float64, events []CalendarEvent,
	ema50H1 float64, rsi14M15 float64,
) *DeskBriefing {
	// Calculate Quantitative Conviction Score (0 - 100)
	// Score > 50 = Bullish bias, Score < 50 = Bearish bias
	// Distance from 50 = strength of conviction
	score := 50 // baseline (neutral)

	// --- MACRO CONTRIBUTION (+/- 15) ---
	score += int(radar.SentimentScore * 15)

	// --- MTF CONFLUENCE CONTRIBUTION (+/- 12) ---
	mtfNet := mtf.BullishCount - mtf.BearishCount
	score += int(float64(mtfNet) * 2.0)

	// --- VOLUME PROFILE CONTRIBUTION (+/- 10) ---
	if vp.POC > 0 && vp.VAL > 0 && vp.VAH > 0 {
		if currentPrice >= vp.VAL && currentPrice <= vp.POC {
			score += 10 // Long: in value discount, mean reversion up likely
		} else if currentPrice > vp.POC && currentPrice <= vp.VAH {
			score -= 5 // Slight bearish: above POC, may revert down
		} else if currentPrice > vp.VAH {
			score -= 10 // Bearish: overextended above VAH, short favorable
		} else if currentPrice < vp.VAL {
			score += 5 // Bullish if just below VAL (deep discount), but risky
		}
	}

	// --- LIQUIDITY SWEEP CONTRIBUTION (+/- 15) ---
	if liq.AsiaSweptLow {
		score += 15 // Bullish: sell-side liquidity grabbed, reversal up likely
	}
	if liq.AsiaSweptHigh {
		score -= 15 // Bearish: buy-side liquidity grabbed, reversal down likely
	}

	// --- SMC PRICING ZONE (+/- 10) ---
	if strings.Contains(smc.PricingZone, "DISCOUNT") {
		score += 10 // Bullish: price in discount zone
	} else if strings.Contains(smc.PricingZone, "PREMIUM") {
		score -= 10 // Bearish: price in premium zone
	}

	// --- MARKET STRUCTURE / CISD BIAS (+/- 8) ---
	switch smc.CISDBias {
	case "Bullish":
		score += 8
	case "Neutral-Bullish":
		score += 4
	case "Bearish":
		score -= 8
	case "Neutral-Bearish":
		score -= 4
	}

	// --- EMA TREND FILTER (+/- 5) ---
	if ema50H1 > 0 {
		if currentPrice > ema50H1 {
			score += 5 // Bullish: price above H1 EMA 50
		} else {
			score -= 5 // Bearish: price below H1 EMA 50
		}
	}

	// --- RSI MOMENTUM (+/- 5) ---
	if rsi14M15 > 0 {
		if rsi14M15 < 30 {
			score += 5 // Oversold bounce potential (bullish)
		} else if rsi14M15 > 70 {
			score -= 5 // Overbought reversal potential (bearish)
		}
	}

	// Clamp score
	if score > 95 {
		score = 95
	}
	if score < 5 {
		score = 5
	}

	// Determine Conviction Grade & Bias Direction
	var grade string
	var bias string
	isBullish := score >= 50

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
	} else if score >= 35 {
		grade = "B-"
		bias = "MODERATE BEARISH"
	} else if score >= 25 {
		grade = "A"
		bias = "BEARISH"
	} else {
		grade = "A+"
		bias = "STRONG BEARISH"
	}

	// Risk Calculation & Institutional Playbook (BI-DIRECTIONAL)
	riskUSD := balance * (riskPerc / 100.0)

	contractMult := 100.0
	if !strings.Contains(symbol, "XAU") && !strings.Contains(symbol, "GOLD") {
		contractMult = 100000.0 // Standard Forex
	}

	var playbook TradePlaybook

	if isBullish {
		// === LONG PLAYBOOK ===
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

		// Target 1: POC or Value Mean Reversion
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

		lots := riskUSD / (slDistance * contractMult)
		if lots < 0.01 {
			lots = 0.01
		}

		winRate := float64(score) / 100.0
		rewardUSD := riskUSD * rrRatio
		evUSD := (winRate * rewardUSD) - ((1.0 - winRate) * riskUSD)

		playbook = TradePlaybook{
			PrimaryPlan:       "LONG PULLBACK / DISCOUNT ACCUMULATION",
			OptimalEntryZone:  fmt.Sprintf("%.2f – %.2f", vp.VAL, currentPrice),
			EntryPrice:        currentPrice,
			InvalidationPrice: slPrice,
			Target1Price:      tp1Price,
			Target2Price:      tp2Price,
			RiskRewardRatio:   rrRatio,
			EstimatedWinRate:  math.Round(winRate*1000) / 10,
			RecommendedLots:   math.Round(lots*100) / 100,
			RiskAmountUSD:     riskUSD,
			ExpectedValueUSD:  evUSD,
			ThesisSummary:     fmt.Sprintf("Akumulasi Long di zona Discount (%s) setelah Liquidity Sweep, menargetkan POC & Day High dengan RRR 1:%.2f.", FormatDeskPrice(vp.VAL, precision), rrRatio),
		}
	} else {
		// === SHORT PLAYBOOK ===
		// Dynamic Stop Loss based on VAH and ATR buffer
		slPrice := currentPrice + (atr15M * 1.5) // ATR fallback default
		if vp.VAH > 0 {
			slPrice = vp.VAH + (atr15M * 0.8)
		}
		if liq.AsiaHigh > 0 && liq.AsiaHigh > currentPrice {
			slPrice = math.Max(slPrice, liq.AsiaHigh+(atr15M*0.5))
		}
		if slPrice <= currentPrice {
			slPrice = currentPrice + (atr15M * 1.5)
		}

		slDistance := slPrice - currentPrice
		if slDistance <= 0 {
			slDistance = atr15M * 1.5
			slPrice = currentPrice + slDistance
		}

		// Target 1: POC or Value Mean Reversion (downward)
		tp1Price := currentPrice - (slDistance * 1.8) // ATR fallback default
		if vp.POC > 0 && vp.POC < currentPrice {
			tp1Price = math.Min(vp.POC, currentPrice-(slDistance*1.5))
		}
		if tp1Price >= currentPrice {
			tp1Price = currentPrice - (slDistance * 1.8)
		}

		// Target 2: PDL / Day Low / Liquidity Pool
		tp2Price := liq.PDL
		if tp2Price <= 0 || tp2Price >= tp1Price {
			tp2Price = tp1Price - slDistance
		}
		if tp2Price >= tp1Price {
			tp2Price = tp1Price - (slDistance * 1.5)
		}

		rrRatio := (currentPrice - tp2Price) / slDistance
		if rrRatio < 1.5 {
			rrRatio = 2.0
		}

		lots := riskUSD / (slDistance * contractMult)
		if lots < 0.01 {
			lots = 0.01
		}

		// For SHORT, invert score for win rate (score 30 -> bearish conviction 70%)
		bearishConviction := float64(100-score) / 100.0
		rewardUSD := riskUSD * rrRatio
		evUSD := (bearishConviction * rewardUSD) - ((1.0 - bearishConviction) * riskUSD)

		playbook = TradePlaybook{
			PrimaryPlan:       "SHORT BREAKDOWN / PREMIUM DISTRIBUTION",
			OptimalEntryZone:  fmt.Sprintf("%.2f – %.2f", currentPrice, vp.VAH),
			EntryPrice:        currentPrice,
			InvalidationPrice: slPrice,
			Target1Price:      tp1Price,
			Target2Price:      tp2Price,
			RiskRewardRatio:   rrRatio,
			EstimatedWinRate:  math.Round(bearishConviction*1000) / 10,
			RecommendedLots:   math.Round(lots*100) / 100,
			RiskAmountUSD:     riskUSD,
			ExpectedValueUSD:  evUSD,
			ThesisSummary:     fmt.Sprintf("Short Distribution dari zona Premium (%s) setelah Liquidity Sweep, menargetkan POC & Day Low dengan RRR 1:%.2f.", FormatDeskPrice(vp.VAH, precision), rrRatio),
		}
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
		MTF:             mtf,
		VolumeProfile:   vp,
		Liquidity:       liq,
		SMC:             smc,
		Levels:          levels,
		Playbook:        playbook,
		UpcomingEvents:  upcoming,
	}
}

func calculateInstitutionalLevels(
	bars1M, bars15M, bars1H, bars4H, barsD []Bar,
	currentPrice float64, vp VolumeProfile, liq SessionLiquidity, smc SMCMetrics, precision int,
) InstitutionalLevelsMap {
	var lm InstitutionalLevelsMap

	// 1. Calculate Today High & Low
	todayHigh := currentPrice
	todayLow := currentPrice
	var currentDay int
	if len(bars15M) > 0 {
		currentDay = time.Unix(bars15M[len(bars15M)-1].Time, 0).UTC().YearDay()
	} else if len(bars1H) > 0 {
		currentDay = time.Unix(bars1H[len(bars1H)-1].Time, 0).UTC().YearDay()
	}

	foundToday := false
	for _, b := range bars15M {
		t := time.Unix(b.Time, 0).UTC()
		if t.YearDay() == currentDay {
			if !foundToday {
				todayHigh = b.High
				todayLow = b.Low
				foundToday = true
			} else {
				if b.High > todayHigh {
					todayHigh = b.High
				}
				if b.Low < todayLow {
					todayLow = b.Low
				}
			}
		}
	}
	if !foundToday && len(barsD) > 0 {
		todayHigh = barsD[len(barsD)-1].High
		todayLow = barsD[len(barsD)-1].Low
	}
	if todayHigh < currentPrice {
		todayHigh = currentPrice
	}
	if todayLow > currentPrice && currentPrice > 0 {
		todayLow = currentPrice
	}

	lm.TodayHigh = todayHigh
	lm.TodayLow = todayLow
	lm.TodayRange = todayHigh - todayLow
	lm.DailyOpen = liq.DailyOpen

	// 2. Standard Pivot Points (Classic Pivot Calculation)
	pdh := liq.PDH
	pdl := liq.PDL
	pdc := liq.PDC
	if pdh <= 0 || pdl <= 0 || pdc <= 0 {
		pdh = todayHigh
		pdl = todayLow
		pdc = currentPrice
	}

	pivot := (pdh + pdl + pdc) / 3.0
	r1 := (2.0 * pivot) - pdl
	s1 := (2.0 * pivot) - pdh
	r2 := pivot + (pdh - pdl)
	s2 := pivot - (pdh - pdl)
	r3 := pdh + 2.0*(pivot - pdl)
	s3 := pdl - 2.0*(pdh - pivot)

	lm.DailyPivot = pivot

	// 3. Count tests of levels across H1 and 15M bars
	countTests := func(targetPrice float64, tolerance float64) int {
		tests := 0
		for _, b := range bars15M {
			if b.High >= targetPrice-tolerance && b.Low <= targetPrice+tolerance {
				tests++
			}
		}
		return tests
	}
	tol := math.Max(0.5, currentPrice*0.0003)

	// Build Resistances (sorted ascending)
	lm.Resistances = []SRLevel{
		{
			Level:       "R1 (Pivot)",
			Price:       r1,
			DistancePts: math.Abs(r1 - currentPrice),
			Strength:    "Moderate",
			TestCount:   countTests(r1, tol),
			Source:      "Daily Classic Pivot",
		},
		{
			Level:       "R2 (Breakout Range)",
			Price:       r2,
			DistancePts: math.Abs(r2 - currentPrice),
			Strength:    "Major (Expansion)",
			TestCount:   countTests(r2, tol),
			Source:      "Daily Pivot Range R2",
		},
		{
			Level:       "R3 (Extreme Ceiling)",
			Price:       r3,
			DistancePts: math.Abs(r3 - currentPrice),
			Strength:    "Extreme Ceiling",
			TestCount:   countTests(r3, tol),
			Source:      "Daily Pivot R3 / Volatility Extension",
		},
	}

	// Build Supports (sorted descending)
	lm.Supports = []SRLevel{
		{
			Level:       "S1 (Pivot)",
			Price:       s1,
			DistancePts: math.Abs(currentPrice - s1),
			Strength:    "Moderate",
			TestCount:   countTests(s1, tol),
			Source:      "Daily Classic Pivot",
		},
		{
			Level:       "S2 (Demand Floor)",
			Price:       s2,
			DistancePts: math.Abs(currentPrice - s2),
			Strength:    "Major (Expansion Floor)",
			TestCount:   countTests(s2, tol),
			Source:      "Daily Pivot Range S2",
		},
		{
			Level:       "S3 (Deep Invalidation)",
			Price:       s3,
			DistancePts: math.Abs(currentPrice - s3),
			Strength:    "Extreme Floor",
			TestCount:   countTests(s3, tol),
			Source:      "Daily Pivot S3 / Volatility Extension",
		},
	}

	// 4. Build Supply & Demand Zones
	// Demand Zones (Buyers Accumulation Bases)
	// Zone 1: Rebound Base / VAL / Bullish OB
	valDemandBottom := vp.VAL - (tol * 1.5)
	valDemandTop := vp.VAL + (tol * 1.5)
	if valDemandTop < valDemandBottom {
		valDemandTop, valDemandBottom = valDemandBottom, valDemandTop
	}
	statusDemand1 := "FRESH (Untested)"
	if currentPrice >= valDemandBottom && currentPrice <= valDemandTop {
		statusDemand1 = "TESTED (Active in Zone)"
	} else if currentPrice > valDemandTop {
		statusDemand1 = "TESTED (Holding as Support)"
	}

	distDemand1 := 0.0
	if currentPrice > valDemandTop {
		distDemand1 = currentPrice - valDemandTop
	} else if currentPrice < valDemandBottom {
		distDemand1 = valDemandBottom - currentPrice
	}

	d1 := SupplyDemandZone{
		Type:        "DEMAND",
		ZoneType:    "Value Area Base (VAL / Bullish OB)",
		TopPrice:    valDemandTop,
		BottomPrice: valDemandBottom,
		MidPrice:    (valDemandTop + valDemandBottom) / 2.0,
		Status:      statusDemand1,
		Strength:    "High (Value Reversion Base)",
		DistancePts: distDemand1,
		Timeframe:   "M15 / H1",
		Summary:     fmt.Sprintf("%.*f - %.*f (VAL Rebound Confluence)", precision, valDemandBottom, precision, valDemandTop),
	}

	// Zone 2: Extreme Demand / Asian Low & Today Low Sweep Floor
	asiaLowFloor := math.Min(todayLow, liq.AsiaLow)
	if asiaLowFloor <= 0 {
		asiaLowFloor = todayLow
	}
	extremeDemandBottom := asiaLowFloor - (tol * 2.0)
	extremeDemandTop := asiaLowFloor + (tol * 1.0)
	statusDemand2 := "TESTED (Active Reversal Base)"
	if currentPrice < extremeDemandBottom {
		statusDemand2 = "BROKEN (Sell-Side Expansion)"
	} else if currentPrice > extremeDemandTop {
		statusDemand2 = "TESTED (Liquidity Swept)"
	}

	distDemand2 := 0.0
	if currentPrice > extremeDemandTop {
		distDemand2 = currentPrice - extremeDemandTop
	} else if currentPrice < extremeDemandBottom {
		distDemand2 = extremeDemandBottom - currentPrice
	}

	d2 := SupplyDemandZone{
		Type:        "DEMAND",
		ZoneType:    "Extreme Liquidity Pool (Asia / Today Low)",
		TopPrice:    extremeDemandTop,
		BottomPrice: extremeDemandBottom,
		MidPrice:    (extremeDemandTop + extremeDemandBottom) / 2.0,
		Status:      statusDemand2,
		Strength:    "Institutional Extreme",
		DistancePts: distDemand2,
		Timeframe:   "H1 / H4",
		Summary:     fmt.Sprintf("%.*f - %.*f (Asia Low Rejection Floor)", precision, extremeDemandBottom, precision, extremeDemandTop),
	}

	lm.DemandZones = []SupplyDemandZone{d1, d2}

	// Supply Zones (Sellers Distribution Bases)
	// Supply Zone 1: POC / Unfilled FVG Resistance
	pocSupplyMid := vp.POC
	if smc.UnfilledFVGPrice > 0 {
		pocSupplyMid = smc.UnfilledFVGPrice
	}
	pocSupplyBottom := pocSupplyMid - (tol * 1.5)
	pocSupplyTop := pocSupplyMid + (tol * 1.5)
	statusSupply1 := "FRESH (Untested Ceiling)"
	if currentPrice >= pocSupplyBottom && currentPrice <= pocSupplyTop {
		statusSupply1 = "TESTED (Active in Zone)"
	} else if currentPrice > pocSupplyTop {
		statusSupply1 = "SWEPT (Reclaimed)"
	}

	distSupply1 := 0.0
	if currentPrice < pocSupplyBottom {
		distSupply1 = pocSupplyBottom - currentPrice
	} else if currentPrice > pocSupplyTop {
		distSupply1 = currentPrice - pocSupplyTop
	}

	sZone1 := SupplyDemandZone{
		Type:        "SUPPLY",
		ZoneType:    "Point of Control / FVG Magnet",
		TopPrice:    pocSupplyTop,
		BottomPrice: pocSupplyBottom,
		MidPrice:    pocSupplyMid,
		Status:      statusSupply1,
		Strength:    "High (Volume Magnet)",
		DistancePts: distSupply1,
		Timeframe:   "M15 / H1",
		Summary:     fmt.Sprintf("%.*f - %.*f (POC / FVG Resistance)", precision, pocSupplyBottom, precision, pocSupplyTop),
	}

	// Supply Zone 2: Extreme Distribution / VAH / Asian High Ceiling
	vahSupplyMid := math.Max(vp.VAH, liq.AsiaHigh)
	if vahSupplyMid <= 0 {
		vahSupplyMid = todayHigh
	}
	vahSupplyBottom := vahSupplyMid - (tol * 2.0)
	vahSupplyTop := vahSupplyMid + (tol * 2.0)
	statusSupply2 := "FRESH (Hard Overhead Resistance)"
	if currentPrice >= vahSupplyBottom && currentPrice <= vahSupplyTop {
		statusSupply2 = "TESTED (Active in Zone)"
	} else if currentPrice > vahSupplyTop {
		statusSupply2 = "BREAKOUT (Bullish Expansion)"
	}

	distSupply2 := 0.0
	if currentPrice < vahSupplyBottom {
		distSupply2 = vahSupplyBottom - currentPrice
	} else if currentPrice > vahSupplyTop {
		distSupply2 = currentPrice - vahSupplyTop
	}

	sZone2 := SupplyDemandZone{
		Type:        "SUPPLY",
		ZoneType:    "Value Area High / Asian High Distribution",
		TopPrice:    vahSupplyTop,
		BottomPrice: vahSupplyBottom,
		MidPrice:    vahSupplyMid,
		Status:      statusSupply2,
		Strength:    "Institutional Extreme",
		DistancePts: distSupply2,
		Timeframe:   "H1 / H4",
		Summary:     fmt.Sprintf("%.*f - %.*f (VAH / Asia High Hard Ceiling)", precision, vahSupplyBottom, precision, vahSupplyTop),
	}

	lm.SupplyZones = []SupplyDemandZone{sZone1, sZone2}

	// 5. Key Levels Summary List
	makeKeyLevel := func(name string, price float64, lvlType, desc string) KeyLevel {
		return KeyLevel{
			Name:        name,
			Price:       price,
			Distance:    price - currentPrice,
			DistancePts: math.Abs(price - currentPrice),
			Type:        lvlType,
			Description: desc,
		}
	}

	var kList []KeyLevel
	if todayHigh > 0 {
		kList = append(kList, makeKeyLevel("Today High", todayHigh, "Resistance", "High of Day (Intraday Peak)"))
	}
	if liq.AsiaHigh > 0 {
		kList = append(kList, makeKeyLevel("Asian High", liq.AsiaHigh, "Resistance", "Asian Session Peak (00:00 - 08:00 UTC)"))
	}
	if liq.PDH > 0 {
		kList = append(kList, makeKeyLevel("Previous Day High (PDH)", liq.PDH, "Resistance", "Prior Day High Liquidity Pool"))
	}
	if vp.VAH > 0 {
		kList = append(kList, makeKeyLevel("Value Area High (VAH)", vp.VAH, "Resistance", "Upper 70% Volume Boundary"))
	}
	if vp.POC > 0 {
		kList = append(kList, makeKeyLevel("Point of Control (POC)", vp.POC, "Benchmark", "Highest Traded Volume Level"))
	}
	if liq.DailyOpen > 0 {
		kList = append(kList, makeKeyLevel("Daily Open", liq.DailyOpen, "Benchmark", "Current Trading Day Open Price"))
	}
	if pivot > 0 {
		kList = append(kList, makeKeyLevel("Daily Pivot", pivot, "Benchmark", "Classic Central Pivot Point"))
	}
	if vp.VAL > 0 {
		kList = append(kList, makeKeyLevel("Value Area Low (VAL)", vp.VAL, "Support", "Lower 70% Volume Boundary"))
	}
	if liq.AsiaLow > 0 {
		kList = append(kList, makeKeyLevel("Asian Low", liq.AsiaLow, "Support", "Asian Session Floor (00:00 - 08:00 UTC)"))
	}
	if todayLow > 0 {
		kList = append(kList, makeKeyLevel("Today Low", todayLow, "Support", "Low of Day (Intraday Floor)"))
	}
	if liq.PDL > 0 {
		kList = append(kList, makeKeyLevel("Previous Day Low (PDL)", liq.PDL, "Support", "Prior Day Low Liquidity Pool"))
	}

	lm.KeyLevels = kList

	return lm
}

func calculateTimeframeAnalysis(tf string, bars []Bar, precision int) TimeframeAnalysis {
	if len(bars) < 2 {
		return TimeframeAnalysis{
			TF:        tf,
			Bias:      "NEUTRAL",
			Structure: "Insufficient Data",
			RSI:       50,
			EMA21:     0,
			Candle:    "⚪ N/A",
		}
	}

	lastBar := bars[len(bars)-1]
	diff := lastBar.Close - lastBar.Open
	candleStr := fmt.Sprintf("⚪ Doji (%.*f)", precision, diff)
	if diff > 0 {
		candleStr = fmt.Sprintf("🟢 Bull (+%.*f)", precision, diff)
	} else if diff < 0 {
		candleStr = fmt.Sprintf("🔴 Bear (%.*f)", precision, diff)
	}

	rsi := calculateRSI(bars, 14)
	ema21 := calculateEMA(bars, 21)
	if ema21 <= 0 {
		ema21 = lastBar.Close
	}

	// Trend & Structure determination
	bias := "NEUTRAL"
	structure := "Consolidation / Range"

	lookback := 5
	if lookback > len(bars)-1 {
		lookback = len(bars) - 1
	}
	prevHigh := bars[len(bars)-1].High
	prevLow := bars[len(bars)-1].Low
	if len(bars) >= 10 {
		for i := len(bars) - lookback; i < len(bars)-1; i++ {
			if bars[i].High > prevHigh {
				prevHigh = bars[i].High
			}
			if bars[i].Low < prevLow {
				prevLow = bars[i].Low
			}
		}
	}

	if lastBar.Close > ema21 && rsi >= 50 {
		bias = "BULLISH"
		if lastBar.High > prevHigh {
			structure = "Higher Highs / Bullish Flow"
		} else {
			structure = "Pullback Support Holding"
		}
	} else if lastBar.Close < ema21 && rsi <= 50 {
		bias = "BEARISH"
		if lastBar.Low < prevLow {
			structure = "Lower Lows / Bearish Flow"
		} else {
			structure = "Lower Highs / Rejection"
		}
	} else {
		if rsi > 55 {
			bias = "MOD. BULLISH"
			structure = "Above EMA / Momentum Building"
		} else if rsi < 45 {
			bias = "MOD. BEARISH"
			structure = "Below EMA / Selling Pressure"
		} else {
			bias = "NEUTRAL"
			structure = "Range Bound / Equilibrium"
		}
	}

	var recentCandles []CandleMorphology
	startIdx := len(bars) - 3
	if startIdx < 0 {
		startIdx = 0
	}
	for i := startIdx; i < len(bars); i++ {
		var prev *Bar
		if i > 0 {
			prev = &bars[i-1]
		}
		cm := analyzeCandleMorphology(bars[i], prev, precision)
		recentCandles = append(recentCandles, cm)
	}

	var latestCandle CandleMorphology
	if len(recentCandles) > 0 {
		latestCandle = recentCandles[len(recentCandles)-1]
	}

	return TimeframeAnalysis{
		TF:            tf,
		Bias:          bias,
		Structure:     structure,
		RSI:           math.Round(rsi*10) / 10,
		EMA21:         ema21,
		Candle:        candleStr,
		LatestCandle:  latestCandle,
		RecentCandles: recentCandles,
	}
}

func analyzeCandleMorphology(b Bar, prevBar *Bar, precision int) CandleMorphology {
	t := time.Unix(b.Time, 0).UTC()
	timeStr := t.Format("15:04")
	rng := b.High - b.Low
	if rng <= 0 {
		rng = 0.0001
	}

	body := math.Abs(b.Close - b.Open)
	upperWick := b.High - math.Max(b.Open, b.Close)
	lowerWick := math.Min(b.Open, b.Close) - b.Low
	diff := b.Close - b.Open

	bodyPct := (body / rng) * 100
	upperPct := (upperWick / rng) * 100
	lowerPct := (lowerWick / rng) * 100

	pattern := "Standard Candle"
	isBull := b.Close >= b.Open

	if upperPct >= 55 {
		if isBull {
			pattern = "Inverted Hammer (Upper Rejection)"
		} else {
			pattern = "Shooting Star (Strong Upper Rejection)"
		}
	} else if lowerPct >= 55 {
		if isBull {
			pattern = "Hammer (Strong Lower Demand)"
		} else {
			pattern = "Hanging Man (Lower Demand Test)"
		}
	} else if bodyPct >= 70 {
		if isBull {
			pattern = "Bullish Marubozu (Strong Momentum)"
		} else {
			pattern = "Bearish Marubozu (Heavy Selling)"
		}
	} else if bodyPct <= 15 {
		if upperPct > 40 && lowerPct > 40 {
			pattern = "Long-Legged Doji (Indecision)"
		} else if upperPct > 50 {
			pattern = "Gravestone Doji (Supply Dominant)"
		} else if lowerPct > 50 {
			pattern = "Dragonfly Doji (Demand Dominant)"
		} else {
			pattern = "Doji (Equilibrium)"
		}
	} else {
		if isBull {
			if prevBar != nil && prevBar.Close < prevBar.Open && b.Close > prevBar.Open && b.Open < prevBar.Close {
				pattern = "Bullish Engulfing"
			} else {
				pattern = "Bullish Body"
			}
		} else {
			if prevBar != nil && prevBar.Close > prevBar.Open && b.Close < prevBar.Open && b.Open > prevBar.Close {
				pattern = "Bearish Engulfing"
			} else {
				pattern = "Bearish Body"
			}
		}
	}

	summary := fmt.Sprintf("%s [Body:%.0f%%, UW:%.0f%%, LW:%.0f%%]", pattern, bodyPct, upperPct, lowerPct)

	return CandleMorphology{
		TimeStr:       timeStr,
		Open:          b.Open,
		High:          b.High,
		Low:           b.Low,
		Close:         b.Close,
		Volume:        b.Volume,
		Change:        diff,
		Range:         rng,
		BodySize:      body,
		UpperWick:     upperWick,
		LowerWick:     lowerWick,
		BodyPercent:   math.Round(bodyPct*10) / 10,
		UpperWickPerc: math.Round(upperPct*10) / 10,
		LowerWickPerc: math.Round(lowerPct*10) / 10,
		Pattern:       pattern,
		Summary:       summary,
	}
}

func calculateMTFMatrix(bars1M, bars5M, bars15M, bars1H, bars4H, barsD []Bar, precision int) MTFMatrix {
	tfList := []struct {
		name string
		bars []Bar
	}{
		{"M1 (1-Min)", bars1M},
		{"M5 (5-Min)", bars5M},
		{"M15 (15-Min)", bars15M},
		{"H1 (1-Hour)", bars1H},
		{"H4 (4-Hour)", bars4H},
		{"D1 (Daily)", barsD},
	}

	var frames []TimeframeAnalysis
	bullCount, bearCount, neutCount := 0, 0, 0

	for _, item := range tfList {
		ta := calculateTimeframeAnalysis(item.name, item.bars, precision)
		frames = append(frames, ta)
		if strings.Contains(ta.Bias, "BULL") {
			bullCount++
		} else if strings.Contains(ta.Bias, "BEAR") {
			bearCount++
		} else {
			neutCount++
		}
	}

	summary := ""
	if bearCount >= 4 {
		summary = fmt.Sprintf("🔴 Strong Bearish Confluence (%d/6 Bearish) ➔ Short Direction Heavily Favored", bearCount)
	} else if bullCount >= 4 {
		summary = fmt.Sprintf("🟢 Strong Bullish Confluence (%d/6 Bullish) ➔ Long Direction Heavily Favored", bullCount)
	} else if bearCount > bullCount {
		summary = fmt.Sprintf("🔴 Moderate Bearish Bias (%d Bear / %d Bull / %d Neutral) ➔ Sell on Rallies", bearCount, bullCount, neutCount)
	} else if bullCount > bearCount {
		summary = fmt.Sprintf("🟢 Moderate Bullish Bias (%d Bull / %d Bear / %d Neutral) ➔ Buy on Dips", bullCount, bearCount, neutCount)
	} else {
		summary = fmt.Sprintf("🟡 Mixed / Choppy Alignment (%d Bull / %d Bear / %d Neutral) ➔ Range Bound", bullCount, bearCount, neutCount)
	}

	return MTFMatrix{
		Frames:           frames,
		AlignmentSummary: summary,
		BullishCount:     bullCount,
		BearishCount:     bearCount,
		NeutralCount:     neutCount,
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
	case "SPX", "SPX500", "US500", "S&P500", "SP500":
		return "TVC:SPX"
	case "NDX", "NAS100", "US100", "NASDAQ":
		return "TVC:NDX"
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
