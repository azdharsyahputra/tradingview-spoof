export type CisdBar = {
  time: number;
  open: number;
  high: number;
  low: number;
  close: number;
  volume?: number;
};

export type CisdSetup = {
  id: string;
  index: number;
  time: number;
  startBar: number;
  endBar: number;
  slBar: number;
  bull: boolean;
  level: number;
  extreme: number;
  slPrice: number;
  target: number;
  grade: number; // 7: A+, 6: A, 5: B+, 4: B-, 3: C
  gradeText: "A+" | "A" | "B+" | "B-" | "C" | "-";
  potential: boolean;
  invalidated: boolean;
  checklist: {
    ckSweep: boolean;
    ckPDA: boolean;
    ckVol: boolean;
    ckSession: boolean;
    ckTargets: boolean;
    ckSMT: boolean;
  };
  contracts?: number;
};

export type CisdSweep = {
  time: number;
  index: number;
  price: number;
  buyside: boolean;
};

export type CisdSessionLevel = {
  name: string;
  price: number;
  time: number;
  isHigh: boolean;
  swept: boolean;
};

export type CisdModelResult = {
  setups: CisdSetup[];
  activeSetup: CisdSetup | null;
  potentialBull: CisdSetup | null;
  potentialBear: CisdSetup | null;
  sweeps: CisdSweep[];
  sessionLevels: CisdSessionLevel[];
  bias: "Neutral" | "Bullish" | "Bearish";
};

const GATE_WINDOW = 15;
const VOL_MULT = 1.2;
const MIN_RR = 1.5;

function gradeToText(grade: number): "A+" | "A" | "B+" | "B-" | "C" | "-" {
  switch (grade) {
    case 7:
      return "A+";
    case 6:
      return "A";
    case 5:
      return "B+";
    case 4:
      return "B-";
    case 3:
      return "C";
    default:
      return "-";
  }
}

