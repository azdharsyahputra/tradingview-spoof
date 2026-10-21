// Command sweepstudy is an offline event study: when price sweeps a liquidity
// level (PDH/PDL, Asia H/L, unswept M15 swing), which observation decides
// whether it reverses or continues?
//
//	go run ./cmd/sweepstudy xauusd eurusd ...   # fetch 10000 M5 per symbol (cached) and report
//
// At checkpoints k M5 bars after the sweep bar, every feature uses only bars
// up to that close; the outcome is measured from that close with symmetric
// ±1 ATR(M15) barriers, so a coin flip scores 50%.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tvspoof "github.com/azdharsyahputra/tradingview-spoof"
)

var checkpoints = []int{0, 1, 2, 3, 6, 12}

const (
	barrierATR = 1.0
	horizon    = 96 // M5 bars = 8h
)

type event struct {
	sym, level string
	t          time.Time
	half       int // 0 = first half of the sample, 1 = second
	static     map[string]string
	cp         []map[string]string // per checkpoint features
	rev        []int               // per checkpoint: 1 reverse, 0 continue, -1 unresolved
}

func main() {
	syms := os.Args[1:]
	if len(syms) == 0 {
		syms = []string{"xauusd"}
	}
	cache := os.Getenv("SWEEP_CACHE")
	if cache == "" {
		cache = os.TempDir()
	}
	var all []event
	for _, s := range syms {
		bars, err := load(cache, s)
		if err != nil {
			fmt.Fprintln(os.Stderr, s, err)
			continue
		}
		ev := study(s, bars)
		fmt.Printf("%s: %d M5 bars (%s → %s), %d sweep events\n", s, len(bars),
			time.Unix(bars[0].Time, 0).UTC().Format("Jan 02"), time.Unix(bars[len(bars)-1].Time, 0).UTC().Format("Jan 02"), len(ev))
		all = append(all, ev...)
	}
	report(all)
}

