// Paired Condition × Time design in Statistical Analysis: the editor takes
// units, conditions and times from a saved design revision (the person
// reviews, never rebuilds), a review card shows the structure before the
// calculation, and the results show Condition, Time and Condition × Time
// separately with their own error strata, sphericity notes, simple effects,
// effect sizes, estimated differences and every unit's own observations.
import { fmtNum, squared } from '../../lib/format';
import { t } from '../../lib/i18n';
import { navigate } from '../../lib/route';
import type { AnalysisDefinition, AnalysisSnapshot, StatisticalResults } from '../../lib/statistics';
import type { DesignView } from '../../lib/types';
import { Button, Notice } from '../../ui/kit';

const num = (n: number | undefined | null) => (n == null ? '—' : fmtNum(n, 6));
const pvalue = (n: number | undefined | null) => (n == null ? '—' : n === 0 ? t('stat.p_underflow') : n < 0.000001 ? n.toExponential(4) : fmtNum(n, 6));
// Degrees of freedom as returned: whole numbers plainly, corrected ones to 4 decimals.
const dfText = (n: number | undefined | null) => (n == null ? '—' : Number.isInteger(n) ? String(n) : fmtNum(n, 4));
const natural = (a: string, b: string) => a.localeCompare(b, undefined, { numeric: true, sensitivity: 'base' });

/** The definition the editor proposes for a saved design revision. */
export function definitionFromDesign(v: DesignView, base: AnalysisDefinition): AnalysisDefinition {
  const paired = v.structure.detected === 'paired_longitudinal';
  const longitudinal = v.structure.timeWithin;
  return {
    ...base,
    title: base.title || v.design.name,
    designId: v.design.id,
    factorAName: t('stat.condition'),
    factorBName: v.design.times.length ? t('stat.time') : '',
    // Still a decision the person must confirm (structureReviewed).
    structure: paired || longitudinal ? 'repeated' : 'independent',
    structureReviewed: false,
    design: paired ? 'within_within' : '',
    method: paired || longitudinal ? 'repeated' : v.design.times.length > 1 ? 'two_way' : 'one_way',
    sphericityCorrection: paired || longitudinal ? 'GG' : '',
    postHoc: paired ? 'simple_effects' : 'none',
    adjustment: paired ? 'holm' : '',
    control: '',
    effectCI: false,
    observations: v.design.assignments.map((a) => ({ measurementId: a.measurementId, factorA: a.condition || '', factorB: a.time || '', unitId: a.unitId || '' })),
  };
}

/** Levels in the order the design declares them, else in natural order. */
export function levelOrder(levels: string[], declared?: string[]) {
  const out = (declared || []).filter((l) => levels.includes(l));
  return [...out, ...levels.filter((l) => !out.includes(l)).sort(natural)];
}

export function unitName(s: AnalysisSnapshot, id: string) {
  return s.unitLabels?.[id] || s.experimentalDesign?.units.find((u) => u.id === id)?.label || id.slice(0, 8);
}

function order(s: AnalysisSnapshot) {
  const d = s.experimentalDesign;
  return {
    a: levelOrder(s.design.levelsA || [], d?.conditions.map((c) => c.label)),
    b: levelOrder(s.design.levelsB || [], d?.times.map((x) => x.label)),
  };
}

/**
 * DesignReviewCard shows, before the calculation, what the analysis will
 * treat as its subjects and factors (addendum B §38, A §15): experimental
 * units by name, the within-unit factors with their levels, how many
 * readings each unit has in each cell and that technical readings are
 * averaged, and N as the number of units.
 */
