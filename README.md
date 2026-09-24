# 🏛️ ZTERM — Institutional Quant Trading Desk & TradingView Engine

High-performance market data engine, quantitative analytics suite, and **AI Hedge Fund Research Desk** in Go.

`zterm` connects directly to TradingView's WebSocket feed using **uTLS Chrome fingerprint spoofing** to bypass Cloudflare WAF/TLS fingerprint detection, while providing an institutional-grade quantitative analysis engine (Intermarket Macro Radar, Volume Profile VPVR, Session Liquidity Tracking, SMC, and Dynamic Risk Position Sizing).

---

## 🚀 Key Features

* **uTLS Chrome Fingerprint Spoofing** — Mimics real Chrome TLS handshakes (`HelloChrome_Auto`) for unblockable WebSocket & chart session connections.
* **🏛️ Executive Prop Desk Briefing (`cmd/desk`)** — Institutional-grade multi-asset quantitative briefing with macro radar, volume profile, liquidity maps, and actionable trade playbooks.
* **⚡ Live Terminal Monitor (`cmd/thick`)** — Pixel-perfect Box Drawing terminal dashboard streaming real-time ticks, bid/ask spreads, OHLCV candle tables, and sparklines.
* **🌐 Intermarket Macro Radar** — Parallel cross-asset feeds tracking US Dollar Index (`DXY`), nominal 10-Year Treasury Yield (`US10Y`), daily 10-Year TIPS real yield (`FRED:DFII10`, preferred for the gold rates regime), S&P 500 (`SPX`), and Silver (`XAGUSD`).
* **📊 Auction Theory & Volume Profile (VPVR)** — Real-time Point of Control (`POC`), Value Area High (`VAH`), Value Area Low (`VAL`), and volume distribution histograms.
* **🎯 Session Liquidity Tracker** — Automatic detection of Asian Range High/Low, London Open sweeps (*Judas Swings*), and Previous Day High/Low (`PDH`/`PDL`).
* **🧠 Smart Money Concepts (SMC)** — Automated Market Structure Shift (`MSS`), Fair Value Gaps (`FVG`), and Premium vs. Discount pricing zones.
* **🛡️ Quantitative Risk Engine** — Volatility-adjusted (ATR) Stop Loss, Kelly Criterion position sizing, Expected Value (`+EV`), and Conviction Scoring (`A+`, `A`, `B+`, `B`, `C`).
* **📅 Economic Calendar Integration** — Real-time cached ForexFactory high/medium impact catalyst calendar.
* **🔌 REST & WebSocket API Server (`cmd/server`)** — Fast HTTP/WebSocket bridge with Next.js web interface.

---

## 🛠️ CLI Tools & Executables ("Kaki Tangan AI Desk")

### 1. Institutional Desk Briefing (`cmd/desk`)
Generates an instant quantitative research briefing and trade playbook:

```bash
# Default briefing for Gold ($10,000 balance, 1% risk)
go run ./cmd/desk xauusd

# Customized portfolio balance and risk budget
go run ./cmd/desk xauusd 50000 1.5

# Multi-asset support (Crypto, Forex, Indices, Equities)
go run ./cmd/desk btc 25000 2.0
go run ./cmd/desk eurusd
```

### 2. Live Terminal Tick & OHLCV Monitor (`cmd/thick`)
Interactive live streaming terminal dashboard:

```bash
# Stream Gold on 15-minute timeframe
go run ./cmd/thick xauusd 15

# Stream Bitcoin on 5-minute timeframe
go run ./cmd/thick btc 5
```

### 3. Institutional Candlestick Feed (`cmd/candles`)
High-resolution OHLCV bar matrix with Fair Value Gaps (FVG), Market Structure Shift (MSS), Swing High/Low, and candlestick pattern classification:

```bash
# Dual-timeframe mandatory setup audit:
# M15 minimum 100 bars (macro/session structural narrative)
go run ./cmd/candles xauusd 15 100

# M5 minimum 300 bars (100 × 3 rasio M15/M5 - micro trigger, MSS & sniper SL)
go run ./cmd/candles xauusd 5 300
```

### 4. Institutional S/R & Supply/Demand Radar (`cmd/levels`)
Calculates today range, classic/dynamic pivot points, key reference benchmarks (PDH/PDL, Asia High/Low, POC, VAH/VAL), and order flow supply/demand zones:

```bash
go run ./cmd/levels xauusd
go run ./cmd/levels usoil
```

### 5. CFTC Commitments of Traders (COT) Smart Money Radar (`cmd/cot`)
Direct CFTC open data parser tracking Non-Commercial (Hedge Funds) vs Commercial (Hedgers) net positions and institutional bias:

```bash
go run ./cmd/cot gold
go run ./cmd/cot # Multi-asset core radar
```

