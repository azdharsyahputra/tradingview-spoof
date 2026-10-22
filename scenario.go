package tvspoof

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// Market-reaction & liquidity advisor. It maps where liquidity rests, what
// structure each timeframe is printing, and turns the nearest levels into
// machine-checkable if-then scenarios that cmd/shadow can paper-trade.

// SwingPoint is a confirmed fractal pivot (2 candles each side).
type SwingPoint struct {
	Price float64 `json:"price"`
	Time  int64   `json:"time"`
	High  bool    `json:"high"`
	Label string  `json:"label"` // HH, LH, HL, LL (first of each side: SH / SL)
	index int
}

// TFStructure is the swing structure of one timeframe.
type TFStructure struct {
	TF        string       `json:"tf"`
	Trend     string       `json:"trend"` // "UP", "DOWN", "RANGE"
	Sequence  string       `json:"sequence"`
	Swings    []SwingPoint `json:"swings"`               // most recent last
	LastEvent string       `json:"last_event,omitempty"` // "BOS UP 4321.5 @15:30", "CHoCH DOWN ..."
	// FlipLevel: a candle close beyond it changes the trend (last LH in a
	// downtrend, last HL in an uptrend).
	FlipLevel float64 `json:"flip_level,omitempty"`
	FlipNote  string  `json:"flip_note,omitempty"`
}

// Channel is a regression channel through recent M15 swing highs and lows.
type Channel struct {
	Kind     string  `json:"kind"` // "ASCENDING", "DESCENDING", "HORIZONTAL"
	Upper    float64 `json:"upper"`
	Lower    float64 `json:"lower"`
	Position float64 `json:"position_pct"` // 0 = lower rail, 100 = upper rail
	State    string  `json:"state"`        // "INSIDE", "BREAK UP", "BREAK DOWN"
}

// LiquidityLevel is a resting-liquidity price (cluster of confluent levels).
type LiquidityLevel struct {
	Price    float64  `json:"price"`
	Side     string   `json:"side"` // "BUY_SIDE" (above price) or "SELL_SIDE" (below)
	Names    []string `json:"names"`
	Strength int      `json:"strength"` // number of confluent sources
	DistATR  float64  `json:"dist_atr"`
	Swept    bool     `json:"swept_today,omitempty"`
	// Touches is the number of separate M5 touch episodes today (±0.1 ATR);
	// 0 = fresh. LastTouchMin is minutes since the last touch (-1 = none today).
	Touches      int `json:"touches_today"`
	LastTouchMin int `json:"last_touch_min"`
}

// MarketContext describes HOW price is trading, not just where levels are.
type MarketContext struct {
	Move1hATR   float64 `json:"move_1h_atr"`  // last 12 M5 candles, close - open, in M15 ATR
	Move4hATR   float64 `json:"move_4h_atr"`  // last 48 M5 candles
	StreakM5    int     `json:"streak_m5"`    // +n consecutive bull closes / -n bear
	BodyRatio   float64 `json:"body_ratio"`   // avg body last 3 M5 / avg body prior 9 (<0.6 = decelerating)
	RelVolLast  float64 `json:"rel_vol_last"` // last closed M5 volume / 20-candle average
	ADR         float64 `json:"adr"`          // 14-day average daily range
	TodayRange  float64 `json:"today_range"`
	ADRUsedPct  float64 `json:"adr_used_pct"`
	D1Trend     string  `json:"d1_trend"`      // close vs EMA20 D1
	D1DistATR   float64 `json:"d1_dist_atr"`   // (price - EMA20 D1) / ATR14 D1
	H4Trend     string  `json:"h4_trend"`      // close vs EMA50 H4
	Killzone    string  `json:"killzone"`      // e.g. "London open 25m lagi"
	Range30mATR float64 `json:"range_30m_atr"` // high-low of last 6 M5 / ATR (small = compression)
	AbsorbLow   int     `json:"absorb_low"`    // last 6 M5 with lower wick >= 50% of range (bids absorbing)
	AbsorbHigh  int     `json:"absorb_high"`   // last 6 M5 with upper wick >= 50% of range (offers absorbing)
	DXYMove1h   float64 `json:"dxy_move_1h_pct,omitempty"`
	DXYMove4h   float64 `json:"dxy_move_4h_pct,omitempty"`
	DXYNote     string  `json:"dxy_note,omitempty"`

	// Evidence-backed filters (cmd/sweepstudy research, Oct 2026).
	Activity     string  `json:"activity"`           // "SIAGA" (M15 vol >=1.5x in 00-04/07-10/12-15 UTC), "SEPI" (<1x off-hours), "NORMAL"
	RelVolM15    float64 `json:"rel_vol_m15"`        // last closed M15 volume / prior 16 M15 average
	EMA15DistATR float64 `json:"ema20_m15_dist_atr"` // (price - EMA20 M15) / ATR M15; |x| >= 1.5 = edge, below = middle
	D1SlopeATR   float64 `json:"d1_ema20_slope_atr"` // EMA20 D1 change over 3 days / ATR14 D1
	Mom24h       float64 `json:"mom_24h"`            // price - H4 close ~24h ago
	SwingBias    string  `json:"swing_bias"`         // "BUY" when slope >= 0.4 and Mom24h > 0, else "NETRAL"
	Box5High     float64 `json:"box5_high"`          // high of the last 5 closed daily candles
	Box5Low      float64 `json:"box5_low"`
	Box5State    string  `json:"box5_state"` // "INSIDE", "ABOVE" or "BELOW": outside = do not fade
}

