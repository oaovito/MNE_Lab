// Draws a figure laid out by the core (the same display list the exports
// use, so the screen and the exported file match) and adds the screen-only
// interaction: inspect on hover, drag to zoom, double-click to reset.
import type { JSX } from 'preact';
import { useMemo, useRef, useState } from 'preact/hooks';
import { fmtNum } from '../lib/format';
import { t } from '../lib/i18n';
import type { Figure as Fig, Op, Series } from '../lib/types';

const FONT = "Inter, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif";
const norm = (s: string) => s.replace(/μ/g, 'µ');

export type View = { XMin: number; XMax: number; YMin: number; YMax: number };

/** seriesLabel localizes generated series labels (same rules as the core). */
export function seriesLabel(label: string): string {
  if(label.startsWith('statistics:')){const [,key,...rest]=label.split(':');return t('series.stat_'+key)+' · '+rest.join(':');}
  if (label.startsWith('series.')) return t(label);
  if (label.startsWith('point:')) {
    const p = label.split(':'); // point:<unit>:<offset>[:<replicate>]
    if (p.length === 3) return t('point.' + p[1], { n: p[2] });
    if (p.length === 4) return t('point.' + p[1], { n: p[2] }) + ' · R' + p[3];
  }
  return label;
}

function draw(ops: Op[], hidden?: Set<string>): JSX.Element[] {
  const out: JSX.Element[] = [];
  let group: JSX.Element[] | null = null;
  let clipN = 0;
  const push = (el: JSX.Element) => (group ? group.push(el) : out.push(el));
  ops.forEach((o, i) => {
    if (o.role && hidden?.has(o.role)) return;
    const paint = {
      fill: o.fill || 'none',
      'fill-opacity': o.fill && o.op && o.op < 1 ? o.op : undefined,
      stroke: o.stroke || undefined,
      'stroke-width': o.stroke ? o.sw : undefined,
      'stroke-dasharray': o.dash?.join(' '),
      'stroke-linecap': o.dash ? ('round' as const) : undefined,
    };
    switch (o.k) {
      case 'rect':
        push(<rect key={i} x={o.x || 0} y={o.y || 0} width={o.w || 0} height={o.h || 0} {...paint} />);
        break;
      case 'line':
        push(<line key={i} x1={o.x || 0} y1={o.y || 0} x2={o.x2 || 0} y2={o.y2 || 0} {...paint} />);
        break;
      case 'poly': {
        let d = '';
        const p = o.pts || [];
        for (let j = 0; j + 1 < p.length; j += 2) d += (j ? ' ' : '') + p[j].toFixed(2) + ',' + p[j + 1].toFixed(2);
        push(<polyline key={i} points={d} fill="none" stroke={o.stroke} stroke-width={o.sw} stroke-dasharray={o.dash?.join(' ')} stroke-linejoin="round" stroke-linecap="round" />);
        break;
      }
      case 'circle':
        push(<circle key={i} cx={o.x || 0} cy={o.y || 0} r={o.r || 0} {...paint} />);
        break;
      case 'text':
        push(
          <text
            key={i}
            x={o.x || 0}
            y={o.y || 0}
            font-size={o.size}
            font-weight={o.font === 'sans-bold' ? 600 : undefined}
            text-anchor={o.anchor === 'middle' || o.anchor === 'end' ? o.anchor : undefined}
            fill={o.fill}
            transform={o.rot ? `rotate(${o.rot} ${o.x || 0} ${o.y || 0})` : undefined}
          >
            {norm(o.t || '')}
          </text>,
        );
        break;
      case 'clip': {
        clipN++;
        const id = `clip-${clipN}`;
        out.push(
          <clipPath key={'c' + i} id={id}>
            <rect x={o.x || 0} y={o.y || 0} width={o.w || 0} height={o.h || 0} />
          </clipPath>,
        );
        group = [];
        out.push(<g key={'g' + i} clip-path={`url(#${id})`}>{group}</g>);
        break;
      }
      case 'unclip':
        group = null;
        break;
    }
  });
  return out;
}

type Props = {
  figure: Fig;
  series?: Series[];
  xUnit?: string;
  yUnit?: string;
  interactive?: boolean;
  onZoom?: (v: View) => void;
  onReset?: () => void;
  class?: string;
  label?: string;
};

