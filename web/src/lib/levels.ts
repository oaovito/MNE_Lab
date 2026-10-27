// Order of design levels, the same rule as the core (design.OrderTimeLabels):
// time points by their numeric offset in the design (0, 7, 14, 21), never by
// the byte order of their labels (0, 14, 21, 7). A label without an offset
// is placed by the number it starts with; labels without any number come
// after, in byte order, which also breaks ties. Conditions follow the
// design's declared order. The order only arranges levels for display,
// exports and the engine's factor levels; no value is derived from it.

type TimeLevel = { label: string; offset?: number | null; unit?: string };

const leadingNumber = /^\s*([-+]?(?:\d+(?:\.\d*)?|\.\d+))/;
const bytes = (a: string, b: string) => (a < b ? -1 : a > b ? 1 : 0);

export function orderTimeLabels(labels: string[], times: TimeLevel[] = []): string[] {
  const withOffset = times.filter((t) => typeof t.offset === 'number');
  const sameUnit = withOffset.every((t) => (t.unit || '') === (withOffset[0]?.unit || ''));
  const key = (l: string): number | undefined => {
    const t = withOffset.find((x) => x.label === l);
    if (t && sameUnit) return t.offset as number;
    const m = leadingNumber.exec(l);
    return m ? parseFloat(m[1]) : undefined;
  };
  return [...labels]
    .map((l, i) => ({ l, i, k: key(l) }))
    .sort((a, b) => {
      if (a.k !== undefined && b.k !== undefined && a.k !== b.k) return a.k - b.k;
      if ((a.k !== undefined) !== (b.k !== undefined)) return a.k !== undefined ? -1 : 1;
      return bytes(a.l, b.l) || a.i - b.i;
    })
    .map((x) => x.l);
}

export function orderConditionLabels(labels: string[], declared: string[] = []): string[] {
  const rank = (l: string) => declared.indexOf(l);
  return [...labels].sort((a, b) => {
    const x = rank(a),
      y = rank(b);
    if (x >= 0 && y >= 0) return x - y;
    if ((x >= 0) !== (y >= 0)) return x >= 0 ? -1 : 1;
    return bytes(a, b);
  });
}

/**
 * The levels of a paired analysis in their scientific order: conditions as
 * the design declares them, times by their offset (0, 7, 14, 21), never the
 * byte order of their labels. The engine receives them as factor levels, so
 * groups, contrasts and exports follow the same order.
 */
export function pairedLevels(s: {
  design: { levelsA?: string[]; levelsB?: string[] };
  experimentalDesign?: { conditions: { label: string }[]; times: TimeLevel[] };
}): { levelsA: string[]; levelsB: string[] } {
  const d = s.experimentalDesign;
  return {
    levelsA: orderConditionLabels(s.design.levelsA || [], d?.conditions.map((c) => c.label)),
    levelsB: orderTimeLabels(s.design.levelsB || [], d?.times),
  };
}