### 6. REST & WebSocket API Server (`cmd/server`)
Launches the HTTP/WebSocket bridge on `http://127.0.0.1:8080`:

```bash
go run ./cmd/server
```

**Available Endpoints:**
* `GET /api/desk?symbol=OANDA:XAUUSD&balance=10000&risk=1.0` — Full JSON Desk Briefing.
* `GET /api/history?symbol=OANDA:XAUUSD&interval=15&bars=500` — Historical OHLCV candles.
* `GET /api/calendar?date=today&currency=USD&limit=20` — Cached ForexFactory economic calendar.
* `GET /api/symbols?q=gold` — Search TradingView symbols.
* `WS /ws/quotes?symbol=OANDA:XAUUSD` — Real-time quote stream (Bid, Ask, Spread, Change).
* `WS /ws/bars?symbol=OANDA:XAUUSD&interval=15` — Real-time active bar update stream.

---

## 💻 Go Library Usage

### Real-Time Quotes

```go
package main

import (
    "fmt"
    "log"

    tvspoof "github.com/azdharsyahputra/tradingview-spoof"
)

func main() {
    client := tvspoof.NewClient()

    client.OnQuote = func(update tvspoof.QuoteUpdate) {
        if update.Price != nil {
            fmt.Printf("%s: %.2f (Bid: %.2f / Ask: %.2f)\n", 
                update.Symbol, *update.Price, *update.Bid, *update.Ask)
        }
    }

    if err := client.Connect(); err != nil {
        log.Fatal(err)
    }

    client.AddSymbol("OANDA:XAUUSD")
    client.AddSymbol("BINANCE:BTCUSDT")

    select {} // Block
}
```

### Quantitative Desk Analysis

```go
package main

import (
    "context"
    "fmt"
    "time"

    tvspoof "github.com/azdharsyahputra/tradingview-spoof"
)

func main() {
    client := tvspoof.NewClient()
    ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
    defer cancel()

    // Analyze XAUUSD with $25,000 balance and 1.5% risk
    briefing, err := client.AnalyzeDesk(ctx, "OANDA:XAUUSD", 25000.0, 1.5)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Executive Bias: %s (Conviction: %d%% | Grade: %s)\n",
        briefing.ExecutiveBias, briefing.ConvictionScore, briefing.ConvictionGrade)
    fmt.Printf("Macro Regime: %s\n", briefing.Intermarket.MacroRegime)
    fmt.Printf("POC: %.2f | VAH: %.2f | VAL: %.2f\n", 
        briefing.VolumeProfile.POC, briefing.VolumeProfile.VAH, briefing.VolumeProfile.VAL)
    fmt.Printf("Playbook: Entry: %.2f | SL: %.2f | TP1: %.2f | TP2: %.2f | Size: %.2f Lots\n",
        briefing.Playbook.EntryPrice, briefing.Playbook.InvalidationPrice,
        briefing.Playbook.Target1Price, briefing.Playbook.Target2Price, briefing.Playbook.RecommendedLots)
}
```

---

## 🤖 AI Assistant Standard Operating Procedure (SOP)

When an AI coding agent or assistant (`agy` / Antigravity) opens this workspace, it acts as an **Institutional Prop Desk Quant**.

Whenever asked for market insights or price predictions:
1. **Execute Quantitative & Candlestick Suite**:
   * Runs `go run ./cmd/levels [symbol]` (Institutional S/R & Supply/Demand).
   * Runs `go run ./cmd/desk [symbol]` (Volume Profile, Macro Radar, SMC & Risk Sizing).
   * Runs `go run ./cmd/candles [symbol] 15 100` AND `go run ./cmd/candles [symbol] 5 300` (Mandatory Dual-TF Audit: M15 100 bars for macro structure, M5 300 bars [100 × 3] for micro trigger & sniper SL).
   * Runs `go run ./cmd/cot [symbol]` (CFTC Smart Money hedge fund positioning).
2. **Synthesize 4 Pillars**:
   * *Macro Backdrop* (DXY, Yields, Cross-Asset Tailwind/Headwind).
   * *Auction Mechanics* (Volume Profile POC, VAH, VAL).
   * *Session Liquidity* (Asia/London High/Low sweeps).
   * *SMC Zone & Morphology* (Discount/Premium, Fair Value Gaps, Wick Absorption, MSS).
3. **Formulate High-Conviction Playbook**: Provides exact Entry, Invalidation (SL), Targets (TP1/TP2), Risk-Reward Ratio (RRR), and CFD Micro-Position Sizing.

---

## 🌐 Next.js Web Dashboard

```bash
cd web
npm install
npm run dev
```
Open `http://localhost:3000` to access the interactive chart, CISD/SOL model visualizers, and market catalyst panels.

---

## 📜 License

MIT License.
