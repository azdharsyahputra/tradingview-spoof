"use client";

import { useEffect, useMemo, useState } from "react";
import { calculateSolSignals, lastConfirmedSolIndex, type SolSignal } from "../lib/sol";
import { calculateCisdModel, type CisdModelResult, type CisdSetup } from "../lib/cisd";

type Bar = { time: number; open: number; high: number; low: number; close: number; volume?: number };
type Quote = { Symbol: string; Price?: number; Bid?: number; Ask?: number; Volume?: number; Open?: number; High?: number; Low?: number; ChangePercent?: number };
type SymbolResult = { Symbol: string; FullName: string; Description: string; Exchange: string; Type: string; CurrencyCode?: string };
type BarUpdate = { symbol: string; interval: string; bar: Bar };
type CalendarEvent = { title: string; country: string; date: string; timestamp: number; impact: string; actual?: string; forecast?: string; previous?: string; unit?: string; surprise?: number; fundamental_bias?: string };
type IndicatorKind = "cisd" | "sol" | "ema" | "sma" | "bb" | "rsi" | "macd" | "bos" | "choch" | "support" | "resistance";
type IndicatorConfig = { id: string; name: string; kind: IndicatorKind; period: number; enabled: boolean; color: string };
type StructureSignal = { index: number; pivotIndex: number; direction: "up" | "down"; kind: "BOS" | "CHoCH"; level: number };
type HorizontalLevel = { price: number; index: number; touches: number };
type DrawingLine = { id: number; startTime: number; startPrice: number; endTime: number; endPrice: number };
type ChartPoint = { time: number; price: number };

const API = process.env.NEXT_PUBLIC_API_BASE ?? "http://127.0.0.1:8080";
const WS = process.env.NEXT_PUBLIC_WS_BASE ?? "ws://127.0.0.1:8080";
const SYMBOLS = ["OANDA:XAUUSD", "TVC:GOLD", "FX:EURUSD", "FX:GBPUSD", "BINANCE:BTCUSDT", "NASDAQ:AAPL"];
const TIMEFRAMES = [{ value: "1", label: "1m" }, { value: "5", label: "5m" }, { value: "15", label: "15m" }, { value: "30", label: "30m" }, { value: "60", label: "1H" }, { value: "240", label: "4H" }, { value: "D", label: "1D" }];
const DEFAULT_INDICATORS: IndicatorConfig[] = [
  { id: "cisd", name: "CISD Model+", kind: "cisd", period: 0, enabled: true, color: "#2962FF" },
  { id: "sol", name: "SOL", kind: "sol", period: 0, enabled: false, color: "#f1c86b" },
  { id: "ema20", name: "EMA", kind: "ema", period: 20, enabled: false, color: "#f1c86b" },
  { id: "sma20", name: "SMA", kind: "sma", period: 20, enabled: false, color: "#72a7ff" },
  { id: "bb20", name: "Bollinger Bands", kind: "bb", period: 20, enabled: false, color: "#b58cff" },
  { id: "rsi14", name: "RSI", kind: "rsi", period: 14, enabled: false, color: "#f38ba8" },
  { id: "macd", name: "MACD", kind: "macd", period: 12, enabled: false, color: "#68d8ff" },
  { id: "bos", name: "Break of Structure", kind: "bos", period: 3, enabled: false, color: "#1ecb9b" },
  { id: "choch", name: "Change of Character", kind: "choch", period: 3, enabled: false, color: "#f1c86b" },
  { id: "support", name: "Support", kind: "support", period: 3, enabled: false, color: "#68d8ff" },
  { id: "resistance", name: "Resistance", kind: "resistance", period: 3, enabled: false, color: "#f45b75" },
];

function upsertLiveBar(current: Bar[], incoming: Bar): Bar[] {
  const index = current.findIndex((bar) => bar.time === incoming.time);
  if (index >= 0) return current.map((bar, position) => position === index ? incoming : bar);
  if (!current.length || incoming.time > current.at(-1)!.time) return [...current, incoming];
  return current;
}

function calendarCurrencies(symbol: string): string {
  const ticker = symbol.split(":").at(-1)?.replace(/[^A-Z]/g, "") ?? "";
  if (ticker === "XAUUSD" || ticker === "GOLD") return "USD";
  if (/^[A-Z]{6}$/.test(ticker)) {
    const quote = ticker.slice(3) === "USDT" ? "USD" : ticker.slice(3);
    return ticker.slice(0, 3) + "," + quote;
  }
  return "USD";
}

function simpleMovingAverage(values: number[], period: number): (number | null)[] {
  const result: (number | null)[] = Array(values.length).fill(null);
  for (let index = period - 1; index < values.length; index += 1) {
    let total = 0;
    for (let cursor = index - period + 1; cursor <= index; cursor += 1) total += values[cursor];
    result[index] = total / period;
  }
  return result;
}

function exponentialMovingAverage(values: number[], period: number): (number | null)[] {
  const result: (number | null)[] = Array(values.length).fill(null);
  if (values.length < period) return result;
  let previous = values.slice(0, period).reduce((total, value) => total + value, 0) / period;
  result[period - 1] = previous;
  const multiplier = 2 / (period + 1);
  for (let index = period; index < values.length; index += 1) {
    previous = (values[index] - previous) * multiplier + previous;
    result[index] = previous;
  }
  return result;
}

function relativeStrengthIndex(values: number[], period: number): (number | null)[] {
  const result: (number | null)[] = Array(values.length).fill(null);
  if (values.length <= period) return result;
  let gains = 0;
  let losses = 0;
  for (let index = 1; index <= period; index += 1) {
    const change = values[index] - values[index - 1];
    if (change >= 0) gains += change;
    else losses -= change;
  }
  let averageGain = gains / period;
  let averageLoss = losses / period;
  result[period] = averageLoss === 0 ? 100 : 100 - 100 / (1 + averageGain / averageLoss);
  for (let index = period + 1; index < values.length; index += 1) {
    const change = values[index] - values[index - 1];
    averageGain = (averageGain * (period - 1) + Math.max(0, change)) / period;
    averageLoss = (averageLoss * (period - 1) + Math.max(0, -change)) / period;
    result[index] = averageLoss === 0 ? 100 : 100 - 100 / (1 + averageGain / averageLoss);
  }
  return result;
}

function bollingerBands(values: number[], period: number): { middle: (number | null)[]; upper: (number | null)[]; lower: (number | null)[] } {
  const middle = simpleMovingAverage(values, period);
  const upper: (number | null)[] = Array(values.length).fill(null);
  const lower: (number | null)[] = Array(values.length).fill(null);
  for (let index = period - 1; index < values.length; index += 1) {
    const mean = middle[index] ?? 0;
    const variance = values.slice(index - period + 1, index + 1).reduce((total, value) => total + (value - mean) ** 2, 0) / period;
    const deviation = Math.sqrt(variance) * 2;
    upper[index] = mean + deviation;
    lower[index] = mean - deviation;
  }
  return { middle, upper, lower };
}

function macdSeries(values: number[]): { macd: (number | null)[]; signal: (number | null)[]; histogram: (number | null)[] } {
  const fast = exponentialMovingAverage(values, 12);
  const slow = exponentialMovingAverage(values, 26);
  const macd: (number | null)[] = values.map((_, index) => fast[index] != null && slow[index] != null ? fast[index]! - slow[index]! : null);
  const first = macd.findIndex((value) => value != null);
  const signal: (number | null)[] = Array(values.length).fill(null);
  if (first >= 0) {
    const signalValues = exponentialMovingAverage(macd.slice(first).map((value) => value ?? 0), 9);
    signalValues.forEach((value, index) => { signal[first + index] = value; });
  }
  const histogram = macd.map((value, index) => value != null && signal[index] != null ? value - signal[index]! : null);
  return { macd, signal, histogram };
}