// FVGZone is an unfilled fair value gap.
type FVGZone struct {
	TF     string  `json:"tf"`
	Bull   bool    `json:"bull"`
	Bottom float64 `json:"bottom"`
	Top    float64 `json:"top"`
	Time   int64   `json:"time"`
}

// Scenario is a conditional, machine-checkable trade idea.
type Scenario struct {
	ID         string  `json:"id"`
	Setup      string  `json:"setup"` // "SWEEP_REJECT" or "BREAK_RETEST"
	Side       string  `json:"side"`  // "BUY" or "SELL"
	Level      float64 `json:"level"`
	LevelName  string  `json:"level_name"`
	Status     string  `json:"status"` // "AT_LEVEL", "APPROACHING", "FAR"
	DistATR    float64 `json:"dist_atr"`
	Aligned    bool    `json:"aligned"`  // with M15/H1 structure
	Priority   string  `json:"priority"` // "PRIMARY", "SECONDARY", "COUNTER"
	Trigger    string  `json:"trigger"`
	EstSL      float64 `json:"est_sl"`
	Target1    float64 `json:"target_1"`
	Target2    float64 `json:"target_2"`
	TargetNote string  `json:"target_note"`
	Invalid    string  `json:"invalidation"`
	Advice     string  `json:"advice"`
}

// ReactionEvent is something price just did at a liquidity level.
type ReactionEvent struct {
	Time  int64   `json:"time"`
	TF    string  `json:"tf"`
	Kind  string  `json:"kind"` // "SWEEP_HIGH", "SWEEP_LOW", "BREAK_UP", "BREAK_DOWN", "FVG_BULL", "FVG_BEAR"
	Level string  `json:"level,omitempty"`
	Price float64 `json:"price"`
}

// ScenarioReport is the advisor output.
type ScenarioReport struct {
	Symbol    string           `json:"symbol"`
	Time      time.Time        `json:"time"`
	Price     float64          `json:"price"`
	Precision int              `json:"precision"`
	ATR15     float64          `json:"atr_m15"`
	ATR5      float64          `json:"atr_m5"`
	Session   string           `json:"session"`
	Structure []TFStructure    `json:"structure"`
	Bias      string           `json:"bias"` // structural bias M15+H1
	Channel   *Channel         `json:"channel,omitempty"`
	Context   MarketContext    `json:"context"`
	Above     []LiquidityLevel `json:"liquidity_above"`
	Below     []LiquidityLevel `json:"liquidity_below"`
	FVGs      []FVGZone        `json:"active_fvgs"`
	Reactions []ReactionEvent  `json:"recent_reactions"`
	Scenarios []Scenario       `json:"scenarios"`
	Summary   string           `json:"summary"`
}

// AnalyzeScenarios fetches M5/M15/H1/H4/D and builds the advisor report.
func (c *Client) AnalyzeScenarios(ctx context.Context, symbol string) (*ScenarioReport, error) {
	sym := NormalizeDeskSymbol(symbol)
	var m5, m15, h1, h4, d []Bar
	var err5, err15 error
	var wg sync.WaitGroup
	wg.Add(5)
	go func() { defer wg.Done(); m5, err5 = c.GetHistory(ctx, sym, "5", 300) }()
	go func() { defer wg.Done(); m15, err15 = c.GetHistory(ctx, sym, "15", 150) }()
	go func() { defer wg.Done(); h1, _ = c.GetHistory(ctx, sym, "60", 120) }()
	go func() { defer wg.Done(); h4, _ = c.GetHistory(ctx, sym, "240", 100) }()
	go func() { defer wg.Done(); d, _ = c.GetHistory(ctx, sym, "D", 60) }()
	wg.Wait()
	if err15 != nil || len(m15) < 30 {
		return nil, fmt.Errorf("M15 history for %s: %v", sym, err15)
	}
	if err5 != nil {
		m5 = nil
	}
	var dxy []Bar
	if usdExposure(sym) != 0 {
		dctx, dcancel := context.WithTimeout(ctx, 15*time.Second)
		dxy, _ = c.GetHistory(dctx, "TVC:DXY", "15", 40)
		dcancel()
	}
	return BuildScenarioReport(sym, m5, m15, h1, h4, d, dxy, time.Now().UTC()), nil
}