func load(dir, sym string) ([]tvspoof.Bar, error) {
	tf := os.Getenv("SWEEP_TF") // e.g. "15" to cache M15 instead of M5
	if tf == "" {
		tf = "5"
	}
	path := filepath.Join(dir, "sweep_"+strings.ToLower(sym)+"_m"+tf+".json")
	if b, err := os.ReadFile(path); err == nil {
		var bars []tvspoof.Bar
		if json.Unmarshal(b, &bars) == nil && len(bars) > 0 {
			return bars, nil
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	bars, err := tvspoof.NewClient().GetHistory(ctx, tvspoof.NormalizeDeskSymbol(sym), tf, 10000)
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(bars)
	_ = os.WriteFile(path, b, 0o644)
	return bars, nil
}

// --- aggregation helpers -------------------------------------------------

type agg struct {
	bars []tvspoof.Bar
	atr  []float64 // ATR14 including bar j
	ema  []float64
	ofM5 []int // for each M5 bar: index of the last aggregated bar CLOSED before it starts
}

func aggregate(m5 []tvspoof.Bar, sec int64) agg {
	var a agg
	a.ofM5 = make([]int, len(m5))
	for i, b := range m5 {
		bucket := b.Time / sec * sec
		if n := len(a.bars); n == 0 || a.bars[n-1].Time != bucket {
			a.bars = append(a.bars, tvspoof.Bar{Time: bucket, Open: b.Open, High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume})
		} else {
			x := &a.bars[n-1]
			x.High = math.Max(x.High, b.High)
			x.Low = math.Min(x.Low, b.Low)
			x.Close = b.Close
			x.Volume += b.Volume
		}
		a.ofM5[i] = len(a.bars) - 2 // current bucket is still forming
	}
	a.atr = make([]float64, len(a.bars))
	a.ema = make([]float64, len(a.bars))
	for j, b := range a.bars {
		tr := b.High - b.Low
		if j > 0 {
			pc := a.bars[j-1].Close
			tr = math.Max(tr, math.Max(math.Abs(b.High-pc), math.Abs(b.Low-pc)))
			a.atr[j] = (a.atr[j-1]*13 + tr) / 14
			a.ema[j] = a.ema[j-1] + (b.Close-a.ema[j-1])*2/51
		} else {
			a.atr[j], a.ema[j] = tr, b.Close
		}
	}
	return a
}

// --- level tracking ------------------------------------------------------

type level struct {
	name  string
	price float64
	high  bool // true = buy-side liquidity above price
	from  int  // first M5 index where the level is known
	until int  // last M5 index where it is valid
	tests int  // M5 highs within 0.15 ATR before the sweep
}

func levels(m5 []tvspoof.Bar, m15 agg) []level {
	var out []level
	day := func(i int) int64 { return m5[i].Time / 86400 }
	// day ranges and Asia ranges
	type rng struct {
		hi, lo      float64
		first, last int
		ok          bool
	}
	days := map[int64]*rng{}
	asia := map[int64]*rng{}
	upd := func(m map[int64]*rng, k int64, i int) {
		r := m[k]
		if r == nil {
			r = &rng{hi: m5[i].High, lo: m5[i].Low, first: i, ok: true}
			m[k] = r
		}
		r.hi, r.lo, r.last = math.Max(r.hi, m5[i].High), math.Min(r.lo, m5[i].Low), i
	}
	for i := range m5 {
		upd(days, day(i), i)
		if h := time.Unix(m5[i].Time, 0).UTC().Hour(); h < 7 {
			upd(asia, day(i), i)
		}
	}
	for d, r := range days {
		if prev, ok := days[d-1]; ok {
			// skip weekend-gapped days: previous day must exist
			out = append(out, level{"PDH", prev.hi, true, r.first, r.last, 0}, level{"PDL", prev.lo, false, r.first, r.last, 0})
		}
		if a, ok := asia[d]; ok && a.last+1 <= r.last {
			out = append(out, level{"ASIA_H", a.hi, true, a.last + 1, r.last, 0}, level{"ASIA_L", a.lo, false, a.last + 1, r.last, 0})
		}
	}
	// M15 fractal swings (2 left, 2 right), valid for 2 days once confirmed
	b := m15.bars
	firstM5 := make([]int, len(b)+1)
	j := 0
	for k := range b {
		for j < len(m5) && m5[j].Time < b[k].Time {
			j++
		}
		firstM5[k] = j
	}
	firstM5[len(b)] = len(m5)
	for k := 2; k+2 < len(b); k++ {
		known := firstM5[k+3] - 0 // after bar k+2 closes
		if known >= len(m5) {
			continue
		}
		until := known + 576
		if until >= len(m5) {
			until = len(m5) - 1
		}
		if b[k].High > b[k-1].High && b[k].High > b[k-2].High && b[k].High >= b[k+1].High && b[k].High >= b[k+2].High {
			out = append(out, level{"SWING_H", b[k].High, true, known, until, 0})
		}
		if b[k].Low < b[k-1].Low && b[k].Low < b[k-2].Low && b[k].Low <= b[k+1].Low && b[k].Low <= b[k+2].Low {
			out = append(out, level{"SWING_L", b[k].Low, false, known, until, 0})
		}
	}
	return out
}

// --- the study ------------------------------------------------------------

func study(sym string, m5 []tvspoof.Bar) []event {
	m15 := aggregate(m5, 900)
	h1 := aggregate(m5, 3600)
	lv := levels(m5, m15)
	mid := m5[len(m5)/2].Time
	taken := map[int]bool{} // one event per (bar, side) – first level listed wins after sorting
	sort.SliceStable(lv, func(a, b int) bool { return rank(lv[a].name) < rank(lv[b].name) })
	var out []event
	for _, L := range lv {
		dir := 1.0 // +1: sweep of a high (continuation = up)
		if !L.high {
			dir = -1
		}
		tests := 0
		for i := L.from; i <= L.until && i < len(m5); i++ {
			ai := m15.ofM5[i]
			if ai < 20 {
				break
			}
			atr := m15.atr[ai]
			beyond := (L.high && m5[i].High > L.price) || (!L.high && m5[i].Low < L.price)
			if !beyond {
				near := dir*(extreme(m5[i], L.high)-L.price) > -0.15*atr
				if near {
					tests++
				}
				continue
			}
			if i == L.from || i < 60 || i+horizon+12 >= len(m5) {
				break // level opened already beyond, or no warm-up/forward data
			}
			key := i*2 + map[bool]int{true: 0, false: 1}[L.high]
			if taken[key] {
				break
			}
			taken[key] = true
			out = append(out, buildEvent(sym, L, i, dir, tests, m5, m15, h1, mid))
			break
		}
	}
	return out
}

func rank(n string) int {
	return map[string]int{"PDH": 0, "PDL": 0, "ASIA_H": 1, "ASIA_L": 1, "SWING_H": 2, "SWING_L": 2}[n]
}

func extreme(b tvspoof.Bar, high bool) float64 {
	if high {
		return b.High
	}
	return b.Low
}

func buildEvent(sym string, L level, i int, dir float64, tests int, m5 []tvspoof.Bar, m15, h1 agg, mid int64) event {
	atr := m15.atr[m15.ofM5[i]]
	t := time.Unix(m5[i].Time, 0).UTC()
	e := event{sym: sym, level: L.name, t: t, static: map[string]string{}}
	if m5[i].Time >= mid {
		e.half = 1
	}
	// approach speed: 1h move toward the level, in ATR
	app := dir * (m5[i-1].Close - m5[i-13].Close) / atr
	e.static["approach_1h"] = bin(app, []float64{0.5, 1.5}, []string{"<0.5 slow", "0.5-1.5", ">1.5 fast"})
	// pressure: prior bars probing within 0.15 ATR in the last hour
	near := 0
	for j := i - 12; j < i; j++ {
		if dir*(extreme(m5[j], L.high)-L.price) > -0.15*atr {
			near++
		}
	}
	e.static["probes_1h"] = bin(float64(near), []float64{1, 3}, []string{"0 (first touch)", "1-2", "3+ (absorption)"})
	e.static["tests_total"] = bin(float64(tests), []float64{1, 4}, []string{"0", "1-3", "4+"})
	// sweep bar anatomy
	b := m5[i]
	rngB := b.High - b.Low
	body := 0.0
	if rngB > 0 {
		body = math.Abs(b.Close-b.Open) / rngB
	}
	e.static["sweepbar_body"] = bin(body, []float64{0.3, 0.6}, []string{"<0.3 wick", "0.3-0.6", ">0.6 marubozu"})
	e.static["sweepbar_size"] = bin(rngB/atr, []float64{0.5, 1.0}, []string{"<0.5 ATR", "0.5-1 ATR", ">1 ATR"})
	vs := 0.0
	for j := i - 48; j < i; j++ {
		vs += m5[j].Volume
	}
	if vs > 0 {
		e.static["sweepbar_relvol"] = bin(b.Volume/(vs/48), []float64{1, 2}, []string{"<1x", "1-2x", ">2x"})
	}
	// HTF trend: last closed H1 vs EMA50 and EMA slope over 5 H1
	hi := h1.ofM5[i]
	if hi > 55 {
		up := h1.bars[hi].Close > h1.ema[hi] && h1.ema[hi] > h1.ema[hi-5]
		dn := h1.bars[hi].Close < h1.ema[hi] && h1.ema[hi] < h1.ema[hi-5]
		switch {
		case (up && dir > 0) || (dn && dir < 0):
			e.static["h1_trend"] = "WITH sweep (trend pushes through)"
		case (up && dir < 0) || (dn && dir > 0):
			e.static["h1_trend"] = "AGAINST sweep (sweep into trend)"
		default:
			e.static["h1_trend"] = "flat/mixed"
		}
	}
	h := t.Hour()
	e.static["session"] = map[bool]string{true: "ASIA 00-07"}[h < 7]
	switch {
	case h >= 7 && h < 12:
		e.static["session"] = "LONDON 07-12"
	case h >= 12 && h < 17:
		e.static["session"] = "NY 12-17"
	case h >= 17:
		e.static["session"] = "LATE 17-24"
	}
	e.static["level"] = map[string]string{"PDH": "PDH/PDL", "PDL": "PDH/PDL", "ASIA_H": "ASIA", "ASIA_L": "ASIA", "SWING_H": "SWING", "SWING_L": "SWING"}[L.name]
	// micro structure reference: extreme of the leg opposite the sweep (last 12 bars)
	opp := m5[i-1].Close
	for j := i - 12; j < i; j++ {
		opp = math.Min(dir*opp, dir*extreme(m5[j], !L.high)) * dir
	}
	for _, k := range checkpoints {
		c := i + k
		f := map[string]string{}
		closes, maxExc, inside := 0, 0.0, dir*(m5[c].Close-L.price) < 0
		fvg, mss, newExt := false, false, false
		for j := i; j <= c; j++ {
			if dir*(m5[j].Close-L.price) > 0 {
				closes++
			}
			maxExc = math.Max(maxExc, dir*(extreme(m5[j], L.high)-L.price)/atr)
			if dir*(m5[j].Close-opp) < 0 {
				mss = true
			}
			if j >= 2 && j-2 >= i-1 {
				// FVG pointing back inside (reversal displacement)
				if dir > 0 && m5[j-2].Low > m5[j].High || dir < 0 && m5[j-2].High < m5[j].Low {
					fvg = true
				}
			}
		}
		if k > 0 {
			prevExt := L.price
			for j := i; j < c; j++ {
				prevExt = dir * math.Max(dir*prevExt, dir*extreme(m5[j], L.high))
			}
			newExt = dir*(extreme(m5[c], L.high)-prevExt) > 0
		}
		f["close_back_inside"] = fmt.Sprint(inside)
		f["closes_beyond"] = bin(float64(closes), []float64{1, 2, 4}, []string{"0 (wick only)", "1", "2-3", "4+"})
		f["max_excursion"] = bin(maxExc, []float64{0.25, 0.6}, []string{"<0.25 ATR", "0.25-0.6", ">0.6 ATR"})
		f["dist_from_level"] = bin(dir*(m5[c].Close-L.price)/atr, []float64{-0.3, 0, 0.3}, []string{"inside >0.3", "inside 0-0.3", "beyond 0-0.3", "beyond >0.3"})
		f["rev_FVG_formed"] = fmt.Sprint(fvg)
		f["mss_close_thru_leg"] = fmt.Sprint(mss)
		if k > 0 {
			f["still_new_extreme"] = fmt.Sprint(newExt)
		}
		e.cp = append(e.cp, f)
		e.rev = append(e.rev, outcome(m5, c, dir, atr))
	}
	return e
}

// outcome from close c: 1 if price moves barrierATR against the sweep first.
func outcome(m5 []tvspoof.Bar, c int, dir, atr float64) int {
	p := m5[c].Close
	up, dn := p+barrierATR*atr, p-barrierATR*atr
	for j := c + 1; j <= c+horizon && j < len(m5); j++ {
		hitUp, hitDn := m5[j].High >= up, m5[j].Low <= dn
		if hitUp && hitDn {
			return -1
		}
		if hitUp {
			return map[bool]int{true: 0, false: 1}[dir > 0]
		}
		if hitDn {
			return map[bool]int{true: 1, false: 0}[dir > 0]
		}
	}
	return -1
}

func bin(v float64, cuts []float64, names []string) string {
	for k, c := range cuts {
		if v < c {
			return names[k]
		}
	}
	return names[len(names)-1]
}

// --- reporting ------------------------------------------------------------

type tally struct{ n, rev, n0, r0, n1, r1 int }

func (t *tally) add(e event, r int) {
	t.n++
	t.rev += r
	if e.half == 0 {
		t.n0++
		t.r0 += r
	} else {
		t.n1++
		t.r1 += r
	}
}

func pct(a, b int) string {
	if b == 0 {
		return "   -"
	}
	return fmt.Sprintf("%3.0f%%", 100*float64(a)/float64(b))
}

func report(ev []event) {
	fmt.Printf("\nOutcome: from the checkpoint close, REV = -%.1f ATR(M15) hit before +%.1f ATR (8h horizon). Coin flip = 50%%.\n", barrierATR, barrierATR)
	fmt.Println("Columns: n, P(reverse) all | first half | second half (stability check)")
	for ci, k := range checkpoints {
		var base tally
		groups := map[string]map[string]*tally{}
		for _, e := range ev {
			r := e.rev[ci]
			if r < 0 {
				continue
			}
			base.add(e, r)
			feats := map[string]string{}
			for a, v := range e.static {
				feats[a] = v
			}
			for a, v := range e.cp[ci] {
				feats[a] = v
			}
			feats["sym"] = e.sym
			for a, v := range feats {
				if groups[a] == nil {
					groups[a] = map[string]*tally{}
				}
				if groups[a][v] == nil {
					groups[a][v] = &tally{}
				}
				groups[a][v].add(e, r)
			}
			// the key interaction: back inside × H1 trend
			x := "inside=" + e.cp[ci]["close_back_inside"] + " | " + e.static["h1_trend"]
			if groups["X inside×trend"] == nil {
				groups["X inside×trend"] = map[string]*tally{}
			}
			if groups["X inside×trend"][x] == nil {
				groups["X inside×trend"][x] = &tally{}
			}
			groups["X inside×trend"][x].add(e, r)
			x = e.static["session"] + " | vol " + e.static["sweepbar_relvol"]
			if groups["X session×relvol"] == nil {
				groups["X session×relvol"] = map[string]*tally{}
			}
			if groups["X session×relvol"][x] == nil {
				groups["X session×relvol"][x] = &tally{}
			}
			groups["X session×relvol"][x].add(e, r)
		}
		fmt.Printf("\n══ CHECKPOINT k=%d (%s after sweep bar opens) ══  base n=%d P(rev)=%s | %s | %s\n",
			k, time.Duration(5*(k+1))*time.Minute, base.n, pct(base.rev, base.n), pct(base.r0, base.n0), pct(base.r1, base.n1))
		names := make([]string, 0, len(groups))
		for a := range groups {
			names = append(names, a)
		}
		sort.Strings(names)
		for _, a := range names {
			fmt.Printf("  %s\n", a)
			vals := make([]string, 0)
			for v := range groups[a] {
				vals = append(vals, v)
			}
			sort.Strings(vals)
			for _, v := range vals {
				t := groups[a][v]
				if t.n < 15 {
					continue
				}
				p := float64(t.rev) / float64(t.n)
				z := (p - 0.5) / math.Sqrt(0.25/float64(t.n))
				fmt.Printf("    %-45s n=%4d  %s | %s | %s  z=%+.1f\n", v, t.n, pct(t.rev, t.n), pct(t.r0, t.n0), pct(t.r1, t.n1), z)
			}
		}
	}
}