function structureSignals(bars: Bar[], pivotPeriod: number): StructureSignal[] {
  const signals: StructureSignal[] = [];
  const period = Math.max(2, Math.min(20, pivotPeriod));
  let swingHigh: { index: number; value: number } | null = null;
  let swingLow: { index: number; value: number } | null = null;
  let brokenHigh = -1;
  let brokenLow = -1;
  let lastBreakDirection: "up" | "down" | null = null;

  const isPivotHigh = (index: number) => bars.slice(index - period, index + period + 1).every((bar, offset) => offset === period || bar.high <= bars[index].high);
  const isPivotLow = (index: number) => bars.slice(index - period, index + period + 1).every((bar, offset) => offset === period || bar.low >= bars[index].low);

  for (let index = period * 2; index < bars.length; index += 1) {
    const confirmedIndex = index - period;
    if (isPivotHigh(confirmedIndex)) swingHigh = { index: confirmedIndex, value: bars[confirmedIndex].high };
    if (isPivotLow(confirmedIndex)) swingLow = { index: confirmedIndex, value: bars[confirmedIndex].low };

    if (swingHigh && swingHigh.index !== brokenHigh && bars[index].close > swingHigh.value) {
      const direction: "up" = "up";
      signals.push({ index, pivotIndex: swingHigh.index, direction, kind: lastBreakDirection === "down" ? "CHoCH" : "BOS", level: swingHigh.value });
      brokenHigh = swingHigh.index;
      lastBreakDirection = direction;
    }
    if (swingLow && swingLow.index !== brokenLow && bars[index].close < swingLow.value) {
      const direction: "down" = "down";
      signals.push({ index, pivotIndex: swingLow.index, direction, kind: lastBreakDirection === "up" ? "CHoCH" : "BOS", level: swingLow.value });
      brokenLow = swingLow.index;
      lastBreakDirection = direction;
    }
  }
  return signals;
}

function horizontalLevels(bars: Bar[], kind: "support" | "resistance", pivotPeriod: number): HorizontalLevel[] {
  const period = Math.max(2, Math.min(20, pivotPeriod));
  const candidates: HorizontalLevel[] = [];
  const isHigh = kind === "resistance";
  const recent = bars.slice(-120);
  const averageRange = recent.length ? recent.reduce((total, bar) => total + bar.high - bar.low, 0) / recent.length : 0;
  const averagePrice = recent.length ? recent.reduce((total, bar) => total + bar.close, 0) / recent.length : 0;
  const tolerance = Math.max(averagePrice * 0.00035, averageRange * 0.7);

  for (let index = period; index < bars.length - period; index += 1) {
    const level = isHigh ? bars[index].high : bars[index].low;
    const isPivot = bars.slice(index - period, index + period + 1).every((bar, offset) => offset === period || (isHigh ? bar.high <= level : bar.low >= level));
    if (isPivot) candidates.unshift({ price: level, index, touches: 1 });
  }

  const levels: HorizontalLevel[] = [];
  for (const candidate of candidates.slice(0, 24)) {
    const existing = levels.find((level) => Math.abs(level.price - candidate.price) <= tolerance);
    if (existing) {
      existing.price = (existing.price * existing.touches + candidate.price) / (existing.touches + 1);
      existing.touches += 1;
      existing.index = Math.max(existing.index, candidate.index);
    } else {
      levels.push({ ...candidate });
    }
  }
  return levels.sort((left, right) => right.touches - left.touches || right.index - left.index).slice(0, 4);
}

function linePath(values: (number | null)[], xStep: number, valueToY: (value: number) => number): string {
  let path = "";
  let connected = false;
  values.forEach((value, index) => {
    if (value == null || !Number.isFinite(value)) {
      connected = false;
      return;
    }
    path += (connected ? "L" : "M") + (xStep * index + xStep / 2).toFixed(2) + " " + valueToY(value).toFixed(2) + " ";
    connected = true;
  });
  return path;
}