// BuildScenarioReport is the pure (replayable) advisor.
// dxy (M15, optional) adds the dollar's intraday move for USD-legged symbols.
func BuildScenarioReport(sym string, m5, m15, h1, h4, d, dxy []Bar, now time.Time) *ScenarioReport {
	prec := DeterminePrecisionForSymbol(sym)
	c5 := closedOnly(m5, 5*time.Minute, now)
	c15 := closedOnly(m15, 15*time.Minute, now)
	c1h := closedOnly(h1, time.Hour, now)
	c4h := closedOnly(h4, 4*time.Hour, now)

	price := 0.0
	switch {
	case len(m5) > 0:
		price = m5[len(m5)-1].Close
	case len(m15) > 0:
		price = m15[len(m15)-1].Close
	}
	rep := &ScenarioReport{Symbol: sym, Time: now, Price: price, Precision: prec}
	rep.ATR15 = calculateATR(c15, 14)
	rep.ATR5 = calculateATR(c5, 14)
	if rep.ATR15 <= 0 {
		rep.ATR15 = price * 0.002
	}
	atr := rep.ATR15
	rep.Session = sessionName(now)

	// 1. Structure per timeframe
	for _, tf := range []struct {
		name string
		bars []Bar
	}{{"M5", c5}, {"M15", c15}, {"H1", c1h}, {"H4", c4h}} {
		if len(tf.bars) >= 10 {
			rep.Structure = append(rep.Structure, analyzeStructure(tf.name, tf.bars, prec))
		}
	}
	rep.Bias = structuralBias(rep.Structure)

	// 2. Channel on M15
	rep.Channel = fitChannel(c15, atr)

	// 3. Liquidity map
	liq := calculateSessionLiquidity(m15, d, price, now)
	var raw []namedLevel
	addLvl := func(name string, px float64) {
		if px > 0 {
			raw = append(raw, namedLevel{name, px})
		}
	}
	addLvl("PDH", liq.PDH)
	addLvl("PDL", liq.PDL)
	addLvl("Asia High", liq.AsiaHigh)
	addLvl("Asia Low", liq.AsiaLow)
	if now.UTC().Hour() >= 8 {
		addLvl("London High", liq.LondonHigh)
		addLvl("London Low", liq.LondonLow)
	}
	addLvl("Daily Open", liq.DailyOpen)
	for _, tf := range []struct {
		name string
		bars []Bar
	}{{"M15", c15}, {"H1", c1h}} {
		highs, lows := unsweptSwings(tf.bars)
		for _, s := range highs {
			addLvl(tf.name+" swing high", s.Price)
		}
		for _, s := range lows {
			addLvl(tf.name+" swing low", s.Price)
		}
		for _, eq := range equalLevels(highs, 0.15*atr) {
			addLvl(tf.name+" EQH (double top liquidity)", eq)
		}
		for _, eq := range equalLevels(lows, 0.15*atr) {
			addLvl(tf.name+" EQL (double bottom liquidity)", eq)
		}
	}
	if rep.Channel != nil && rep.Channel.State == "INSIDE" {
		addLvl("Channel upper rail", rep.Channel.Upper)
		addLvl("Channel lower rail", rep.Channel.Lower)
	}
	clusters := clusterLevels(raw, 0.25*atr)
	todayHigh, todayLow := todayRange(m15)
	for _, cl := range clusters {
		cl.DistATR = math.Abs(cl.Price-price) / atr
		if cl.Price > price {
			cl.Side = "BUY_SIDE"
			cl.Swept = todayHigh > cl.Price && containsAny(cl.Names, "PDH", "Asia High")
			rep.Above = append(rep.Above, cl)
		} else {
			cl.Side = "SELL_SIDE"
			cl.Swept = todayLow < cl.Price && containsAny(cl.Names, "PDL", "Asia Low")
			rep.Below = append(rep.Below, cl)
		}
	}
	levelTouches(rep.Above, c5, atr, now)
	levelTouches(rep.Below, c5, atr, now)
	sort.Slice(rep.Above, func(i, j int) bool { return rep.Above[i].Price < rep.Above[j].Price })
	sort.Slice(rep.Below, func(i, j int) bool { return rep.Below[i].Price > rep.Below[j].Price })
	rep.Above = capLevels(rep.Above, 5)
	rep.Below = capLevels(rep.Below, 5)

	// 4. Active FVGs (M5 + M15) and recent reactions
	rep.FVGs = append(unfilledFVGs("M15", c15, price, 3), unfilledFVGs("M5", c5, price, 3)...)
	rep.Reactions = recentReactions(c5, clusters, 12, prec)

	rep.Context = marketContext(sym, c5, c15, c4h, closedOnly(d, 24*time.Hour, now), closedOnly(dxy, 15*time.Minute, now), todayHigh-todayLow, price, atr, now)

	// 5. Scenarios on the nearest level each side
	rep.Scenarios = buildScenarios(rep, atr)
	rep.Summary = summarize(rep)
	return rep
}

type namedLevel struct {
	name  string
	price float64
}

func sessionName(t time.Time) string {
	h := t.UTC().Hour()
	switch {
	case h < 7:
		return "ASIA"
	case h < 12:
		return "LONDON"
	case h < 16:
		return "LONDON_NY_OVERLAP"
	case h < 21:
		return "NEW_YORK"
	default:
		return "LATE_NY"
	}
}

// fractalSwings finds pivots with 2 candles on each side.
func fractalSwings(bars []Bar) []SwingPoint {
	var out []SwingPoint
	for i := 2; i < len(bars)-2; i++ {
		h, l := bars[i].High, bars[i].Low
		if h > bars[i-1].High && h > bars[i-2].High && h >= bars[i+1].High && h >= bars[i+2].High {
			out = append(out, SwingPoint{Price: h, Time: bars[i].Time, High: true, index: i})
		}
		if l < bars[i-1].Low && l < bars[i-2].Low && l <= bars[i+1].Low && l <= bars[i+2].Low {
			out = append(out, SwingPoint{Price: l, Time: bars[i].Time, High: false, index: i})
		}
	}
	return out
}