export function FigureView(p: Props) {
  const f = p.figure;
  const svgRef = useRef<SVGSVGElement>(null);
  const [hover, setHover] = useState<{ x: number; y: number; s: number; i: number } | null>(null);
  const [drag, setDrag] = useState<{ x0: number; y0: number; x1: number; y1: number } | null>(null);
  const body = useMemo(() => draw(f.ops), [f]);
  const plot = f.map.plot;

  const toFig = (e: MouseEvent) => {
    const svg = svgRef.current!;
    const pt = svg.createSVGPoint();
    pt.x = e.clientX;
    pt.y = e.clientY;
    const m = svg.getScreenCTM();
    if (!m) return { x: 0, y: 0 };
    const r = pt.matrixTransform(m.inverse());
    return { x: r.x, y: r.y };
  };
  const inPlot = (x: number, y: number) => x >= plot.X && x <= plot.X + plot.W && y >= plot.Y && y <= plot.Y + plot.H;

  // Figure coordinates back to data values (log or linear axes).
  const inv = (v: number, a: number, len: number, min: number, max: number, scale: string, flip: boolean) => {
    let u = (v - a) / len;
    if (flip) u = 1 - u;
    if (scale === 'log') {
      const l0 = Math.log10(min),
        l1 = Math.log10(max);
      return Math.pow(10, l0 + u * (l1 - l0));
    }
    return min + u * (max - min);
  };
  const dataAt = (x: number, y: number) => ({
    x: inv(x, plot.X, plot.W, f.map.xMin, f.map.xMax, f.map.xScale, false),
    y: inv(y, plot.Y, plot.H, f.map.yMin, f.map.yMax, f.map.yScale, true),
  });

  const move = (e: MouseEvent) => {
    if (!p.interactive) return;
    const { x, y } = toFig(e);
    if (drag) {
      setDrag({ ...drag, x1: Math.min(Math.max(x, plot.X), plot.X + plot.W), y1: Math.min(Math.max(y, plot.Y), plot.Y + plot.H) });
      return;
    }
    if (!f.hits?.length || !inPlot(x, y)) return setHover(null);
    let best = -1,
      bd = Infinity;
    const tol = Math.max(10, f.width / 60);
    for (let k = 0; k < f.hits.length; k++) {
      const h = f.hits[k];
      const d = Math.hypot(h.x - x, (h.y - y) * 0.6);
      if (d < bd) {
        bd = d;
        best = k;
      }
    }
    if (best < 0 || bd > tol * 3) return setHover(null);
    const h = f.hits[best];
    setHover({ x: h.x, y: h.y, s: h.s, i: h.i });
  };
  const down = (e: MouseEvent) => {
    if (!p.interactive || !p.onZoom || e.button !== 0) return;
    const { x, y } = toFig(e);
    if (!inPlot(x, y)) return;
    setHover(null);
    setDrag({ x0: x, y0: y, x1: x, y1: y });
  };
  const up = () => {
    if (!drag) return;
    const w = Math.abs(drag.x1 - drag.x0),
      h = Math.abs(drag.y1 - drag.y0);
    setDrag(null);
    if (w < 8 || h < 8) return;
    const a = dataAt(Math.min(drag.x0, drag.x1), Math.max(drag.y0, drag.y1));
    const b = dataAt(Math.max(drag.x0, drag.x1), Math.min(drag.y0, drag.y1));
    p.onZoom?.({ XMin: a.x, XMax: b.x, YMin: a.y, YMax: b.y });
  };

  const se = hover && p.series ? p.series[hover.s] : undefined;
  let tip: JSX.Element | null = null;
  if (hover && se) {
    const xv = se.x[hover.i],
      yv = se.y[hover.i];
    const left = (hover.x / f.width) * 100;
    const top = (hover.y / f.height) * 100;
    tip = (
      <div class="fig-tip" style={{ left: `${left}%`, top: `${top}%`, transform: `translate(${left > 70 ? '-100%' : '0'}, -100%)` }}>
        <div class="row gap1">
          <span class="dot" style={{ color: se.color || 'var(--accent)' }} />
          <strong class="ellipsis">{seriesLabel(se.label)}</strong>
        </div>
        <div class="num">
          {fmtNum(xv)} {p.xUnit || ''} · {fmtNum(yv)} {p.yUnit || ''}
        </div>
        {se.err?.[hover.i] !== undefined && (
          <div class="faint num">
            ± {fmtNum(se.err[hover.i])} · n = {se.n?.[hover.i]}
          </div>
        )}
      </div>
    );
  }

  return (
    <div class={`figure ${p.class || ''}`} onMouseLeave={() => (setHover(null), drag && setDrag(null))}>
      <svg
        ref={svgRef}
        viewBox={`0 0 ${f.width} ${f.height}`}
        preserveAspectRatio="xMidYMid meet"
        font-family={FONT}
        role="img"
        aria-label={p.label || f.title}
        onMouseMove={move as any}
        onMouseDown={down as any}
        onMouseUp={up}
        onDblClick={() => p.onReset?.()}
        style={{ cursor: p.interactive && p.onZoom ? 'crosshair' : undefined }}
      >
        <title>{f.title}</title>
        {body}
        {hover && <circle cx={hover.x} cy={hover.y} r={Math.max(4, f.width / 220)} fill="none" stroke="#111827" stroke-width={1.4} pointer-events="none" />}
        {drag && <rect x={Math.min(drag.x0, drag.x1)} y={Math.min(drag.y0, drag.y1)} width={Math.abs(drag.x1 - drag.x0)} height={Math.abs(drag.y1 - drag.y0)} fill="rgba(13,148,136,0.10)" stroke="#0d9488" stroke-width={1} stroke-dasharray="4 3" pointer-events="none" />}
      </svg>
      {tip}
    </div>
  );
}