export default function Home() {
  const [symbol, setSymbol] = useState("OANDA:XAUUSD");
  const [symbolDraft, setSymbolDraft] = useState(symbol);
  const [interval, setInterval] = useState("15");
  const [bars, setBars] = useState<Bar[]>([]);
  const [quote, setQuote] = useState<Quote | null>(null);
  const [connection, setConnection] = useState<"loading" | "live" | "offline" | "error">("loading");
  const [error, setError] = useState("");
  const [reload, setReload] = useState(0);
  const [historyLimit, setHistoryLimit] = useState(500);
  const [historyLoading, setHistoryLoading] = useState(true);
  const [now, setNow] = useState(() => Date.now());
  const [searchResults, setSearchResults] = useState<SymbolResult[]>([]);
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchLoading, setSearchLoading] = useState(false);
  const [calendarEvents, setCalendarEvents] = useState<CalendarEvent[]>([]);
  const [calendarLoading, setCalendarLoading] = useState(true);
  const [indicatorOpen, setIndicatorOpen] = useState(false);
  const [indicators, setIndicators] = useState<IndicatorConfig[]>(() => DEFAULT_INDICATORS.map((indicator) => ({ ...indicator })));
  const [calendarOpen, setCalendarOpen] = useState(false);
  const [drawingMode, setDrawingMode] = useState(false);
  const [drawingLines, setDrawingLines] = useState<DrawingLine[]>([]);

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1_000);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    setHistoryLoading(true);
    setError("");
    fetch(API + "/api/history?symbol=" + encodeURIComponent(symbol) + "&interval=" + interval + "&bars=" + historyLimit, { signal: controller.signal })
      .then(async (response) => {
        const payload = await response.json();
        if (!response.ok || !payload.ok) throw new Error(payload.error ?? "History request failed");
        setBars(payload.bars);
        setHistoryLoading(false);
      })
      .catch((reason) => {
        if (reason?.name === "AbortError") return;
        setError(reason instanceof Error ? reason.message : "Unable to load historical data");
        setConnection("error");
        setHistoryLoading(false);
      });
    return () => controller.abort();
  }, [symbol, interval, reload, historyLimit]);

  useEffect(() => {
    setQuote(null);
    const socket = new WebSocket(WS + "/ws/quotes?symbol=" + encodeURIComponent(symbol));
    socket.onopen = () => setConnection("live");
    socket.onmessage = (event) => {
      const message = JSON.parse(event.data);
      if (message.type === "quote") { setQuote(message.data); setConnection("live"); }
      if (message.type === "error") { setError(message.error); setConnection("error"); }
    };
    socket.onerror = () => setConnection("error");
    socket.onclose = () => setConnection((state) => state === "error" ? "error" : "offline");
    return () => socket.close();
  }, [symbol]);

  useEffect(() => {
    const socket = new WebSocket(WS + "/ws/bars?symbol=" + encodeURIComponent(symbol) + "&interval=" + encodeURIComponent(interval));
    socket.onmessage = (event) => {
      const message = JSON.parse(event.data);
      if (message.type !== "bar") return;
      const update = message.data as BarUpdate;
      if (update.symbol !== symbol || update.interval !== interval) return;
      setBars((current) => upsertLiveBar(current, update.bar));
    };
    socket.onerror = () => setError("Live candle stream disconnected");
    return () => socket.close();
  }, [symbol, interval]);

  useEffect(() => {
    const query = symbolDraft.trim();
    if (query.length < 2) {
      setSearchResults([]);
      return;
    }
    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      setSearchLoading(true);
      fetch(API + "/api/symbols?q=" + encodeURIComponent(query) + "&limit=8", { signal: controller.signal })
        .then(async (response) => {
          const payload = await response.json();
          if (!response.ok || !payload.ok) throw new Error(payload.error ?? "Symbol search failed");
          setSearchResults(payload.data);
        })
        .catch((reason) => {
          if (reason?.name !== "AbortError") setSearchResults([]);
        })
        .finally(() => setSearchLoading(false));
    }, 180);
    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [symbolDraft]);

  useEffect(() => {
    const controller = new AbortController();
    setCalendarLoading(true);
    fetch(API + "/api/calendar?date=today&currency=" + encodeURIComponent(calendarCurrencies(symbol)), { signal: controller.signal })
      .then(async (response) => {
        const payload = await response.json();
        if (!payload.ok || !payload.ok) throw new Error(payload.error ?? "Calendar request failed");
        setCalendarEvents(payload.events ?? []);
      })
      .catch((reason) => {
        if (reason?.name !== "AbortError") setCalendarEvents([]);
      })
      .finally(() => setCalendarLoading(false));
    return () => controller.abort();
  }, [reload, symbol]);

  const displayBars = bars;
  const lastBar = displayBars.at(-1);
  const cisdEnabled = indicators.some((indicator) => indicator.kind === "cisd" && indicator.enabled);
  const cisdResult = useMemo(() => cisdEnabled ? calculateCisdModel(bars) : null, [bars, cisdEnabled]);
  const solEnabled = indicators.some((indicator) => indicator.kind === "sol" && indicator.enabled);
  const lastConfirmedIndex = lastConfirmedSolIndex(bars, interval, now);
  const solSignals = useMemo(() => solEnabled ? calculateSolSignals(bars, lastConfirmedIndex) : [], [bars, lastConfirmedIndex, solEnabled]);
  const latestSolSignal = solSignals.at(-1);
  const price = quote?.Price ?? lastBar?.close;
  const liveLabel = candleCountdown(interval, now);
  const tfLabel = TIMEFRAMES.find((item) => item.value === interval)?.label ?? interval;
  const upcomingCalendar = calendarEvents.filter((event) => event.timestamp >= Math.floor(now / 1_000) - 3_600).slice(0, 8);
  const applySymbol = () => {
    const next = symbolDraft.trim().toUpperCase();
    if (!next) return;
    setHistoryLimit(500);
    setSymbol(next);
    setSymbolDraft(next);
  };
  const selectTimeframe = (next: string) => {
    setHistoryLimit(500);
    setInterval(next);
  };
  const selectSymbol = (next: string) => {
    setHistoryLimit(500);
    setSymbol(next);
    setSymbolDraft(next);
    setSearchOpen(false);
  };
  const loadOlderHistory = () => {
    if (historyLoading || historyLimit >= 10_000) return;
    setHistoryLoading(true);
    setHistoryLimit((current) => Math.min(10_000, current + 500));
  };
  const updateIndicator = (id: string, update: Partial<IndicatorConfig>) => {
    setIndicators((current) => current.map((indicator) => indicator.id === id ? { ...indicator, ...update } : indicator));
  };
  const resetIndicators = () => setIndicators(DEFAULT_INDICATORS.map((indicator) => ({ ...indicator })));

  return <main className="terminal">
    <header className="topbar">
      <div className="brand"><span className="brand-mark">◈</span><span>vertex</span><em>terminal</em></div>
      <div className="symbol-picker"><span className="market-dot" /><input value={symbolDraft} onFocus={() => setSearchOpen(true)} onBlur={() => window.setTimeout(() => setSearchOpen(false), 120)} onChange={(event) => { setSymbolDraft(event.target.value.toUpperCase()); setSearchOpen(true); }} onKeyDown={(event) => { if (event.key === "Enter") applySymbol(); }} aria-label="Market symbol" /><button className="symbol-go" onClick={applySymbol}>↵</button>{searchOpen && symbolDraft.trim().length >= 2 && <div className="symbol-results">{searchLoading && <div className="search-state">Searching TradingView…</div>}{!searchLoading && searchResults.map((item) => <button key={item.FullName} onMouseDown={(event) => event.preventDefault()} onClick={() => selectSymbol(item.FullName)}><span className="search-icon">⌁</span><span className="search-copy"><b>{item.FullName}</b><small>{item.Description || item.Type}</small></span><span className="search-market">{item.Exchange}<em>{item.Type}</em></span></button>)}{!searchLoading && searchResults.length === 0 && <div className="search-state">No symbols found</div>}</div>}</div>
      <div className="tf-bar">{TIMEFRAMES.map((item) => <button key={item.value} className={interval === item.value ? "active" : ""} onClick={() => selectTimeframe(item.value)}>{item.label}</button>)}</div>
      <button className={"news-btn " + (calendarOpen ? "active" : "")} onClick={() => { setCalendarOpen((open) => !open); setIndicatorOpen(false); }} title="Economic Calendar & News"><span className="news-btn-icon">📅</span><span>News Calendar</span>{upcomingCalendar.some((event) => event.impact === "High") && <span className="news-alert-dot" />}</button>
      <div className="top-actions"><button title="Refresh chart" onClick={() => setReload((value) => value + 1)}>↻</button><div className={"connection " + connection}><i />{connection === "live" ? "Live" : connection}</div></div>
    </header>

    <section className="workspace">
      <aside className="tool-rail" aria-label="Chart tools">
        <button className={indicatorOpen ? "selected" : ""} title="Indicators" onClick={() => { setIndicatorOpen((open) => !open); setCalendarOpen(false); }}>⌁</button>
        <button className={calendarOpen ? "selected" : ""} title="Economic Calendar & News" onClick={() => { setCalendarOpen((open) => !open); setIndicatorOpen(false); }}>📅</button>
        <button className={drawingMode ? "selected" : ""} title="Trend line" onClick={() => { setDrawingMode((active) => !active); setIndicatorOpen(false); setCalendarOpen(false); }}>╱</button>
      </aside>
      <section className="chart-region">
        <div className="chart-toolbar">
          <div><span className="instrument-name">{symbol.replace(":", " · ")}</span><span className="instrument-meta"> · {tfLabel} · OANDA</span></div>
          <span className="gesture-help">drag to pan · scroll to zoom</span>
          <div className="ohlc-row">{lastBar && <><span>O <b>{lastBar.open.toFixed(3)}</b></span><span>H <b>{lastBar.high.toFixed(3)}</b></span><span>L <b>{lastBar.low.toFixed(3)}</b></span><span>C <b>{lastBar.close.toFixed(3)}</b></span><span className={lastBar.close >= lastBar.open ? "positive" : "negative"}>{((lastBar.close - lastBar.open) / lastBar.open * 100).toFixed(2)}%</span></>}</div>
        </div>
        <CandleChart bars={displayBars} indicators={indicators} solSignals={solSignals} cisdResult={cisdResult} drawings={drawingLines} drawingMode={drawingMode} onAddDrawing={(line) => setDrawingLines((current) => [...current, { ...line, id: Date.now() }])} currentPrice={lastBar?.close ?? price} interval={interval} liveLabel={liveLabel} onNeedOlderHistory={loadOlderHistory} hasMoreHistory={historyLimit < 10_000} historyLoading={historyLoading} />
        <footer className="chart-footer"><div><button>1D</button><button>5D</button><button>1M</button><button>3M</button><button>YTD</button><button>1Y</button><button>All</button></div><span>{new Intl.DateTimeFormat("en-GB", { hour: "2-digit", minute: "2-digit", second: "2-digit", timeZone: "UTC" }).format(new Date())} UTC</span></footer>
      </section>
    </section>
    {(indicatorOpen || calendarOpen) && <button className="indicator-scrim" aria-label="Close drawer" onClick={() => { setIndicatorOpen(false); setCalendarOpen(false); }} />}
    <IndicatorDrawer open={indicatorOpen} indicators={indicators} latestSolSignal={latestSolSignal} cisdResult={cisdResult} onToggle={(id, enabled) => updateIndicator(id, { enabled })} onPeriodChange={(id, period) => updateIndicator(id, { period })} onReset={resetIndicators} onClose={() => setIndicatorOpen(false)} />
    <CalendarDrawer open={calendarOpen} events={calendarEvents} loading={calendarLoading} symbol={symbol} onClose={() => setCalendarOpen(false)} />
    {error && <div className="terminal-error">{error}</div>}
  </main>;
}