func analyzeStructure(tf string, bars []Bar, prec int) TFStructure {
	st := TFStructure{TF: tf, Trend: "RANGE"}
	swings := fractalSwings(bars)
	var lastH, lastL *SwingPoint
	var prevH, prevL *SwingPoint
	for i := range swings {
		s := &swings[i]
		if s.High {
			switch {
			case lastH == nil:
				s.Label = "SH"
			case s.Price > lastH.Price:
				s.Label = "HH"
			default:
				s.Label = "LH"
			}
			prevH, lastH = lastH, s
		} else {
			switch {
			case lastL == nil:
				s.Label = "SL"
			case s.Price > lastL.Price:
				s.Label = "HL"
			default:
				s.Label = "LL"
			}
			prevL, lastL = lastL, s
		}
	}
	if len(swings) > 6 {
		st.Swings = swings[len(swings)-6:]
	} else {
		st.Swings = swings
	}
	var parts []string
	for _, s := range st.Swings {
		parts = append(parts, fmt.Sprintf("%s %.*f", s.Label, prec, s.Price))
	}
	st.Sequence = strings.Join(parts, " → ")
	if lastH == nil || lastL == nil || prevH == nil || prevL == nil {
		return st
	}
	up := lastH.Price > prevH.Price && lastL.Price > prevL.Price
	down := lastH.Price < prevH.Price && lastL.Price < prevL.Price
	switch {
	case up:
		st.Trend = "UP"
		st.FlipLevel = lastL.Price
		st.FlipNote = fmt.Sprintf("close %s di bawah HL %.*f = CHoCH bearish", tf, prec, lastL.Price)
	case down:
		st.Trend = "DOWN"
		st.FlipLevel = lastH.Price
		st.FlipNote = fmt.Sprintf("close %s di atas LH %.*f = CHoCH bullish", tf, prec, lastH.Price)
	}

	// Break events after the last swing of each side, by candle CLOSE.
	last := bars[len(bars)-1]
	tstr := time.Unix(last.Time, 0).UTC().Format("15:04")
	for i := maxInt(lastH.index, lastL.index) + 1; i < len(bars); i++ {
		b := bars[i]
		tstr = time.Unix(b.Time, 0).UTC().Format("Jan 02 15:04")
		if b.Close > lastH.Price {
			kind := "BOS UP"
			if st.Trend == "DOWN" {
				kind = "CHoCH UP"
				st.Trend = "TRANSITION_UP"
			}
			st.LastEvent = fmt.Sprintf("%s: close %.*f tembus %s %.*f (%s UTC)", kind, prec, b.Close, lastH.Label, prec, lastH.Price, tstr)
			break
		}
		if b.Close < lastL.Price {
			kind := "BOS DOWN"
			if st.Trend == "UP" {
				kind = "CHoCH DOWN"
				st.Trend = "TRANSITION_DOWN"
			}
			st.LastEvent = fmt.Sprintf("%s: close %.*f tembus %s %.*f (%s UTC)", kind, prec, b.Close, lastL.Label, prec, lastL.Price, tstr)
			break
		}
	}
	return st
}

func structuralBias(sts []TFStructure) string {
	trend := map[string]string{}
	for _, s := range sts {
		trend[s.TF] = s.Trend
	}
	score := 0
	for tf, w := range map[string]int{"M15": 1, "H1": 2, "H4": 1} {
		switch {
		case strings.HasSuffix(trend[tf], "UP"):
			score += w
		case strings.HasSuffix(trend[tf], "DOWN"):
			score -= w
		}
	}
	switch {
	case score >= 2:
		return "BULLISH"
	case score <= -2:
		return "BEARISH"
	}
	return "MIXED"
}

func fitChannel(bars []Bar, atr float64) *Channel {
	swings := fractalSwings(bars)
	var hs, ls []SwingPoint
	for _, s := range swings {
		if s.High {
			hs = append(hs, s)
		} else {
			ls = append(ls, s)
		}
	}
	if len(hs) < 3 || len(ls) < 3 || atr <= 0 {
		return nil
	}
	hs, ls = hs[len(hs)-3:], ls[len(ls)-3:]
	fit := func(pts []SwingPoint) (slope, icpt float64) {
		var sx, sy, sxx, sxy float64
		for _, p := range pts {
			x := float64(p.index)
			sx += x
			sy += p.Price
			sxx += x * x
			sxy += x * p.Price
		}
		n := float64(len(pts))
		den := n*sxx - sx*sx
		if den == 0 {
			return 0, sy / n
		}
		slope = (n*sxy - sx*sy) / den
		return slope, (sy - slope*sx) / n
	}
	sh, ih := fit(hs)
	sl, il := fit(ls)
	// Rails must be roughly parallel (slopes within 0.05 ATR/bar).
	if math.Abs(sh-sl) > 0.05*atr {
		return nil
	}
	x := float64(len(bars) - 1)
	ch := &Channel{Upper: sh*x + ih, Lower: sl*x + il}
	if ch.Upper-ch.Lower < atr {
		return nil
	}
	avg := (sh + sl) / 2
	switch {
	case avg > 0.02*atr:
		ch.Kind = "ASCENDING"
	case avg < -0.02*atr:
		ch.Kind = "DESCENDING"
	default:
		ch.Kind = "HORIZONTAL"
	}
	lastClose := bars[len(bars)-1].Close
	ch.Position = (lastClose - ch.Lower) / (ch.Upper - ch.Lower) * 100
	switch {
	case lastClose > ch.Upper+0.1*atr:
		ch.State = "BREAK UP"
	case lastClose < ch.Lower-0.1*atr:
		ch.State = "BREAK DOWN"
	default:
		ch.State = "INSIDE"
	}
	return ch
}

