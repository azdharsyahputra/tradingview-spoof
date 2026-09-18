import assert from "node:assert/strict";
import { test } from "node:test";
import { calculateSolSignals, lastConfirmedSolIndex } from "../lib/sol.ts";

function sampleBars() {
  const bars = Array.from({ length: 50 }, (_, index) => {
    const close = 100 + index * 0.12;
    return { time: 1_700_000_000 + index * 900, open: close - 0.07, high: close + 0.13, low: close - 0.16, close };
  });
  bars[45] = { ...bars[45], open: 105.25, high: 105.88, low: 105.2, close: 105.8 };
  return bars;
}

test("SOL waits for the active candle to close", () => {
  const bars = sampleBars();
  const signalBar = bars[45];
  assert.equal(lastConfirmedSolIndex(bars.slice(0, 46), "15", (signalBar.time + 899) * 1_000), 44);
  assert.equal(lastConfirmedSolIndex(bars.slice(0, 46), "15", (signalBar.time + 900) * 1_000), 45);
});

test("SOL does not change earlier signals when later candles arrive", () => {
  const bars = sampleBars();
  const prior = calculateSolSignals(bars.slice(0, 46), 45);
  const after = calculateSolSignals(bars, 45);
  assert.deepEqual(after, prior);
  assert.ok(prior.some((signal) => signal.side === "BUY"));
  assert.ok(prior.every((signal) => signal.side === "BUY" ? signal.invalidation < signal.entry && signal.target > signal.entry : signal.invalidation > signal.entry && signal.target < signal.entry));
});