function CalendarDrawer({ open, events, loading, symbol, onClose }: { open: boolean; events: CalendarEvent[]; loading: boolean; symbol: string; onClose: () => void }) {
  const [filterImpact, setFilterImpact] = useState<"ALL" | "HIGH">("ALL");
  const calendarLabel = calendarCurrencies(symbol).replace(",", " · ");
  const filteredEvents = events.filter((event) => filterImpact === "ALL" || event.impact === "High");

  return (
    <aside className={"indicator-drawer calendar-drawer " + (open ? "open" : "")} aria-hidden={!open}>
      <div className="indicator-drawer-head">
        <div>
          <b>Economic Calendar & News</b>
          <small>ForexFactory Live · {calendarLabel}</small>
        </div>
        <button onClick={onClose} aria-label="Close calendar">×</button>
      </div>
      <div className="calendar-filter-bar">
        <button className={filterImpact === "ALL" ? "active" : ""} onClick={() => setFilterImpact("ALL")}>All Impacts ({events.length})</button>
        <button className={filterImpact === "HIGH" ? "active" : ""} onClick={() => setFilterImpact("HIGH")}>🔥 High Impact ({events.filter((e) => e.impact === "High").length})</button>
      </div>
      <div className="calendar-drawer-list">
        {loading && <div className="calendar-state">Loading latest economic events…</div>}
        {!loading && filteredEvents.map((event) => (
          <div className="calendar-event" key={event.timestamp + event.title}>
            <div className="calendar-event-head">
              <i className={"impact-dot " + event.impact.toLowerCase()} />
              <b>{formatCalendarTime(event.timestamp)}</b>
              <span className="calendar-event-country">{event.country}</span>
              <span className={"calendar-impact-badge " + event.impact.toLowerCase()}>{event.impact}</span>
            </div>
            <strong>{event.title}</strong>
            <div className="calendar-event-metrics">
              {event.actual && <span>Act: <b>{event.actual}</b></span>}
              {event.forecast && <span>Fcst: <b>{event.forecast}</b></span>}
              {event.previous && <span>Prev: <b>{event.previous}</b></span>}
              {event.surprise !== undefined && <span className={event.surprise > 0 ? "positive" : "negative"}>Surprise: <b>{(event.surprise > 0 ? "+" : "") + event.surprise + (event.unit ?? "")}</b></span>}
            </div>
          </div>
        ))}
        {!loading && filteredEvents.length === 0 && <div className="calendar-state">No upcoming economic events found.</div>}
      </div>
    </aside>
  );
}

function IndicatorDrawer({ open, indicators, latestSolSignal, cisdResult, onToggle, onPeriodChange, onReset, onClose }: { open: boolean; indicators: IndicatorConfig[]; latestSolSignal?: SolSignal; cisdResult?: CisdModelResult | null; onToggle: (id: string, enabled: boolean) => void; onPeriodChange: (id: string, period: number) => void; onReset: () => void; onClose: () => void }) {
  return <aside className={"indicator-drawer " + (open ? "open" : "")} aria-hidden={!open}>
    <div className="indicator-drawer-head"><div><b>Indicators</b><small>Technical analysis</small></div><button onClick={onClose} aria-label="Close indicators">×</button></div>
    <div className="indicator-drawer-copy">Toggle an indicator to draw it on the chart. Periods update instantly.</div>
    <div className="indicator-items">{indicators.map((indicator) => <div className={"indicator-item " + (indicator.enabled ? "enabled" : "")} key={indicator.id}>
      <button className="indicator-toggle" onClick={() => onToggle(indicator.id, !indicator.enabled)}><i style={{ background: indicator.color }} /><span>{indicator.name}</span><em>{indicator.kind.toUpperCase()}</em><strong>{indicator.enabled ? "ON" : "OFF"}</strong></button>
      {!["cisd", "sol", "macd", "bos", "choch", "support", "resistance"].includes(indicator.kind) && <label>Period<input type="number" min="2" max="200" value={indicator.period} onChange={(event) => onPeriodChange(indicator.id, Math.max(2, Math.min(200, Number(event.target.value) || 2)))} /></label>}
      {indicator.kind === "cisd" && <>
        <small className="indicator-hint">Change in State of Delivery · ICT / SMC Strategy</small>
        {indicator.enabled && cisdResult && <div className="cisd-detail">
          <div className="cisd-detail-head">
            <b style={{ color: cisdResult.bias === "Bullish" ? "#2962FF" : cisdResult.bias === "Bearish" ? "#F23645" : "#787B86" }}>CISD {cisdResult.bias.toUpperCase()}</b>
            {cisdResult.activeSetup && <span className={`cisd-grade-badge ${cisdResult.activeSetup.gradeText === "A+" ? "a-plus" : cisdResult.activeSetup.gradeText === "A" ? "a" : cisdResult.activeSetup.gradeText === "B+" ? "b-plus" : cisdResult.activeSetup.gradeText === "B-" ? "b-minus" : cisdResult.activeSetup.gradeText === "C" ? "c" : "none"}`}>Grade {cisdResult.activeSetup.gradeText}</span>}
          </div>
          {cisdResult.activeSetup ? <>
            <div><span>Entry Level</span><b>{cisdResult.activeSetup.level.toFixed(3)}</b></div>
            <div><span>Stop Loss</span><b>{cisdResult.activeSetup.slPrice.toFixed(3)}</b></div>
            <div><span>Target (1.5R)</span><b>{cisdResult.activeSetup.target.toFixed(3)}</b></div>
            <div className="cisd-checklist">
              <div className="cisd-checklist-row"><span>Liquidity Sweep</span><b>{cisdResult.activeSetup.checklist.ckSweep ? "✅" : "❌"}</b></div>
              <div className="cisd-checklist-row"><span>HTF PDA Delivery</span><b>{cisdResult.activeSetup.checklist.ckPDA ? "✅" : "❌"}</b></div>
              <div className="cisd-checklist-row"><span>Volume Spike</span><b>{cisdResult.activeSetup.checklist.ckVol ? "✅" : "❌"}</b></div>
              <div className="cisd-checklist-row"><span>Session Sweep</span><b>{cisdResult.activeSetup.checklist.ckSession ? "✅" : "❌"}</b></div>
              <div className="cisd-checklist-row"><span>Clear Targets</span><b>{cisdResult.activeSetup.checklist.ckTargets ? "✅" : "❌"}</b></div>
              <div className="cisd-checklist-row"><span>SMT Alignment</span><b>{cisdResult.activeSetup.checklist.ckSMT ? "✅" : "❌"}</b></div>
            </div>
          </> : <small>No active confirmed CISD setup in recent bars.</small>}
        </div>}
      </>}
      {indicator.kind === "sol" && <><small className="indicator-hint">Confirmed candle · sweep or range break</small>{indicator.enabled && <div className="sol-detail">{latestSolSignal ? <><div className="sol-detail-title"><b className={latestSolSignal.side.toLowerCase()}>SOL {latestSolSignal.side}</b><span>{latestSolSignal.setup}</span></div><small>{new Date(latestSolSignal.time * 1_000).toISOString().replace("T", " ").slice(0, 16)} UTC · last confirmed signal</small><div><span>Signal close</span><b>{latestSolSignal.entry.toFixed(3)}</b></div><div><span>Invalidation</span><b>{latestSolSignal.invalidation.toFixed(3)}</b></div><div><span>1.5R reference</span><b>{latestSolSignal.target.toFixed(3)}</b></div></> : <small>No confirmed signal in loaded history.</small>}</div>}</>}
      {indicator.kind === "macd" && <small className="indicator-hint">12 / 26 / 9 · oscillator pane</small>}
      {indicator.kind === "rsi" && <small className="indicator-hint">0–100 · oscillator pane</small>}
      {(indicator.kind === "bos" || indicator.kind === "choch") && <small className="indicator-hint">Auto · confirmed swing break</small>}
      {(indicator.kind === "support" || indicator.kind === "resistance") && <small className="indicator-hint">Auto · merged swing levels</small>}
    </div>)}</div>
    <button className="indicator-reset" onClick={onReset}>Reset indicators</button>
  </aside>;
}