// unsweptSwings returns the swing highs/lows no later candle has traded through.
func unsweptSwings(bars []Bar) (highs, lows []SwingPoint) {
	for _, s := range fractalSwings(bars) {
		swept := false
		for j := s.index + 1; j < len(bars); j++ {
			if (s.High && bars[j].High > s.Price) || (!s.High && bars[j].Low < s.Price) {
				swept = true
				break
			}
		}
		if swept {
			continue
		}
		if s.High {
			highs = append(highs, s)
		} else {
			lows = append(lows, s)
		}
	}
	return highs, lows
}

func equalLevels(pts []SwingPoint, tol float64) []float64 {
	var out []float64
	for i := 0; i < len(pts); i++ {
		for j := i + 1; j < len(pts); j++ {
			if math.Abs(pts[i].Price-pts[j].Price) <= tol {
				out = append(out, (pts[i].Price+pts[j].Price)/2)
			}
		}
	}
	return out
}

func clusterLevels(raw []namedLevel, tol float64) []LiquidityLevel {
	sort.Slice(raw, func(i, j int) bool { return raw[i].price < raw[j].price })
	var out []LiquidityLevel
	for _, r := range raw {
		if n := len(out); n > 0 && r.price-out[n-1].Price <= tol {
			c := &out[n-1]
			c.Price = (c.Price*float64(c.Strength) + r.price) / float64(c.Strength+1)
			c.Strength++
			if !containsAny(c.Names, r.name) {
				c.Names = append(c.Names, r.name)
			}
			continue
		}
		out = append(out, LiquidityLevel{Price: r.price, Names: []string{r.name}, Strength: 1})
	}
	return out
}

func capLevels(l []LiquidityLevel, n int) []LiquidityLevel {
	if len(l) > n {
		return l[:n]
	}
	return l
}

func containsAny(names []string, keys ...string) bool {
	for _, n := range names {
		for _, k := range keys {
			if n == k {
				return true
			}
		}
	}
	return false
}

func todayRange(m15 []Bar) (hi, lo float64) {
	if len(m15) == 0 {
		return 0, 0
	}
	day := time.Unix(m15[len(m15)-1].Time, 0).UTC().Format("2006-01-02")
	first := true
	for _, b := range m15 {
		if time.Unix(b.Time, 0).UTC().Format("2006-01-02") != day {
			continue
		}
		if first {
			hi, lo, first = b.High, b.Low, false
		}
		hi, lo = math.Max(hi, b.High), math.Min(lo, b.Low)
	}
	return hi, lo
}

func unfilledFVGs(tf string, bars []Bar, price float64, keep int) []FVGZone {
	var zones []FVGZone
	for i := 2; i < len(bars); i++ {
		if bars[i].Low > bars[i-2].High {
			z := FVGZone{TF: tf, Bull: true, Bottom: bars[i-2].High, Top: bars[i].Low, Time: bars[i-1].Time}
			if !fvgFilled(z, bars[i+1:]) {
				zones = append(zones, z)
			}
		}
		if bars[i].High < bars[i-2].Low {
			z := FVGZone{TF: tf, Bull: false, Bottom: bars[i].High, Top: bars[i-2].Low, Time: bars[i-1].Time}
			if !fvgFilled(z, bars[i+1:]) {
				zones = append(zones, z)
			}
		}
	}
	sort.Slice(zones, func(i, j int) bool {
		return math.Abs((zones[i].Top+zones[i].Bottom)/2-price) < math.Abs((zones[j].Top+zones[j].Bottom)/2-price)
	})
	if len(zones) > keep {
		zones = zones[:keep]
	}
	return zones
}

func fvgFilled(z FVGZone, later []Bar) bool {
	for _, b := range later {
		if (z.Bull && b.Low <= z.Bottom) || (!z.Bull && b.High >= z.Top) {
			return true
		}
	}
	return false
}

func recentReactions(c5 []Bar, levels []LiquidityLevel, lookback, prec int) []ReactionEvent {
	var out []ReactionEvent
	start := len(c5) - lookback
	if start < 2 {
		start = 2
	}
	for i := start; i < len(c5); i++ {
		b := c5[i]
		for _, l := range levels {
			name := strings.Join(l.Names, "+")
			switch {
			case b.High > l.Price && b.Close < l.Price && b.Open < l.Price:
				out = append(out, ReactionEvent{b.Time, "M5", "SWEEP_HIGH", name, l.Price})
			case b.Low < l.Price && b.Close > l.Price && b.Open > l.Price:
				out = append(out, ReactionEvent{b.Time, "M5", "SWEEP_LOW", name, l.Price})
			case b.Open < l.Price && b.Close > l.Price:
				out = append(out, ReactionEvent{b.Time, "M5", "BREAK_UP", name, l.Price})
			case b.Open > l.Price && b.Close < l.Price:
				out = append(out, ReactionEvent{b.Time, "M5", "BREAK_DOWN", name, l.Price})
			}
		}
		if c5[i].Low > c5[i-2].High {
			out = append(out, ReactionEvent{Time: c5[i-1].Time, TF: "M5", Kind: "FVG_BULL", Price: c5[i-2].High})
		}
		if c5[i].High < c5[i-2].Low {
			out = append(out, ReactionEvent{Time: c5[i-1].Time, TF: "M5", Kind: "FVG_BEAR", Price: c5[i-2].Low})
		}
	}
	return out
}