export function DesignReviewCard(p: { snapshot: AnalysisSnapshot }) {
  const s = p.snapshot;
  const def = s.definition;
  const { a, b } = order(s);
  const paired = def.design === 'within_within';
  const rows = s.observations.filter((o) => !o.missing && !o.excludeReason);
  const units = [...new Set(rows.map((o) => o.unitId || ''))].filter(Boolean);
  const cells = new Map<string, number>();
  for (const o of rows) {
    const k = `${o.unitId}\u0000${o.factorA}\u0000${o.factorB || ''}`;
    cells.set(k, (cells.get(k) || 0) + 1);
  }
  const readingCells = [...cells.values()].filter((n) => n > 1);
  const design = s.experimentalDesign;
  const cols = b.length ? a.flatMap((x) => b.map((y) => [x, y] as const)) : a.map((x) => [x, ''] as const);
  return (
    <section class="card pad col gap2 stat-review" aria-label={t('stat.review_card')}>
      <h3>{t('stat.review_card')}</h3>
      <p class="faint">
        {design ? t('stat.review_from_design', { name: design.name, n: design.revision }) : t('stat.review_from_editor')}
      </p>
      <div class="stat-review-grid">
        <div class="col gap1">
          <b>{t('stat.review_units', { n: s.design.experimentalUnits })}</b>
          <span class="stat-review-units">{units.map((u) => unitName(s, u)).join(' · ') || t('stat.review_units_none')}</span>
        </div>
        <div class="col gap1">
          <b>{paired || def.structure === 'repeated' ? (paired ? t('stat.review_within', { name: def.factorAName || 'A' }) : t('stat.review_between', { name: def.factorAName || 'A' })) : t('stat.review_groups', { name: def.factorAName || 'A' })}</b>
          <span>{a.join(' / ')}</span>
        </div>
        {b.length > 0 && b.some(Boolean) && (
          <div class="col gap1">
            <b>{def.structure === 'repeated' ? t('stat.review_within', { name: def.factorBName || 'B' }) : t('stat.review_groups', { name: def.factorBName || 'B' })}</b>
            <span>{b.join(' / ')}</span>
          </div>
        )}
      </div>
      <p>
        <b>{t('stat.review_n', { n: s.design.experimentalUnits, m: s.design.n })}</b>
      </p>
      {paired && (
        <Notice kind="info">
          {readingCells.length
            ? t('stat.review_readings', { cells: readingCells.length, readings: readingCells.reduce((x, y) => x + y, 0) })
            : t('stat.review_readings_none')}
        </Notice>
      )}
      {units.length > 0 && (
        <div style={{ overflowX: 'auto' }}>
          <table class="table stat-review-cells">
            <thead>
              <tr>
                <th>{t('stat.unit')}</th>
                {cols.map(([x, y]) => (
                  <th>{[x, y].filter(Boolean).join(' · ')}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {units.map((u) => (
                <tr>
                  <td>{unitName(s, u)}</td>
                  {cols.map(([x, y]) => {
                    const n = cells.get(`${u}\u0000${x}\u0000${y}`) || 0;
                    return <td class={n === 0 ? 'faint' : n > 1 ? 'stat-cell-readings' : ''}>{n === 0 ? '—' : n === 1 ? '1' : t('stat.review_cell_mean', { n })}</td>;
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <div class="row gap2 wrap">
        {design ? (
          <Button icon="layers" onClick={() => navigate('/ls/designs/' + design.id)}>
            {t('stat.review_correct')}
          </Button>
        ) : (
          <span class="faint">{t('stat.review_correct_above')}</span>
        )}
      </div>
    </section>
  );
}

function sphericityNote(r: StatisticalResults, term: string, context = '') {
  const label = context ? `${term}; ${context}` : term;
  const d = r.diagnostics.find((x) => (x.code === 'mauchly' || x.code === 'mauchly_not_estimable') && (x.details === label || x.details?.startsWith(label + ';')) && (context || !x.details?.includes('A=')));
  if (!d) return null;
  return d.code === 'mauchly' ? t('stat.sphericity_tested', { w: num(d.statistic), p: pvalue(d.p) }) : t('stat.sphericity_untested');
}

type Term = StatisticalResults['terms'][0];

/** One effect with its corrected variants and its own error stratum. */
function EffectTable(p: { r: StatisticalResults; terms: Term[]; term: string; unit: string; context?: string; corrections?: Record<string, number> }) {
  const { r, terms, term } = p;
  const classical = !r.model;
  const rows = terms.filter((x) => x.source === term || x.source.startsWith(term + '_'));
  const errorName = { A: 'residual_unit:A', B: 'residual_unit:B', 'A:B': 'residual_unit:A:B' }[term];
  const error = terms.find((x) => x.source === errorName);
  const main = rows.find((x) => x.source === term);
  const u2 = p.unit ? ` (${squared(p.unit)})` : '';
  const note = sphericityNote(r, term, p.context);
  const applied = (p.corrections || r.corrections)?.['applied:' + term];
  const kind = (src: string) => ({ GG: 'Greenhouse–Geisser', HF: 'Huynh–Feldt', LB: t('stat.lower_bound') })[src.split('_').pop() || ''] || t('stat.uncorrected');
  return (
    <div class="col gap1">
      <div style={{ overflowX: 'auto' }}>
        <table class="table">
          <thead>
            <tr>
              <th>{t('stat.test')}</th>
              {classical && <th>SS{u2}</th>}
              <th>df</th>
              <th>{t('stat.denominator_df')}</th>
              {classical && <th>MS{u2}</th>}
              <th>F</th>
              <th>p</th>
              {classical && <th>ε</th>}
            </tr>
          </thead>
          <tbody>
            {rows.map((x) => (
              <tr>
                <td>{x.source === term ? t('stat.uncorrected') : kind(x.source)}</td>
                {classical && <td>{num(x.ss)}</td>}
                <td>{dfText(x.df)}</td>
                <td>{dfText(x.denominatorDF)}</td>
                {classical && <td>{num(x.ms)}</td>}
                <td>{num(x.f)}</td>
                <td>{pvalue(x.p)}</td>
                {classical && <td>{num(x.epsilon)}</td>}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {classical && main && (
        <p class="faint">
          {t('stat.effect_sizes')}: partial η² {num(main.partialEtaSquared)} · {t('stat.generalized_eta')} {num(main.generalizedEtaSquared)} · η² {num(main.etaSquared)}
          {error ? ` · ${t('stat.error_stratum')}: ${error.source} (SS ${num(error.ss)}, df ${dfText(error.df)})` : ''}
        </p>
      )}
      {note && <p class="faint">{note}{applied != null ? ` · ${t('stat.applied_correction')} ε=${num(applied)}` : ''}</p>}
    </div>
  );
}

/** Results of the paired Condition × Time design. */
export function PairedResults(p: { snapshot: AnalysisSnapshot; result: StatisticalResults }) {
  const { snapshot: s, result: r } = p;
  const def = s.definition;
  const { a, b } = order(s);
  const u = s.unit ? ` (${s.unit})` : '';
  const nameA = def.factorAName || 'A', nameB = def.factorBName || 'B';
  const effects: [string, string][] = [
    ['A', t('stat.effect_condition', { name: nameA, levels: a.join(' / ') })],
    ['B', t('stat.effect_time', { name: nameB, levels: b.join(' / ') })],
    ['A:B', t('stat.effect_interaction', { a: nameA, b: nameB })],
  ];
  const adjustment = def.adjustment === 'bonferroni' ? 'Bonferroni' : 'Holm';
  const comparisons = r.comparisons;
  const across = comparisons.filter((c) => c.family === 'A_within_B');
  const units = [...new Set(r.groups.flatMap((g) => g.units || []))];
  const cell = (x: string, y: string) => r.groups.find((g) => g.factorA === x && g.factorB === y);
  const CompTable = (q: { list: typeof comparisons; by: 'time' | 'condition' }) => (
    <div style={{ overflowX: 'auto' }}>
      <table class="table">
        <thead>
          <tr>
            <th>{q.by === 'time' ? nameB : nameA}</th>
            <th>{t('stat.contrast')}</th>
            <th>{t('stat.estimated_difference')}{u}</th>
            <th>{t('stat.unadjusted_ci', { level: 100 * (1 - def.alpha) })}{u}</th>
            <th>t</th>
            <th>df</th>
            <th>p</th>
            <th>{t('stat.adjusted_p_named', { name: adjustment })}</th>
          </tr>
        </thead>
        <tbody>
          {[...q.list]
            // Rows in the design's level order; each contrast keeps the
            // direction the engine computed (right − left).
            .sort((x, y) => (q.by === 'time' ? b.indexOf(x.leftB || '') - b.indexOf(y.leftB || '') : Math.min(b.indexOf(x.leftB || ''), b.indexOf(x.rightB || '')) - Math.min(b.indexOf(y.leftB || ''), b.indexOf(y.rightB || '')) || Math.max(b.indexOf(x.leftB || ''), b.indexOf(x.rightB || '')) - Math.max(b.indexOf(y.leftB || ''), b.indexOf(y.rightB || ''))))
            .map((c) => (
            <tr>
              <td>{q.by === 'time' ? c.leftB : c.leftA}</td>
              <td>{q.by === 'time' ? `${c.rightA} − ${c.leftA}` : `${c.rightB} − ${c.leftB}`}</td>
              <td>{num(c.difference)}</td>
              <td>{num(c.lower)} … {num(c.upper)}</td>
              <td>{num(c.statistic)}</td>
              <td>{dfText(c.df)}</td>
              <td>{pvalue(c.p)}</td>
              <td>{pvalue(c.adjustedP)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
  return (
    <div class="col gap3 stat-paired">
      <Notice kind="info">{t('stat.small_n', { n: s.design.experimentalUnits, m: s.design.n })}</Notice>
      <section class="card pad col gap1">
        <b>{t('stat.paired_layout')}</b>
        <span>
          {t('stat.review_units', { n: s.design.experimentalUnits })} — {units.map((x) => unitName(s, x)).join(' · ')}
        </span>
        <span class="faint">{r.model ? t('stat.paired_mixed_scope') : t('stat.paired_rm_scope')}</span>
      </section>
      {effects.map(([term, title]) => (
        <section class="card pad col gap2 stat-effect" data-term={term}>
          <h3>{title}</h3>
          <EffectTable r={r} terms={r.terms} term={term} unit={s.unit} />
        </section>
      ))}
      {r.model && (
        <section class="card pad col gap2">
          <h3>{t('stat.mixed_model')}</h3>
          <p>
            {r.model.estimation} · y ~ {r.model.fixed} · {r.model.random} · {r.model.test}
          </p>
          <p>{t('stat.df_as_returned')}</p>
          <p>
            {(r.model.randomComponents || []).map((c) => `${t('stat.random_variance')} (${c.level}): ${num(c.variance)}${s.unit ? ' ' + squared(s.unit) : ''}`).join(' · ')} · {t('stat.residual_variance')}: {num(r.model.residualVariance)}
            {s.unit ? ' ' + squared(s.unit) : ''} · log likelihood: {num(r.model.logLikelihood)}
          </p>
          <div style={{ overflowX: 'auto' }}>
            <table class="table">
              <thead>
                <tr>
                  <th>{t('stat.coefficient')}</th>
                  <th>{t('stat.estimate')}{u}</th>
                  <th>SE{u}</th>
                  <th>df</th>
                  <th>CI{u}</th>
                </tr>
              </thead>
              <tbody>
                {r.model.fixedCoefficients.map((v) => (
                  <tr>
                    <td>{v.name}</td>
                    <td>{num(v.estimate)}</td>
                    <td>{num(v.se)}</td>
                    <td>{dfText(v.df)}</td>
                    <td>{num(v.lower)} … {num(v.upper)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
      {def.postHoc === 'simple_effects' && (
        <section class="card pad col gap2 stat-simple">
          <h3>{t('stat.simple_effects')}</h3>
          <p class="faint">{t('stat.simple_effects_scope', { name: adjustment })}</p>
          <h4>{t('stat.simple_across', { a: nameA, b: nameB })}</h4>
          <CompTable list={across} by="time" />
          {(r.simpleEffects || []).map((e) => (
            <div class="col gap2">
              <h4>{t('stat.simple_within', { b: nameB, level: e.factorA })}</h4>
              <EffectTable r={r} terms={e.terms} term="B" unit={s.unit} context={e.context} corrections={e.corrections} />
              <CompTable list={comparisons.filter((c) => c.family === 'B_within_A' && c.leftA === e.factorA)} by="condition" />
            </div>
          ))}
        </section>
      )}
      <section class="card pad col gap2 stat-units">
        <h3>{t('stat.unit_observations')}</h3>
        <p class="faint">{t('stat.unit_observations_help')}</p>
        <div style={{ overflowX: 'auto' }}>
          <table class="table">
            <thead>
              <tr>
                <th>{t('stat.unit')}</th>
                {a.flatMap((x) => b.map((y) => <th>{x} · {y}{u}</th>))}
              </tr>
            </thead>
            <tbody>
              {units.map((id) => (
                <tr>
                  <td>{unitName(s, id)}</td>
                  {a.flatMap((x) =>
                    b.map((y) => {
                      const g = cell(x, y);
                      const i = g?.units?.indexOf(id) ?? -1;
                      if (!g || i < 0) return <td class="faint">—</td>;
                      const n = g.readings?.[i] || 1;
                      return (
                        <td>
                          {num(g.values[i])}
                          {n > 1 && <small class="faint"> ({t('stat.review_cell_mean', { n })})</small>}
                        </td>
                      );
                    }),
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}