function CandleChart({ bars, indicators, solSignals, cisdResult, drawings, drawingMode, onAddDrawing, currentPrice, interval, liveLabel, onNeedOlderHistory, hasMoreHistory, historyLoading }: { bars: Bar[]; indicators: IndicatorConfig[]; solSignals: SolSignal[]; cisdResult?: CisdModelResult | null; drawings: DrawingLine[]; drawingMode: boolean; onAddDrawing: (line: Omit<DrawingLine, "id">) => void; currentPrice?: number; interval: string; liveLabel: string; onNeedOlderHistory: () => void; hasMoreHistory: boolean; historyLoading: boolean }) {
  const [hover, setHover] = useState<{ bar: Bar; x: number; y: number } | null>(null);
  const [visibleCount, setVisibleCount] = useState(65);
  const [pan, setPan] = useState(0);
  const [drag, setDrag] = useState<{ startX: number; startPan: number } | null>(null);
  const [drawingStart, setDrawingStart] = useState<ChartPoint | null>(null);
  const [drawingPreview, setDrawingPreview] = useState<ChartPoint | null>(null);
  if (!bars.length) return <div className="chart-loading"><span className="spinner" />Loading market history…</div>;

  const count = Math.max(10, Math.min(visibleCount, bars.length));
  const maxHistoryOffset = Math.max(0, bars.length - count);
  const maxFutureGap = Math.min(60, Math.max(10, Math.floor(count * 0.5)));
  const clampedPan = Math.max(-maxFutureGap, Math.min(maxHistoryOffset, pan));
  const historyOffset = Math.max(0, clampedPan);
  const futureGap = Math.max(0, -clampedPan);
  const dataSlotCount = Math.max(2, count - futureGap);
  const visibleEnd = bars.length - historyOffset;
  const visibleStart = Math.max(0, visibleEnd - dataSlotCount);
  const visible = bars.slice(visibleStart, visibleEnd);
  const activeIndicators = indicators.filter((indicator) => indicator.enabled);
  const priceIndicators = activeIndicators.filter((indicator) => indicator.kind === "ema" || indicator.kind === "sma" || indicator.kind === "bb");
  const paneIndicators = activeIndicators.filter((indicator) => indicator.kind === "rsi" || indicator.kind === "macd");
  const structureIndicators = activeIndicators.filter((indicator) => indicator.kind === "bos" || indicator.kind === "choch");
  const levelIndicators = activeIndicators.filter((indicator) => indicator.kind === "support" || indicator.kind === "resistance");
  const closes = bars.map((bar) => bar.close);
  const visibleSeries = (series: (number | null)[]) => series.slice(visibleStart, visibleEnd);
  const live = currentPrice ?? visible.at(-1)?.close ?? 0;
  const currentBar = bars.at(-1);
  const liveRising = (currentBar?.close ?? live) >= (currentBar?.open ?? live);
  const rawMin = Math.min(...visible.map((bar) => Math.min(bar.low, live)));
  const rawMax = Math.max(...visible.map((bar) => Math.max(bar.high, live)));
  const buffer = (rawMax - rawMin || 1) * 0.035;
  const min = rawMin - buffer;
  const max = rawMax + buffer;
  const range = max - min;
  const plotWidth = 1240;
  const axisWidth = 132;
  const plotHeight = 680;
  const paneRowHeight = 124;
  const paneHeight = paneIndicators.length * paneRowHeight;
  const footer = 46;
  const height = plotHeight + paneHeight + footer;
  const step = plotWidth / count;
  const y = (value: number) => 16 + ((max - value) / range) * (plotHeight - 32);
  const priceTicks = Array.from({ length: 9 }, (_, index) => max - (range * index) / 8);
  const xTicks = visible.filter((_, index) => index % Math.max(1, Math.floor(visible.length / 7)) === 0);
  const formatPrice = (value: number) => value.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 3 });
  const formatTime = (value: number) => new Intl.DateTimeFormat("en-GB", { hour: "2-digit", minute: "2-digit", timeZone: "UTC" }).format(new Date(value * 1000));
  const liveY = y(live);
  const pointFromEvent = (event: { clientX: number; clientY: number; currentTarget: SVGSVGElement }): ChartPoint | null => {
    const bounds = event.currentTarget.getBoundingClientRect();
    const chartX = (event.clientX - bounds.left) / bounds.width * (plotWidth + axisWidth);
    const index = Math.floor(chartX / step);
    if (index < 0 || index >= visible.length) return null;
    const svgY = (event.clientY - bounds.top) / bounds.height * height;
    const price = max - ((svgY - 16) / (plotHeight - 32)) * range;
    return { time: visible[index].time, price };
  };
  const xForTime = (time: number): number => {
    const index = visible.findIndex((bar) => bar.time === time);
    if (index >= 0) return step * index + step / 2;
    const intervalSec = bars.length >= 2 ? bars[1].time - bars[0].time : 60;
    if (visible.length && time < visible[0].time) {
      const diffBars = Math.round((visible[0].time - time) / Math.max(1, intervalSec));
      return Math.max(-500, -diffBars * step + step / 2);
    }
    if (visible.length && time > visible.at(-1)!.time) {
      const diffBars = Math.round((time - visible.at(-1)!.time) / Math.max(1, intervalSec));
      return (visible.length - 1 + diffBars) * step + step / 2;
    }
    return 0;
  };
  const overlaySeries = priceIndicators.filter((indicator) => indicator.kind !== "bb").map((indicator) => {
    if (indicator.kind === "ema") return { indicator, values: visibleSeries(exponentialMovingAverage(closes, indicator.period)) };
    return { indicator, values: visibleSeries(simpleMovingAverage(closes, indicator.period)) };
  });
  const bbSeries = priceIndicators.filter((indicator) => indicator.kind === "bb").map((indicator) => ({ indicator, bands: bollingerBands(closes, indicator.period) }));
  const macd = macdSeries(closes);
  const structureEvents = structureIndicators.flatMap((indicator) => structureSignals(bars, indicator.period).filter((signal) => signal.kind.toLowerCase() === indicator.kind));
  const horizontalLevelSets = levelIndicators.map((indicator) => ({ indicator, levels: horizontalLevels(bars, indicator.kind as "support" | "resistance", indicator.period) }));

  const zoom = (direction: 1 | -1) => {
    const nextCount = Math.max(10, Math.min(bars.length, visibleCount + direction * 8));
    const nextMaxFuture = Math.min(60, Math.max(10, Math.floor(nextCount * 0.5)));
    const nextMaxHistory = Math.max(0, bars.length - nextCount);
    setVisibleCount(nextCount);
    setPan((current) => Math.max(-nextMaxFuture, Math.min(nextMaxHistory, current)));
  };

  return <svg className={drawingMode ? "candle-chart drawing-active" : "candle-chart"} style={{ userSelect: "none", WebkitUserSelect: "none" }} onMouseDown={(e) => e.preventDefault()} viewBox={"0 0 " + (plotWidth + axisWidth) + " " + height} preserveAspectRatio="none" role="img" aria-label="Interactive OHLC candlestick chart" onWheel={(event) => {
    event.preventDefault();
    if (Math.abs(event.deltaX) > Math.abs(event.deltaY) || event.shiftKey) {
      const rawDelta = event.shiftKey ? event.deltaY : event.deltaX;
      const barDelta = Math.round(rawDelta / 14);
      if (barDelta !== 0) {
        const nextPan = Math.max(-maxFutureGap, Math.min(maxHistoryOffset, pan + barDelta));
        setPan(nextPan);
        if (hasMoreHistory && !historyLoading && nextPan >= Math.max(0, maxHistoryOffset - 8)) {
          onNeedOlderHistory();
        }
      }
    } else {
      zoom(event.deltaY > 0 ? 1 : -1);
    }
  }} onPointerLeave={() => { setHover(null); }} onPointerDown={(event) => {
    event.currentTarget.setPointerCapture(event.pointerId);
    if (drawingMode) {
      const point = pointFromEvent(event);
      if (point) {
        setDrawingStart(point);
        setDrawingPreview(point);
      }
      setDrag(null);
      return;
    }
    setDrag({ startX: event.clientX, startPan: clampedPan });
  }} onPointerUp={(event) => {
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
    if (drawingStart && drawingPreview) {
      onAddDrawing({ startTime: drawingStart.time, startPrice: drawingStart.price, endTime: drawingPreview.time, endPrice: drawingPreview.price });
      setDrawingStart(null);
      setDrawingPreview(null);
    }
    setDrag(null);
  }} onPointerCancel={() => {
    setDrag(null);
    setDrawingStart(null);
    setDrawingPreview(null);
  }} onPointerMove={(event) => {
    if (drawingMode && drawingStart) {
      const point = pointFromEvent(event);
      if (point) setDrawingPreview(point);
      setHover(null);
      return;
    }
    if (drag) {
      const bounds = event.currentTarget.getBoundingClientRect();
      const screenBarWidth = bounds.width > 0 ? (bounds.width / (plotWidth + axisWidth)) * step : step;
      const deltaBars = Math.round((event.clientX - drag.startX) / Math.max(1, screenBarWidth));
      const nextPan = Math.max(-maxFutureGap, Math.min(maxHistoryOffset, drag.startPan + deltaBars));
      setPan(nextPan);
      if (hasMoreHistory && !historyLoading && nextPan >= Math.max(0, maxHistoryOffset - 8)) onNeedOlderHistory();
      setHover(null);
      return;
    }
    const bounds = event.currentTarget.getBoundingClientRect();
    const x = (event.clientX - bounds.left) / bounds.width * (plotWidth + axisWidth);
    const index = Math.max(0, Math.min(visible.length - 1, Math.floor(x / step)));
    const bar = visible[index];
    if (bar) setHover({ bar, x: step * index + step / 2, y: event.nativeEvent.offsetY / bounds.height * height });
  }}>
    <defs><clipPath id="chart-clip"><rect x="0" y="0" width={plotWidth} height={plotHeight} /></clipPath><linearGradient id="chart-bg" x1="0" x2="0" y1="0" y2="1"><stop stopColor="#0e1520" /><stop offset="1" stopColor="#0a1018" /></linearGradient></defs>
    <rect width={plotWidth} height={plotHeight} fill="url(#chart-bg)" />
    {paneHeight > 0 && <rect x="0" y={plotHeight} width={plotWidth} height={paneHeight} className="indicator-pane-bg" />}
    <g className="chart-grid">{priceTicks.map((tick) => <g key={tick}><line x1="0" x2={plotWidth} y1={y(tick)} y2={y(tick)} /><text x={plotWidth + 16} y={y(tick) + 4}>{formatPrice(tick)}</text></g>)}{xTicks.map((bar) => { const index = visible.indexOf(bar); const x = step * index + step / 2; return <line key={bar.time} x1={x} x2={x} y1="0" y2={plotHeight} />; })}</g>
    <g clipPath="url(#chart-clip)">{visible.map((bar, index) => { const x = step * index + step / 2; const rising = bar.close >= bar.open; const bodyTop = Math.min(y(bar.open), y(bar.close)); const bodyHeight = Math.max(2, Math.abs(y(bar.open) - y(bar.close))); const candleWidth = Math.max(3.5, step * 0.72); return <g key={bar.time} className="candle"><line x1={x} x2={x} y1={y(bar.high)} y2={y(bar.low)} className={rising ? "up" : "down"} /><rect x={x - candleWidth / 2} y={bodyTop} width={candleWidth} height={bodyHeight} rx={1} className={rising ? "up-fill" : "down-fill"} /></g>; })}{overlaySeries.map(({ indicator, values }) => <path key={indicator.id} d={linePath(values, step, y)} className="indicator-line" style={{ stroke: indicator.color }} />)}{bbSeries.map(({ indicator, bands }) => <g key={indicator.id} className="bollinger-lines"><path d={linePath(visibleSeries(bands.upper), step, y)} style={{ stroke: indicator.color }} /><path d={linePath(visibleSeries(bands.lower), step, y)} style={{ stroke: indicator.color }} /><path d={linePath(visibleSeries(bands.middle), step, y)} style={{ stroke: indicator.color }} /></g>)}</g>
    {horizontalLevelSets.flatMap(({ indicator, levels }) => levels.map((level) => ({ indicator, level }))).map(({ indicator, level }) => { const levelY = y(level.price); if (levelY < 0 || levelY > plotHeight) return null; return <g key={indicator.id + level.index} className="horizontal-level" style={{ color: indicator.color }}><line x1="0" x2={plotWidth} y1={levelY} y2={levelY} /><text x={plotWidth - 8} y={levelY + 4}>{indicator.kind === "support" ? "S" : "R"} {formatPrice(level.price)} · {level.touches}×</text></g>; })}
    <g className="drawing-lines">{drawings.map((line) => <line key={line.id} x1={xForTime(line.startTime)} y1={y(line.startPrice)} x2={xForTime(line.endTime)} y2={y(line.endPrice)} />)}{drawingStart && drawingPreview && <line className="drawing-preview" x1={xForTime(drawingStart.time)} y1={y(drawingStart.price)} x2={xForTime(drawingPreview.time)} y2={y(drawingPreview.price)} />}</g>
    {structureEvents.filter((signal) => signal.index >= visibleStart && signal.index < visibleEnd).map((signal) => { const breakX = step * (signal.index - visibleStart) + step / 2; const pivotX = step * Math.max(0, signal.pivotIndex - visibleStart) + step / 2; const levelY = y(signal.level); if (levelY < 0 || levelY > plotHeight) return null; return <g key={signal.kind + signal.index} className={signal.direction === "up" ? "structure-up" : "structure-down"}><line x1={pivotX} x2={breakX} y1={levelY} y2={levelY} /><circle cx={breakX} cy={levelY} r="3" /><text x={breakX + 6} y={levelY + (signal.direction === "up" ? -7 : 14)}>{signal.kind}</text></g>; })}
    <g className="sol-signal-markers">{solSignals.filter((signal) => signal.index >= visibleStart && signal.index < visibleEnd).map((signal) => { const bar = bars[signal.index]; const x = step * (signal.index - visibleStart) + step / 2; const buy = signal.side === "BUY"; const wickY = y(buy ? bar.low : bar.high); const markerY = Math.max(22, Math.min(plotHeight - 22, wickY + (buy ? 26 : -26))); return <g key={signal.time} className={buy ? "sol-buy" : "sol-sell"} transform={`translate(${x},${markerY})`}><title>{`SOL ${signal.side} · ${signal.setup} · close ${formatPrice(signal.entry)} · invalidation ${formatPrice(signal.invalidation)}`}</title><line x1="0" y1={buy ? -14 : 14} x2="0" y2={wickY - markerY} /><rect x="-31" y="-11" width="62" height="22" rx="5" /><text x="0" y="4">SOL {signal.side}</text></g>; })}</g>
    
    {cisdResult && <g className="cisd-layers">
      {cisdResult.sessionLevels.map((lvl, idx) => {
        const lvlY = y(lvl.price);
        if (lvlY < 0 || lvlY > plotHeight) return null;
        return <g key={`sess-${idx}`} className="cisd-session"><line x1="0" x2={plotWidth} y1={lvlY} y2={lvlY} /><text x={plotWidth - 10} y={lvlY - 4} textAnchor="end">{lvl.name} {formatPrice(lvl.price)}</text></g>;
      })}
      {cisdResult.sweeps.filter((s) => s.index >= visibleStart && s.index < visibleEnd).map((s, idx) => {
        const x = step * (s.index - visibleStart) + step / 2;
        const rawY = y(s.price);
        if (rawY < 0 || rawY > plotHeight) return null;
        const sweepY = rawY + (s.buyside ? -10 : 18);
        return <g key={`swp-${idx}`} className="cisd-sweep"><text x={x} y={sweepY} textAnchor="middle">$$$</text></g>;
      })}
      {cisdResult.setups.filter((s) => s.endBar >= visibleStart - 20).map((s) => {
        const isLatest = cisdResult.activeSetup?.id === s.id;
        const x1 = Math.max(0, step * Math.max(0, s.startBar - visibleStart) + step / 2);
        const x2 = isLatest ? plotWidth : Math.min(plotWidth, step * Math.min(visible.length - 1, s.endBar - visibleStart + 16) + step / 2);
        const levelY = y(s.level);
        const slY = y(s.slPrice);
        const tpY = y(s.target);
        if (levelY < 0 || levelY > plotHeight) return null;
        return <g key={s.id} className={s.bull ? "cisd-level-bull" : "cisd-level-bear"}>
          <line x1={x1} x2={x2} y1={levelY} y2={levelY} />
          <text x={isLatest ? plotWidth - 10 : Math.min(plotWidth - 10, x2 + 6)} y={levelY - 4} textAnchor={isLatest ? "end" : "start"}>{s.bull ? "🟢 OPEN BUY (CISD+)" : "🔴 OPEN SELL (CISD-)"} [{s.gradeText}] {formatPrice(s.level)}</text>
          {!s.invalidated && slY >= 0 && slY <= plotHeight && <g className="cisd-sl"><line x1={x1} x2={x2} y1={slY} y2={slY} /><text x={isLatest ? plotWidth - 10 : Math.min(plotWidth - 10, x2 + 6)} y={slY - 4} textAnchor={isLatest ? "end" : "start"}>SL {formatPrice(s.slPrice)}</text></g>}
          {!s.invalidated && tpY >= 0 && tpY <= plotHeight && <g className="cisd-tp"><line x1={x1} x2={x2} y1={tpY} y2={tpY} /><text x={isLatest ? plotWidth - 10 : Math.min(plotWidth - 10, x2 + 6)} y={tpY - 4} textAnchor={isLatest ? "end" : "start"}>TP 1.5R {formatPrice(s.target)}</text></g>}
        </g>;
      })}
      {cisdResult.potentialBull && y(cisdResult.potentialBull.level) >= 0 && y(cisdResult.potentialBull.level) <= plotHeight && <g className="cisd-potential"><line x1={Math.max(0, step * Math.max(0, cisdResult.potentialBull.startBar - visibleStart) + step / 2)} x2={plotWidth} y1={y(cisdResult.potentialBull.level)} y2={y(cisdResult.potentialBull.level)} /><text x={plotWidth - 10} y={y(cisdResult.potentialBull.level) - 6} textAnchor="end">⚡ POTENTIAL BUY: Close above {formatPrice(cisdResult.potentialBull.level)}</text></g>}
      {cisdResult.potentialBear && y(cisdResult.potentialBear.level) >= 0 && y(cisdResult.potentialBear.level) <= plotHeight && <g className="cisd-potential"><line x1={Math.max(0, step * Math.max(0, cisdResult.potentialBear.startBar - visibleStart) + step / 2)} x2={plotWidth} y1={y(cisdResult.potentialBear.level)} y2={y(cisdResult.potentialBear.level)} /><text x={plotWidth - 10} y={y(cisdResult.potentialBear.level) + 14} textAnchor="end">⚡ POTENTIAL SELL: Close below {formatPrice(cisdResult.potentialBear.level)}</text></g>}
      {cisdResult.activeSetup && <g className="cisd-chart-checklist" transform="translate(56, 16)"><rect width="195" height="180" rx="6" fill="#0c1622" stroke="#253a50" fillOpacity="0.9" /><text x="12" y="20" fill="#e0e3eb" fontSize="11" fontWeight="700">CISD Signal & Checklist</text><text x="12" y="38" fill="#8ca0b5" fontSize="10">Action: <tspan fill={cisdResult.activeSetup.bull ? "#1ecb9b" : "#f45b75"} fontWeight="800">{cisdResult.activeSetup.bull ? "OPEN BUY" : "OPEN SELL"}</tspan></text><text x="125" y="38" fill="#8ca0b5" fontSize="10">Grade: <tspan fill="#00C853" fontWeight="800">{cisdResult.activeSetup.gradeText}</tspan></text><text x="12" y="54" fill="#8ca0b5" fontSize="9">Entry: <tspan fill="#fff" fontWeight="700">{formatPrice(cisdResult.activeSetup.level)}</tspan> | SL: <tspan fill="#ff7890">{formatPrice(cisdResult.activeSetup.slPrice)}</tspan></text><line x1="8" x2="187" y1="62" y2="62" stroke="#223547" /><text x="12" y="78" fill="#9cb1c5" fontSize="9">Liquidity Sweep</text><text x="170" y="78" fill="#fff" fontSize="9">{cisdResult.activeSetup.checklist.ckSweep ? "✅" : "❌"}</text><text x="12" y="94" fill="#9cb1c5" fontSize="9">HTF PDA Delivery</text><text x="170" y="94" fill="#fff" fontSize="9">{cisdResult.activeSetup.checklist.ckPDA ? "✅" : "❌"}</text><text x="12" y="110" fill="#9cb1c5" fontSize="9">Volume Spike</text><text x="170" y="110" fill="#fff" fontSize="9">{cisdResult.activeSetup.checklist.ckVol ? "✅" : "❌"}</text><text x="12" y="126" fill="#9cb1c5" fontSize="9">Session Sweep</text><text x="170" y="126" fill="#fff" fontSize="9">{cisdResult.activeSetup.checklist.ckSession ? "✅" : "❌"}</text><text x="12" y="142" fill="#9cb1c5" fontSize="9">Clear Targets (1.5R)</text><text x="170" y="142" fill="#fff" fontSize="9">{cisdResult.activeSetup.checklist.ckTargets ? "✅" : "❌"}</text><text x="12" y="158" fill="#9cb1c5" fontSize="9">SMT Alignment</text><text x="170" y="158" fill="#fff" fontSize="9">{cisdResult.activeSetup.checklist.ckSMT ? "✅" : "❌"}</text></g>}
    </g>}

    <g className={"live-line " + (liveRising ? "rising" : "falling")}><line x1="0" x2={plotWidth} y1={liveY} y2={liveY} /><rect x={plotWidth + 5} y={liveY - 18} width={axisWidth - 11} height="36" rx="4" /><text x={plotWidth + 15} y={liveY - 2}>{formatPrice(live)}</text><text x={plotWidth + 15} y={liveY + 12}>{liveLabel}</text></g>
    {paneIndicators.map((indicator, paneIndex) => {
      const paneTop = plotHeight + paneIndex * paneRowHeight;
      if (indicator.kind === "rsi") {
        const values = visibleSeries(relativeStrengthIndex(closes, indicator.period));
        const paneY = (value: number) => paneTop + 18 + ((100 - value) / 100) * (paneRowHeight - 30);
        return <g key={indicator.id} className="indicator-pane"><line x1="0" x2={plotWidth} y1={paneTop} y2={paneTop} className="pane-divider" /><line x1="0" x2={plotWidth} y1={paneY(70)} y2={paneY(70)} className="pane-guide" /><line x1="0" x2={plotWidth} y1={paneY(30)} y2={paneY(30)} className="pane-guide" /><path d={linePath(values, step, paneY)} style={{ stroke: indicator.color }} /><text x="12" y={paneTop + 18}>RSI {indicator.period}</text><text x={plotWidth + 16} y={paneY(70) + 4}>70</text><text x={plotWidth + 16} y={paneY(30) + 4}>30</text></g>;
      }
      const macdValues = visibleSeries(macd.macd);
      const signalValues = visibleSeries(macd.signal);
      const histogramValues = visibleSeries(macd.histogram);
      const valid = [...macdValues, ...signalValues, ...histogramValues].filter((value): value is number => value != null && Number.isFinite(value));
      const paneMin = Math.min(0, ...(valid.length ? valid : [0]));
      const paneMax = Math.max(0, ...(valid.length ? valid : [1]));
      const paneRange = paneMax - paneMin || 1;
      const paneY = (value: number) => paneTop + 18 + ((paneMax - value) / paneRange) * (paneRowHeight - 30);
      return <g key={indicator.id} className="indicator-pane"><line x1="0" x2={plotWidth} y1={paneTop} y2={paneTop} className="pane-divider" /><line x1="0" x2={plotWidth} y1={paneY(0)} y2={paneY(0)} className="pane-guide" />{histogramValues.map((value, index) => value == null ? null : <rect key={index} x={step * index + step * 0.2} y={Math.min(paneY(0), paneY(value))} width={Math.max(1, step * 0.6)} height={Math.max(1, Math.abs(paneY(value) - paneY(0)))} className={value >= 0 ? "macd-positive" : "macd-negative"} />)}<path d={linePath(macdValues, step, paneY)} className="macd-line" /><path d={linePath(signalValues, step, paneY)} className="macd-signal" /><text x="12" y={paneTop + 18}>MACD 12 / 26 / 9</text></g>;
    })}
    {historyLoading && <g className="history-loading"><rect x={plotWidth - 152} y="14" width="136" height="27" rx="5" /><text x={plotWidth - 84} y="32">loading history…</text></g>}
    <g className="zoom-control"><g onPointerDown={(event) => event.stopPropagation()} onClick={() => zoom(-1)}><rect x="14" y="14" width="30" height="30" rx="5" /><text x="29" y="35">+</text></g><g onPointerDown={(event) => event.stopPropagation()} onClick={() => zoom(1)}><rect x="14" y="48" width="30" height="30" rx="5" /><text x="29" y="69">−</text></g></g>
    {hover && <g className="crosshair"><line x1={hover.x} x2={hover.x} y1="0" y2={plotHeight + paneHeight} /><line x1="0" x2={plotWidth} y1={hover.y} y2={hover.y} /><rect x={Math.min(hover.x + 16, plotWidth - 208)} y="18" width="192" height="94" rx="5" /><text x={Math.min(hover.x + 28, plotWidth - 196)} y="40">{formatTime(hover.bar.time)} · {interval}</text><text x={Math.min(hover.x + 28, plotWidth - 196)} y="61">O {hover.bar.open.toFixed(3)}  H {hover.bar.high.toFixed(3)}</text><text x={Math.min(hover.x + 28, plotWidth - 196)} y="82">L {hover.bar.low.toFixed(3)}  C {hover.bar.close.toFixed(3)}</text><text x={Math.min(hover.x + 28, plotWidth - 196)} y="101">Vol {hover.bar.volume?.toLocaleString() ?? "—"}</text></g>}
    <line x1="0" x2={plotWidth + axisWidth} y1={plotHeight + paneHeight} y2={plotHeight + paneHeight} className="chart-axis" />
    <g className="chart-time">{xTicks.map((bar) => { const index = visible.indexOf(bar); return <text key={bar.time} x={step * index + step / 2} y={plotHeight + paneHeight + 28}>{formatTime(bar.time)}</text>; })}</g>
  </svg>;
}

function candleCountdown(interval: string, now: number): string {
  const secondsByInterval: Record<string, number> = {
    "1": 60,
    "5": 5 * 60,
    "15": 15 * 60,
    "30": 30 * 60,
    "60": 60 * 60,
    "240": 4 * 60 * 60,
    D: 24 * 60 * 60,
  };
  const duration = secondsByInterval[interval] ?? 15 * 60;
  const elapsed = Math.floor(now / 1_000) % duration;
  const remaining = duration - elapsed;
  const hours = Math.floor(remaining / 3_600);
  const minutes = Math.floor((remaining % 3_600) / 60);
  const seconds = remaining % 60;
  return hours > 0
    ? String(hours).padStart(2, "0") + ":" + String(minutes).padStart(2, "0") + ":" + String(seconds).padStart(2, "0")
    : String(minutes).padStart(2, "0") + ":" + String(seconds).padStart(2, "0");
}

function formatCalendarTime(timestamp: number): string {
  return new Intl.DateTimeFormat("en-GB", { weekday: "short", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false, timeZone: "UTC" }).format(new Date(timestamp * 1_000)) + " UTC";
}