func buildScenarios(rep *ScenarioReport, atr float64) []Scenario {
	p := rep.Precision
	f := func(v float64) string { return fmt.Sprintf("%.*f", p, v) }
	var out []Scenario
	status := func(dist float64) string {
		switch {
		case dist <= 0.3:
			return "AT_LEVEL"
		case dist <= 1.2:
			return "APPROACHING"
		}
		return "FAR"
	}
	nextBeyond := func(levels []LiquidityLevel, i int, fallback float64) (float64, string) {
		if i < len(levels) {
			return levels[i].Price, strings.Join(levels[i].Names, "+")
		}
		return fallback, "proyeksi 1.5R"
	}
	priority := func(side string) (bool, string) {
		aligned := (side == "BUY" && rep.Bias == "BULLISH") || (side == "SELL" && rep.Bias == "BEARISH")
		switch {
		case aligned:
			return true, "PRIMARY"
		case rep.Bias == "MIXED":
			return false, "SECONDARY"
		}
		return false, "COUNTER"
	}

	if len(rep.Above) > 0 {
		r := rep.Above[0]
		name := strings.Join(r.Names, "+")
		dist := r.DistATR
		// Reject at resistance -> SELL
		t1, n1 := nextBeyond(rep.Below, 0, r.Price-2*atr)
		t2, n2 := nextBeyond(rep.Below, 1, t1-1.5*atr)
		al, pr := priority("SELL")
		out = append(out, Scenario{
			ID: "R1-REJECT-SELL", Setup: "SWEEP_REJECT", Side: "SELL", Level: r.Price, LevelName: name,
			Status: status(dist), DistATR: dist, Aligned: al, Priority: pr,
			Trigger: fmt.Sprintf("harga nyentuh/sweep %s %s, lalu candle M5 bearish CLOSE di bawahnya + ninggalin FVG bearish baru", name, f(r.Price)),
			EstSL:   r.Price + 0.5*atr, Target1: t1, Target2: t2, TargetNote: n1 + " → " + n2,
			Invalid: fmt.Sprintf("M5 close di atas %s sebelum trigger = reject gagal, pindah ke skenario break", f(r.Price+0.5*atr)),
		})
		// Break of resistance -> BUY
		t1, n1 = nextBeyond(rep.Above, 1, r.Price+2*atr)
		t2, n2 = nextBeyond(rep.Above, 2, t1+1.5*atr)
		al, pr = priority("BUY")
		out = append(out, Scenario{
			ID: "R1-BREAK-BUY", Setup: "BREAK_RETEST", Side: "BUY", Level: r.Price, LevelName: name,
			Status: status(dist), DistATR: dist, Aligned: al, Priority: pr,
			Trigger: fmt.Sprintf("M5 CLOSE tembus di atas %s %s, tunggu retest ke level itu yang HOLD (close tetap di atas)", name, f(r.Price)),
			EstSL:   r.Price - 0.5*atr, Target1: t1, Target2: t2, TargetNote: n1 + " → " + n2,
			Invalid: fmt.Sprintf("setelah break, M5 close balik di bawah %s = fake breakout (bisa jadi sinyal sell)", f(r.Price-0.3*atr)),
		})
	}
	if len(rep.Below) > 0 {
		s := rep.Below[0]
		name := strings.Join(s.Names, "+")
		dist := s.DistATR
		t1, n1 := nextBeyond(rep.Above, 0, s.Price+2*atr)
		t2, n2 := nextBeyond(rep.Above, 1, t1+1.5*atr)
		al, pr := priority("BUY")
		out = append(out, Scenario{
			ID: "S1-REJECT-BUY", Setup: "SWEEP_REJECT", Side: "BUY", Level: s.Price, LevelName: name,
			Status: status(dist), DistATR: dist, Aligned: al, Priority: pr,
			Trigger: fmt.Sprintf("harga nyentuh/sweep %s %s, lalu candle M5 bullish CLOSE di atasnya + ninggalin FVG bullish baru", name, f(s.Price)),
			EstSL:   s.Price - 0.5*atr, Target1: t1, Target2: t2, TargetNote: n1 + " → " + n2,
			Invalid: fmt.Sprintf("M5 close di bawah %s sebelum trigger = support jebol, pindah ke skenario breakdown", f(s.Price-0.5*atr)),
		})
		t1, n1 = nextBeyond(rep.Below, 1, s.Price-2*atr)
		t2, n2 = nextBeyond(rep.Below, 2, t1-1.5*atr)
		al, pr = priority("SELL")
		out = append(out, Scenario{
			ID: "S1-BREAK-SELL", Setup: "BREAK_RETEST", Side: "SELL", Level: s.Price, LevelName: name,
			Status: status(dist), DistATR: dist, Aligned: al, Priority: pr,
			Trigger: fmt.Sprintf("M5 CLOSE jebol di bawah %s %s, tunggu retest dari bawah yang gagal reclaim", name, f(s.Price)),
			EstSL:   s.Price + 0.5*atr, Target1: t1, Target2: t2, TargetNote: n1 + " → " + n2,
			Invalid: fmt.Sprintf("setelah breakdown, M5 close balik di atas %s = fake breakdown (bisa jadi sinyal buy)", f(s.Price+0.3*atr)),
		})
	}
	for i := range out {
		sc := &out[i]
		alignTargets(sc)
		verb := "siap-siap"
		switch sc.Status {
		case "APPROACHING":
			verb = "pantau, harga lagi jalan ke sana"
		case "FAR":
			verb = "masih jauh, jangan dikejar"
		}
		side := strings.ToLower(sc.Side)
		sc.Advice = fmt.Sprintf("[%s | %s] %s %s %.1f ATR — %s. Kalau kejadian: %s %s, target %s lalu %s.",
			sc.Priority, sc.Status, sc.LevelName, f(sc.Level), sc.DistATR, verb, side, sc.Setup, f(sc.Target1), f(sc.Target2))
	}
	// Nearest status first, then structure-aligned before counter-trend.
	rank := func(sc Scenario) int {
		st := map[string]int{"AT_LEVEL": 0, "APPROACHING": 1, "FAR": 2}[sc.Status]
		pr := map[string]int{"PRIMARY": 0, "SECONDARY": 1, "COUNTER": 2}[sc.Priority]
		return st*10 + pr
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) })
	return out
}

