export type SolBar = {
  time: number;
  open: number;
  high: number;
  low: number;
  close: number;
};

export type SolSignal = {
  index: number;
  time: number;
  side: "BUY" | "SELL";
  setup: "SWEEP" | "BREAK";
  entry: number;
  invalidation: number;
  target: number;
};

const LOOKBACK = 10;
const ATR_PERIOD = 14;
const COOLDOWN = 7;

export function solIntervalSeconds(interval: string): number {
  if (interval === "D") return 86_400;
  if (interval === "W") return 604_800;
  const minutes = Number(interval);
  return Number.isFinite(minutes) && minutes > 0 ? minutes * 60 : 900;
}

// The latest bar is only usable once its own interval has finished.
export function lastConfirmedSolIndex(bars: SolBar[], interval: string, nowMs: number): number {
  const nowSeconds = Math.floor(nowMs / 1_000);
  const duration = solIntervalSeconds(interval);
  let index = bars.length - 1;
  while (index >= 0 && bars[index].time + duration > nowSeconds) index -= 1;
  return index;
}

export function calculateSolSignals(bars: SolBar[], lastConfirmedIndex: number): SolSignal[] {
  const end = Math.min(lastConfirmedIndex, bars.length - 1);
  if (end < 35) return [];

  const signals: SolSignal[] = [];
  const fastMultiplier = 2 / 9;
  const slowMultiplier = 2 / 22;
  let fast = bars[0].close;
  let slow = bars[0].close;
  let previousFast = fast;
  let atr = 0;
  let previousClose = bars[0].close;
  let previousSignalIndex = -COOLDOWN - 1;

  for (let index = 1; index <= end; index += 1) {
    const bar = bars[index];
    const previous = bars[index - 1];
    previousFast = fast;
    fast += (bar.close - fast) * fastMultiplier;
    slow += (bar.close - slow) * slowMultiplier;

    const trueRange = Math.max(bar.high - bar.low, Math.abs(bar.high - previousClose), Math.abs(bar.low - previousClose));
    atr = index === 1 ? trueRange : atr + (trueRange - atr) / ATR_PERIOD;
    previousClose = bar.close;
    if (index < 35 || atr <= 0 || index - previousSignalIndex <= COOLDOWN) continue;

    const prior = bars.slice(index - LOOKBACK, index);
    const priorHigh = Math.max(...prior.map((candle) => candle.high));
    const priorLow = Math.min(...prior.map((candle) => candle.low));
    const span = bar.high - bar.low;
    if (span <= 0) continue;
    const closePosition = (bar.close - bar.low) / span;
    const body = Math.abs(bar.close - bar.open);
    const sweptBothSides = bar.low < priorLow && bar.high > priorHigh;

    let side: SolSignal["side"] | null = null;
    let setup: SolSignal["setup"] = "BREAK";
    if (!sweptBothSides && bar.low < priorLow - atr * 0.08 && bar.close > priorLow + atr * 0.04 && bar.close > bar.open && closePosition >= 0.68 && bar.close >= slow - atr * 0.5) {
      side = "BUY";
      setup = "SWEEP";
    } else if (!sweptBothSides && bar.high > priorHigh + atr * 0.08 && bar.close < priorHigh - atr * 0.04 && bar.close < bar.open && closePosition <= 0.32 && bar.close <= slow + atr * 0.5) {
      side = "SELL";
      setup = "SWEEP";
    } else if (bar.close > priorHigh + atr * 0.08 && bar.close > bar.open && closePosition >= 0.72 && body >= atr * 0.42 && fast > slow && fast > previousFast && previous.close <= priorHigh) {
      side = "BUY";
    } else if (bar.close < priorLow - atr * 0.08 && bar.close < bar.open && closePosition <= 0.28 && body >= atr * 0.42 && fast < slow && fast < previousFast && previous.close >= priorLow) {
      side = "SELL";
    }
    if (!side) continue;

    const invalidation = side === "BUY" ? bar.low - atr * 0.15 : bar.high + atr * 0.15;
    const risk = Math.abs(bar.close - invalidation);
    if (risk < atr * 0.25 || risk > atr * 2.5) continue;
    signals.push({
      index,
      time: bar.time,
      side,
      setup,
      entry: bar.close,
      invalidation,
      target: bar.close + (side === "BUY" ? 1 : -1) * risk * 1.5,
    });
    previousSignalIndex = index;
  }
  return signals;
}
