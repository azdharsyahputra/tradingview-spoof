# tradingview-spoof

Real-time market data from TradingView's WebSocket — without getting blocked.

This Go library connects to TradingView's live data stream using **uTLS** to spoof a Chrome TLS fingerprint, bypassing Cloudflare's WAF and TLS fingerprint detection.

## Features

- **TLS Fingerprint Spoofing** — Uses [uTLS](https://github.com/refraction-networking/utls) with `HelloChrome_Auto` to mimic a real Chrome browser
- **Real-time Quotes** — Price, Volume, Bid, Ask, OHLC, Change, Change%
- **Historical OHLCV** — Recent candles through TradingView's chart session protocol
- **Live OHLCV bars** — Streaming updates for the active candle
- **ForexFactory Calendar** — Weekly economic events through the linked JSON export
- **Auto-reconnect** — Exponential backoff with automatic symbol re-subscription
- **Heartbeat Management** — Automatic ping/pong to keep the connection alive
- **Thread-safe** — All write operations are mutex-protected
- **Zero Configuration** — Works out of the box with sensible defaults
- **Functional Options** — Clean, extensible configuration pattern

## Installation

```bash
go get github.com/azdharsyahputra/tradingview-spoof
```

## Quick Start

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
            fmt.Printf("%s: %.2f\n", update.Symbol, *update.Price)
        }
    }

    client.OnError = func(err error) {
        log.Printf("Error: %v", err)
    }

    if err := client.Connect(); err != nil {
        log.Fatal(err)
    }

    client.AddSymbol("OANDA:XAUUSD")
    client.AddSymbol("BINANCE:BTCUSDT")
    client.AddSymbol("NASDAQ:AAPL")

    // Block forever
    select {}
}
```

## Configuration

Use functional options to customize the client:

```go
client := tvspoof.NewClient(
    tvspoof.WithAutoReconnect(true),
    tvspoof.WithReconnectDelay(5 * time.Second),
    tvspoof.WithMaxReconnectWait(2 * time.Minute),
    tvspoof.WithOrigin("https://id.tradingview.com"),
    tvspoof.WithQuoteFields([]string{"lp", "volume", "bid", "ask"}),
)
```

### Available Options

| Option | Default | Description |
| :--- | :--- | :--- |
| `WithAutoReconnect(bool)` | `true` | Auto-reconnect on disconnect |
| `WithReconnectDelay(duration)` | `3s` | Initial reconnect delay |
| `WithMaxReconnectWait(duration)` | `60s` | Max reconnect backoff |
| `WithOrigin(string)` | `https://www.tradingview.com` | HTTP Origin header |
| `WithUserAgent(string)` | Chrome 148 on Windows | User-Agent header |
| `WithQuoteFields([]string)` | All fields | Data fields to subscribe |

## QuoteUpdate Fields

| Field | TradingView Key | Description |
| :--- | :--- | :--- |
| `Price` | `lp` | Last traded price |
| `Volume` | `volume` | Current volume |
| `Bid` | `bid` | Best bid price |
| `Ask` | `ask` | Best ask price |
| `Open` | `open_price` | Session open price |
| `High` | `high_price` | Session high |
| `Low` | `low_price` | Session low |
| `PrevClose` | `prev_close_price` | Previous session close |
| `Change` | `ch` | Absolute price change |
| `ChangePercent` | `chp` | Percentage change |

> **Note:** TradingView sends partial updates. Fields that didn't change will be `nil`.

## Historical candles

`GetHistory` opens a separate short-lived chart session and returns normalized
OHLCV bars. It supports TradingView resolutions such as `1`, `15`, `60`, `240`,
`D`, `W`, and `M`:

```go
ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
defer cancel()

client := tvspoof.NewClient()
bars, err := client.GetHistory(ctx, "OANDA:XAUUSD", "15", 500)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("loaded %d candles; latest close %.2f\n", len(bars), bars[len(bars)-1].Close)
```

Historical requests use the same Chrome-fingerprint uTLS dialer as live
quotes, but do not reuse or interrupt the quote session.

## Local API + Next.js view

Run the local HTTP/WebSocket bridge:

```bash
go run ./cmd/server
```

Endpoints:

- `GET /api/history?symbol=OANDA:XAUUSD&interval=15&bars=500` — historical OHLCV
- `GET /api/calendar?date=today&currency=USD&limit=20` — today's cached ForexFactory economic calendar (`date` also accepts `YYYY-MM-DD`)
- `WS /ws/quotes?symbol=OANDA:XAUUSD` — realtime quote stream
- `WS /ws/bars?symbol=OANDA:XAUUSD&interval=15` — realtime OHLCV bar stream
- `GET /api/health` — health check

Then run the dashboard:

```bash
cd web
npm install
npm run dev
```

Open `http://localhost:3000`. The view loads historical candles through the
REST endpoint, updates the active candle from the bar WebSocket, and shows
related ForexFactory events for the current day in the market panel.

The `SOL` chart indicator marks BUY/SELL setups from a sweep and reclaim of
the prior 10-bar range or a strong close through that range. Its breakout
filter uses EMA 8/21 and ATR 14; it applies a seven-bar cooldown. Signals are
calculated only from completed candles. The drawer shows the signal's close
price, invalidation level, and a 1.5R reference, all in chart price units.
SOL is an experimental chart aid, not a guarantee that a trade will work.

## Symbol Format

Symbols use the `EXCHANGE:TICKER` format:

| Category | Examples |
| :--- | :--- |
| Forex | `OANDA:XAUUSD`, `FX:EURUSD`, `OANDA:GBPUSD` |
| Crypto | `BINANCE:BTCUSDT`, `COINBASE:ETHUSD`, `BITSTAMP:BTCUSD` |
| Stocks | `NASDAQ:AAPL`, `NYSE:TSLA`, `NASDAQ:NVDA` |
| Indices | `TVC:SPX`, `TVC:DXY`, `FOREXCOM:NAS100` |
| Commodities | `TVC:GOLD`, `TVC:SILVER`, `NYMEX:CL1!` |

## How It Works

```
┌─────────────┐     TCP + uTLS (Chrome)     ┌──────────────────┐
│  Your App   │ ──────────────────────────▶  │   TradingView    │
│             │     WSS (Engine.IO)          │   Data Server    │
│  OnQuote()  │ ◀──────────────────────────  │                  │
└─────────────┘    ~m~LEN~m~{JSON}           └──────────────────┘
```

1. **TCP Connection** — Raw TCP dial to `data.tradingview.com:443`
2. **TLS Handshake** — uTLS spoofs Chrome's ClientHello fingerprint
3. **WebSocket Upgrade** — Standard HTTP upgrade with browser-like headers
4. **Session Init** — Auth token → Create session → Set fields
5. **Data Stream** — Subscribe to symbols → Receive real-time quotes
6. **Heartbeat** — Auto-echo `~h~N` pings to stay alive

## License

MIT