func summarize(rep *ScenarioReport) string {
	p := rep.Precision
	var b strings.Builder
	fmt.Fprintf(&b, "%s @ %.*f (%s). Bias struktur: %s.", rep.Symbol, p, rep.Price, rep.Session, rep.Bias)
	for _, s := range rep.Structure {
		if s.TF == "M15" || s.TF == "H1" {
			fmt.Fprintf(&b, " %s %s", s.TF, s.Trend)
			if s.FlipNote != "" {
				fmt.Fprintf(&b, " (%s)", s.FlipNote)
			}
			b.WriteString(".")
		}
	}
	if rep.Channel != nil {
		fmt.Fprintf(&b, " Channel M15 %s %s, posisi %.0f%%.", rep.Channel.Kind, rep.Channel.State, rep.Channel.Position)
	}
	if len(rep.Above) > 0 {
		fmt.Fprintf(&b, " Atap terdekat %s %.*f (%.1f ATR).", strings.Join(rep.Above[0].Names, "+"), p, rep.Above[0].Price, rep.Above[0].DistATR)
	}
	if len(rep.Below) > 0 {
		fmt.Fprintf(&b, " Lantai terdekat %s %.*f (%.1f ATR).", strings.Join(rep.Below[0].Names, "+"), p, rep.Below[0].Price, rep.Below[0].DistATR)
	}
	near := false
	for _, s := range rep.Scenarios {
		if s.Status != "FAR" {
			near = true
		}
	}
	if !near {
		b.WriteString(" Harga di tengah range, belum ada level yang dekat — tunggu, jangan entry di tengah.")
	}
	return b.String()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func levelTouches(levels []LiquidityLevel, c5 []Bar, atr float64, now time.Time) {
	day := now.UTC().Format("2006-01-02")
	for k := range levels {
		l := &levels[k]
		l.LastTouchMin = -1
		prev := false
		for _, b := range c5 {
			if time.Unix(b.Time, 0).UTC().Format("2006-01-02") != day {
				prev = false
				continue
			}
			touch := b.High >= l.Price-0.1*atr && b.Low <= l.Price+0.1*atr
			if touch && !prev {
				l.Touches++
			}
			if touch {
				l.LastTouchMin = int(now.Sub(time.Unix(b.Time, 0).Add(5 * time.Minute)).Minutes())
			}
			prev = touch
		}
	}
}

func marketContext(sym string, c5, c15, c4h, cD, dxy []Bar, todayRange, price, atr float64, now time.Time) MarketContext {
	var mc MarketContext
	n := len(c5)
	move := func(k int) float64 {
		if n < k || atr <= 0 {
			return 0
		}
		return (c5[n-1].Close - c5[n-k].Open) / atr
	}
	mc.Move1hATR, mc.Move4hATR = round2(move(12)), round2(move(48))
	for i := n - 1; i >= 0; i-- {
		up := c5[i].Close > c5[i].Open
		down := c5[i].Close < c5[i].Open
		if mc.StreakM5 >= 0 && up {
			mc.StreakM5++
		} else if mc.StreakM5 <= 0 && down {
			mc.StreakM5--
		} else {
			break
		}
	}
	if n >= 12 {
		body := func(bs []Bar) float64 {
			s := 0.0
			for _, b := range bs {
				s += math.Abs(b.Close - b.Open)
			}
			return s / float64(len(bs))
		}
		if prior := body(c5[n-12 : n-3]); prior > 0 {
			mc.BodyRatio = round2(body(c5[n-3:]) / prior)
		}
	}
	if n >= 6 && atr > 0 {
		hi, lo := c5[n-6].High, c5[n-6].Low
		for _, b := range c5[n-6:] {
			hi, lo = math.Max(hi, b.High), math.Min(lo, b.Low)
			if rng := b.High - b.Low; rng > 0 {
				if (math.Min(b.Open, b.Close)-b.Low)/rng >= 0.5 {
					mc.AbsorbLow++
				}
				if (b.High-math.Max(b.Open, b.Close))/rng >= 0.5 {
					mc.AbsorbHigh++
				}
			}
		}
		mc.Range30mATR = round2((hi - lo) / atr)
	}
	if n >= 21 {
		avg := 0.0
		for _, b := range c5[n-21 : n-1] {
			avg += b.Volume
		}
		if avg /= 20; avg > 0 {
			mc.RelVolLast = round2(c5[n-1].Volume / avg)
		}
	}
	if len(cD) > 0 {
		k := 14
		if len(cD) < k {
			k = len(cD)
		}
		for _, b := range cD[len(cD)-k:] {
			mc.ADR += b.High - b.Low
		}
		mc.ADR /= float64(k)
		mc.TodayRange = todayRange
		if mc.ADR > 0 {
			mc.ADRUsedPct = math.Round(todayRange / mc.ADR * 100)
		}
	}
	if ema := calculateEMA(cD, 20); ema > 0 {
		mc.D1Trend = trendWord(price - ema)
		if a := calculateATR(cD, 14); a > 0 {
			mc.D1DistATR = round2((price - ema) / a)
		}
	}
	if ema := calculateEMA(c4h, 50); ema > 0 {
		mc.H4Trend = trendWord(price - ema)
	}
	mc.Killzone = killzone(now)
	evidenceFilters(&mc, c15, c4h, cD, price, atr)
	if m := len(dxy); m >= 17 {
		mc.DXYMove1h = round2((dxy[m-1].Close/dxy[m-5].Close - 1) * 100)
		mc.DXYMove4h = round2((dxy[m-1].Close/dxy[m-17].Close - 1) * 100)
		effect := "netral"
		if fav := -mc.DXYMove4h * usdExposure(sym); fav > 0.05 {
			effect = "tailwind (USD gerak menguntungkan aset)"
		} else if fav < -0.05 {
			effect = "headwind (USD gerak melawan aset)"
		}
		mc.DXYNote = fmt.Sprintf("DXY 1h %+.2f%%, 4h %+.2f%% → %s", mc.DXYMove1h, mc.DXYMove4h, effect)
	}
	return mc
}

// evidenceFilters fills the filters that survived out-of-sample testing:
// M15 activity (when price moves) and, for gold, the D1 swing bias. Entry
// patterns themselves (sweep, FVG, MSS, BOS) tested at coin-flip direction.
func evidenceFilters(mc *MarketContext, c15, c4h, cD []Bar, price, atr float64) {
	mc.Activity = "NORMAL"
	if n := len(c15); n >= 17 {
		avg := 0.0
		for _, b := range c15[n-17 : n-1] {
			avg += b.Volume
		}
		if avg /= 16; avg > 0 {
			mc.RelVolM15 = round2(c15[n-1].Volume / avg)
			h := time.Unix(c15[n-1].Time, 0).UTC().Hour()
			active := h < 4 || (h >= 7 && h < 10) || (h >= 12 && h < 15)
			switch {
			case mc.RelVolM15 >= 1.5 && active:
				mc.Activity = "SIAGA"
			case mc.RelVolM15 < 1 && !active:
				mc.Activity = "SEPI"
			}
		}
	}
	if ema := calculateEMA(c15, 20); ema > 0 && atr > 0 {
		mc.EMA15DistATR = round2((price - ema) / atr)
	}
	if n := len(c4h); n >= 6 {
		mc.Mom24h = round2(price - c4h[n-6].Close)
	}
	if n := len(cD); n >= 5 {
		mc.Box5High, mc.Box5Low = cD[n-5].High, cD[n-5].Low
		for _, b := range cD[n-5:] {
			mc.Box5High, mc.Box5Low = math.Max(mc.Box5High, b.High), math.Min(mc.Box5Low, b.Low)
		}
		switch {
		case price > mc.Box5High:
			mc.Box5State = "ABOVE"
		case price < mc.Box5Low:
			mc.Box5State = "BELOW"
		default:
			mc.Box5State = "INSIDE"
		}
	}
	mc.SwingBias = "NETRAL"
	if n := len(cD); n >= 24 {
		now, prev, a := calculateEMA(cD, 20), calculateEMA(cD[:n-3], 20), calculateATR(cD, 14)
		if a > 0 {
			mc.D1SlopeATR = round2((now - prev) / a)
		}
		if mc.D1SlopeATR >= 0.4 && mc.Mom24h > 0 {
			mc.SwingBias = "BUY"
		}
	}
}

func trendWord(d float64) string {
	if d > 0 {
		return "UP"
	}
	return "DOWN"
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// killzone describes proximity to London (07:00 UTC) and New York (13:30 UTC) opens.
func killzone(now time.Time) string {
	t := now.UTC()
	mins := t.Hour()*60 + t.Minute()
	for _, kz := range []struct {
		name string
		at   int
	}{{"London open", 7 * 60}, {"NY open", 13*60 + 30}} {
		d := kz.at - mins
		switch {
		case d > 0 && d <= 60:
			return fmt.Sprintf("%s %dm lagi (rawan spike/sweep)", kz.name, d)
		case d <= 0 && d > -90:
			return fmt.Sprintf("%s %dm lalu (killzone aktif)", kz.name, -d)
		}
	}
	return "di luar killzone"
}

// alignTargets applies the same target rule EvaluateShadow uses at trigger
// time (TP1 between 1.5x and 6x the stop distance, else 1.5x; TP2 0.5x–8x
// beyond TP1, else +1.5x), using the estimated stop. The advisor, the agent
// prompt and the grader therefore all show the same targets.
func alignTargets(sc *Scenario) {
	dir := 1.0
	if sc.Side == "SELL" {
		dir = -1
	}
	risk := math.Abs(sc.Level - sc.EstSL)
	if risk <= 0 {
		return
	}
	names := strings.SplitN(sc.TargetNote, " → ", 2)
	for len(names) < 2 {
		names = append(names, "")
	}
	if d := (sc.Target1 - sc.Level) * dir; d < 1.5*risk || d > 6*risk {
		sc.Target1 = sc.Level + dir*1.5*risk
		names[0] = "proyeksi 1.5x SL"
	}
	if d := (sc.Target2 - sc.Target1) * dir; d < 0.5*risk || d > 8*risk {
		sc.Target2 = sc.Target1 + dir*1.5*risk
		names[1] = "proyeksi +1.5x SL"
	}
	sc.TargetNote = names[0] + " → " + names[1]
}