export function calculateCisdModel(bars: CisdBar[]): CisdModelResult {
  if (bars.length < 20) {
    return {
      setups: [],
      activeSetup: null,
      potentialBull: null,
      potentialBear: null,
      sweeps: [],
      sessionLevels: [],
      bias: "Neutral",
    };
  }

  // 1. Calculate ATR (14) & Volume SMA (20)
  const atrs: number[] = new Array(bars.length).fill(0);
  const volSmas: (number | null)[] = new Array(bars.length).fill(null);

  let prevClose = bars[0].close;
  let runningAtr = 0;
  for (let i = 0; i < bars.length; i++) {
    const b = bars[i];
    const tr = i === 0 ? b.high - b.low : Math.max(b.high - b.low, Math.abs(b.high - prevClose), Math.abs(b.low - prevClose));
    runningAtr = i === 0 ? tr : runningAtr + (tr - runningAtr) / 14;
    atrs[i] = runningAtr;
    prevClose = b.close;

    if (i >= 19) {
      let sumVol = 0;
      for (let k = i - 19; k <= i; k++) sumVol += bars[k].volume ?? 0;
      volSmas[i] = sumVol / 20;
    }
  }

  // 2. Pivot Liquidity & Sweeps (pivothigh / pivotlow with len 2)
  const sweeps: CisdSweep[] = [];
  const bslLevels: { price: number; index: number; swept: boolean }[] = [];
  const sslLevels: { price: number; index: number; swept: boolean }[] = [];
  let lastSweepBuyBar = -10000;
  let lastSweepSellBar = -10000;

  for (let i = 2; i < bars.length - 2; i++) {
    // Pivot High
    if (bars[i].high > bars[i - 1].high && bars[i].high > bars[i - 2].high && bars[i].high >= bars[i + 1].high && bars[i].high >= bars[i + 2].high) {
      bslLevels.push({ price: bars[i].high, index: i, swept: false });
    }
    // Pivot Low
    if (bars[i].low < bars[i - 1].low && bars[i].low < bars[i - 2].low && bars[i].low <= bars[i + 1].low && bars[i].low <= bars[i + 2].low) {
      sslLevels.push({ price: bars[i].low, index: i, swept: false });
    }
  }

  // Track Sweeps over time (strict zero lookahead confirmation: pivot len = 2 requires i >= pivot.index + 2)
  for (let i = 0; i < bars.length; i++) {
    const b = bars[i];
    for (const bsl of bslLevels) {
      if (bsl.index + 2 <= i && !bsl.swept && b.high > bsl.price && b.close < bsl.price) {
        bsl.swept = true;
        lastSweepBuyBar = i;
        sweeps.push({ time: b.time, index: i, price: bsl.price, buyside: true });
      }
    }
    for (const ssl of sslLevels) {
      if (ssl.index + 2 <= i && !ssl.swept && b.low < ssl.price && b.close > ssl.price) {
        ssl.swept = true;
        lastSweepSellBar = i;
        sweeps.push({ time: b.time, index: i, price: ssl.price, buyside: false });
      }
    }
  }

  // 3. FVG Deliveries
  let lastBullFvgDeliver = -10000;
  let lastBearFvgDeliver = -10000;
  for (let i = 2; i < bars.length; i++) {
    const isBullFvg = bars[i].low > bars[i - 2].high && bars[i - 1].close > bars[i - 2].high;
    const isBearFvg = bars[i].high < bars[i - 2].low && bars[i - 1].close < bars[i - 2].low;
    if (isBullFvg) lastBullFvgDeliver = i;
    if (isBearFvg) lastBearFvgDeliver = i;
  }

  // 4. Session Levels (Current active PDH & PDL)
  const sessionLevels: CisdSessionLevel[] = [];
  let lastSessSweepBar = -10000;
  let lastSessSweepWasHigh = false;

  let currentDay = -1;
  let prevDayHigh = -1;
  let prevDayLow = -1;
  let dayHigh = -1;
  let dayLow = -1;

  for (let i = 0; i < bars.length; i++) {
    const date = new Date(bars[i].time * 1000);
    const day = date.getUTCDate();
    if (day !== currentDay) {
      if (currentDay !== -1) {
        prevDayHigh = dayHigh;
        prevDayLow = dayLow;
      }
      currentDay = day;
      dayHigh = bars[i].high;
      dayLow = bars[i].low;
    } else {
      if (bars[i].high > dayHigh) dayHigh = bars[i].high;
      if (bars[i].low < dayLow) dayLow = bars[i].low;
    }

    if (prevDayHigh > 0 && bars[i].high > prevDayHigh) {
      lastSessSweepBar = i;
      lastSessSweepWasHigh = true;
    }
    if (prevDayLow > 0 && bars[i].low < prevDayLow) {
      lastSessSweepBar = i;
      lastSessSweepWasHigh = false;
    }
  }

  if (prevDayHigh > 0 && prevDayLow > 0) {
    sessionLevels.push({ name: "PDH", price: prevDayHigh, time: bars[bars.length - 1].time, isHigh: true, swept: false });
    sessionLevels.push({ name: "PDL", price: prevDayLow, time: bars[bars.length - 1].time, isHigh: false, swept: false });
  }

  // Helper functions for CISD legs
  function legExtremeCurrent(idx: number, bias: number): [number, number] {
    let extreme = bars[idx].open;
    let extremeIdx = idx;
    const ct0 = bars[idx].close > bars[idx].open ? 1 : bars[idx].close < bars[idx].open ? -1 : 0;
    if (ct0 !== 0 && ct0 === bias) {
      for (let i = idx - 1; i >= 0; i--) {
        const ctype = bars[i].close > bars[i].open ? 1 : bars[i].close < bars[i].open ? -1 : 0;
        if (ctype === 0) continue;
        if (ctype !== bias) break;
        if (bias === 1 && bars[i].open < extreme) {
          extreme = bars[i].open;
          extremeIdx = i;
        } else if (bias === -1 && bars[i].open > extreme) {
          extreme = bars[i].open;
          extremeIdx = i;
        }
      }
    }
    return [extreme, extremeIdx];
  }

  function legExtremePrevious(idx: number, bias: number): [number, number] {
    let found = false;
    let extreme = bars[idx].open;
    let extremeIdx = idx;
    for (let j = idx - 1; j >= 0; j--) {
      const ctype = bars[j].close > bars[j].open ? 1 : bars[j].close < bars[j].open ? -1 : 0;
      if (ctype === 0) continue;
      const matches = ctype === bias;
      if (!found) {
        if (matches) {
          found = true;
          extreme = bars[j].open;
          extremeIdx = j;
        }
      } else {
        if (!matches) break;
        if (bias === 1 && bars[j].open < extreme) {
          extreme = bars[j].open;
          extremeIdx = j;
        } else if (bias === -1 && bars[j].open > extreme) {
          extreme = bars[j].open;
          extremeIdx = j;
        }
      }
    }
    return [extreme, extremeIdx];
  }

  // 5. State Machine for CISD Sequence
  let cmBias = 0;
  let cmLevel = 0;
  let cmLevelBar = 0;
  let cmPivot = 0;
  let cmPivotBar = 0;

  // Initialize seed
  for (let k = 0; k < Math.min(bars.length, 50); k++) {
    const seed = bars[k].close > bars[k].open ? 1 : bars[k].close < bars[k].open ? -1 : 0;
    if (seed !== 0) {
      cmBias = seed;
      const [oPrice, oBar] = legExtremeCurrent(k, seed);
      cmLevel = oPrice;
      cmLevelBar = oBar;
      cmPivot = seed === 1 ? bars[k].high : bars[k].low;
      cmPivotBar = k;
      break;
    }
  }

  const setups: CisdSetup[] = [];

  for (let i = 1; i < bars.length; i++) {
    const bar = bars[i];
    const cmCType = bar.close > bar.open ? 1 : bar.close < bar.open ? -1 : 0;

    if (cmBias === 1 && bar.high > cmPivot) {
      cmPivot = bar.high;
      cmPivotBar = i;
      const [p, pb] = cmCType === 1 ? legExtremeCurrent(i, 1) : legExtremePrevious(i, 1);
      cmLevel = p;
      cmLevelBar = pb;
    } else if (cmBias === -1 && bar.low < cmPivot) {
      cmPivot = bar.low;
      cmPivotBar = i;
      const [p, pb] = cmCType === -1 ? legExtremeCurrent(i, -1) : legExtremePrevious(i, -1);
      cmLevel = p;
      cmLevelBar = pb;
    }

    const bullFlip = cmBias === -1 && bar.close > cmLevel;
    const bearFlip = cmBias === 1 && bar.close < cmLevel;

    if (bullFlip || bearFlip) {
      const isBull = bullFlip;
      const level = cmLevel;
      const extreme = cmPivot;
      const startB = cmLevelBar;
      const endB = i;
      const slB = cmPivotBar;

      // Quality Checklist
      const ckSweep = isBull ? i - lastSweepSellBar <= GATE_WINDOW : i - lastSweepBuyBar <= GATE_WINDOW;
      const ckPDA = isBull ? i - lastBullFvgDeliver <= GATE_WINDOW : i - lastBearFvgDeliver <= GATE_WINDOW;
      const volSMA = volSmas[i] ?? 0;
      const ckVol = (bar.volume ?? 0) > volSMA * VOL_MULT;
      const ckSession = i - lastSessSweepBar <= GATE_WINDOW && (isBull ? !lastSessSweepWasHigh : lastSessSweepWasHigh);
      const slDist = Math.abs(level - extreme);
      const targetDist = slDist * MIN_RR;
      const target = isBull ? level + targetDist : level - targetDist;
      const ckTargets = slDist > 0 && targetDist / slDist >= MIN_RR;
      const ckSMT = true; // SMT alignment

      let hits = 0;
      if (ckSweep) hits++;
      if (ckPDA) hits++;
      if (ckVol) hits++;
      if (ckSession) hits++;
      if (ckTargets) hits++;
      if (ckSMT) hits++;

      const ratio = hits / 6;
      const grade = ratio >= 0.95 ? 7 : ratio >= 0.8 ? 6 : ratio >= 0.65 ? 5 : ratio >= 0.45 ? 4 : 3;

      const setup: CisdSetup = {
        id: `cisd-${bar.time}-${isBull ? "bull" : "bear"}`,
        index: i,
        time: bar.time,
        startBar: startB,
        endBar: endB,
        slBar: slB,
        bull: isBull,
        level,
        extreme,
        slPrice: extreme,
        target,
        grade,
        gradeText: gradeToText(grade),
        potential: false,
        invalidated: false,
        checklist: {
          ckSweep,
          ckPDA,
          ckVol,
          ckSession,
          ckTargets,
          ckSMT,
        },
      };

      setups.push(setup);

      // State flip update
      const newBias = isBull ? 1 : -1;
      cmBias = newBias;
      const [np, npb] = legExtremeCurrent(i, newBias);
      cmLevel = np;
      cmLevelBar = npb;
      cmPivot = newBias === 1 ? bar.high : bar.low;
      cmPivotBar = i;
    }
  }

  // Check invalidation for active setups
  for (const setup of setups) {
    for (let k = setup.endBar + 1; k < bars.length; k++) {
      const b = bars[k];
      if (setup.bull && b.close < setup.slPrice) {
        setup.invalidated = true;
        break;
      }
      if (!setup.bull && b.close > setup.slPrice) {
        setup.invalidated = true;
        break;
      }
    }
  }

  // Potential (unconfirmed) CISD on current live candle
  const lastIndex = bars.length - 1;
  const lastClose = bars[lastIndex].close;
  let potentialBull: CisdSetup | null = null;
  let potentialBear: CisdSetup | null = null;

  if (cmBias === -1 && lastClose < cmLevel) {
    const slDist = Math.abs(cmLevel - cmPivot);
    potentialBull = {
      id: "potential-bull",
      index: lastIndex,
      time: bars[lastIndex].time,
      startBar: cmLevelBar,
      endBar: lastIndex,
      slBar: cmPivotBar,
      bull: true,
      level: cmLevel,
      extreme: cmPivot,
      slPrice: cmPivot,
      target: cmLevel + slDist * MIN_RR,
      grade: 5,
      gradeText: "B+",
      potential: true,
      invalidated: false,
      checklist: {
        ckSweep: lastIndex - lastSweepSellBar <= GATE_WINDOW,
        ckPDA: lastIndex - lastBullFvgDeliver <= GATE_WINDOW,
        ckVol: (bars[lastIndex].volume ?? 0) > (volSmas[lastIndex] ?? 0) * VOL_MULT,
        ckSession: lastIndex - lastSessSweepBar <= GATE_WINDOW && !lastSessSweepWasHigh,
        ckTargets: true,
        ckSMT: true,
      },
    };
  } else if (cmBias === 1 && lastClose > cmLevel) {
    const slDist = Math.abs(cmLevel - cmPivot);
    potentialBear = {
      id: "potential-bear",
      index: lastIndex,
      time: bars[lastIndex].time,
      startBar: cmLevelBar,
      endBar: lastIndex,
      slBar: cmPivotBar,
      bull: false,
      level: cmLevel,
      extreme: cmPivot,
      slPrice: cmPivot,
      target: cmLevel - slDist * MIN_RR,
      grade: 5,
      gradeText: "B+",
      potential: true,
      invalidated: false,
      checklist: {
        ckSweep: lastIndex - lastSweepBuyBar <= GATE_WINDOW,
        ckPDA: lastIndex - lastBearFvgDeliver <= GATE_WINDOW,
        ckVol: (bars[lastIndex].volume ?? 0) > (volSmas[lastIndex] ?? 0) * VOL_MULT,
        ckSession: lastIndex - lastSessSweepBar <= GATE_WINDOW && lastSessSweepWasHigh,
        ckTargets: true,
        ckSMT: true,
      },
    };
  }

  const validSetups = setups.filter((s) => !s.invalidated);
  const activeSetup = validSetups.length > 0 ? validSetups[validSetups.length - 1] : null;

  const bias: "Neutral" | "Bullish" | "Bearish" =
    activeSetup ? (activeSetup.bull ? "Bullish" : "Bearish") : cmBias === 1 ? "Bullish" : cmBias === -1 ? "Bearish" : "Neutral";

  // Only display the most recent 3 valid setups to keep chart clean (matches Pine Script i_setupsShown & i_hideInvalid)
  const displaySetups = validSetups.slice(-3);

  return {
    setups: displaySetups,
    activeSetup,
    potentialBull,
    potentialBear,
    sweeps: sweeps.slice(-4),
    sessionLevels,
    bias,
  };
}
