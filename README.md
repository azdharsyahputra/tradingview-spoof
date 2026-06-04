# tradingview-spoof

Real-time market data from TradingView's WebSocket — without getting blocked.

This Go library connects to TradingView's live data stream using **uTLS** to spoof a Chrome TLS fingerprint, bypassing Cloudflare's WAF and TLS fingerprint detection.

## Features

- **TLS Fingerprint Spoofing** — Uses [uTLS](https://github.com/refraction-networking/utls) with `HelloChrome_Auto` to mimic a real Chrome browser
- **Real-time Quotes** — Price, Volume, Bid, Ask, OHLC, Change, Change%
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
