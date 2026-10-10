import { useEffect, useRef, useState } from "preact/hooks";
import { get, post } from "../../lib/api";
import { statisticsHeaders, useAnalyses } from "../../lib/analyses";
import { fmtNum } from "../../lib/format";
import { errText, t } from "../../lib/i18n";
import {
  allMeasurements,
  measurementName,
  PARAMS,
  useFiles,
  useRev,
  warnText,
} from "../../lib/library";
import { navigate, useRoute } from "../../lib/route";
import { app, openPanel, run } from "../../lib/state";
import { useStore } from "../../lib/store";
import {
  calculateStatistics,
  type AnalysisDefinition,
  type AnalysisSnapshot,
  type StatisticalAnalysis,
  type StatisticalResults,
} from "../../lib/statistics";
import type { CycleView } from "../../lib/types";
import { Button, Empty, Notice, Skeleton, useAsync } from "../../ui/kit";
import { MeasurementPicker } from "./picker";

export function StatisticalModule(p: { id?: string }) {
  const s = useStore(app, (x) => x.s);
  return (
    <StatisticalPage
      key={`${s?.account?.id}/${s?.profile?.id}/${p.id || "library"}`}
      {...p}
    />
  );
}
function StatisticalPage(p: { id?: string }) {
  const rev = useRev();
  const { path } = useRoute();
  const cycleId = new URL(location.href).searchParams.get("cycle") || "";
  const saved = useAsync(
    () =>
      p.id && p.id !== "new"
        ? get<StatisticalAnalysis>("/api/statistics/" + p.id, {
            headers: statisticsHeaders(),
          })
        : Promise.resolve(null),
    [p.id, rev],
  );
  if (p.id === "new") return <AnalysisEditor cycleId={cycleId} />;
  if (p.id) {
    if (saved.error)
      return <Notice kind="danger">{errText(saved.error)}</Notice>;
    if (!saved.data) return <Skeleton h={200} />;
    return (
      <div class="col gap3 statistics-page">
        <div class="row gap2 wrap">
          <Button onClick={() => navigate("/ls/statistics")}>
            {t("stat.back")}
          </Button>
          <h2 class="grow">{saved.data.snapshot.definition.title}</h2>
          <Button
            onClick={() =>
              navigate("/ls/statistics/new?previous=" + saved.data!.id)
            }
          >
            {t("stat.recalculate")}
          </Button>
          <Button
            icon="download"
            onClick={() =>
              openPanel("export", {
                items: [{ kind: "analysis", id: saved.data!.id }],
              })
            }
          >
            {t("ui.export")}
          </Button>
          <Button
            icon="chart"
            onClick={async () => {
              const g = await run(() =>
                post<{ id: string }>("/api/graphs", {
                  kind: "statistical_groups",
                  analysisId: saved.data!.id,
                  errorBars: "sd",
                  title: saved.data!.snapshot.definition.title,
                }),
              );
              if (g) navigate("/ls/graphs/" + g.id);
            }}
          >
            {t("stat.graph")}
          </Button>
        </div>
        {saved.data.sourceChanged && (
          <Notice kind="warning">{t("stat.changed")}</Notice>
        )}
        <AnalysisResults
          snapshot={saved.data.snapshot}
          result={saved.data.results}
        />
      </div>
    );
  }
  return <AnalysisLibrary key={path} />;
}
function AnalysisLibrary() {
  const list = useAnalyses();
  const [q, setQ] = useState("");
  return (
    <div class="panel" style={{ flex: 1 }}>
      <div class="panel-head row gap3">
        <h2 class="grow">{t("stat.title")}</h2>
        <input
          class="input"
          aria-label={t("stat.search")}
          placeholder={t("stat.search")}
          value={q}
          onInput={(e) => setQ(e.currentTarget.value)}
        />
        <Button
          kind="primary"
          icon="plus"
          onClick={() => navigate("/ls/statistics/new")}
        >
          {t("stat.new")}
        </Button>
      </div>
      <div class="panel-body col gap3">
        {list.error && <Notice kind="danger">{errText(list.error)}</Notice>}
        {!list.data ? (
          <Skeleton h={160} />
        ) : !list.data.length ? (
          <Empty icon="sigma" title={t("stat.empty")} body={t("stat.help")} />
        ) : (
          list.data
            .filter((a) =>
              a.snapshot.definition.title
                .toLowerCase()
                .includes(q.toLowerCase()),
            )
            .map((a) => (
              <button
                class="card pad row gap3"
                onClick={() => navigate("/ls/statistics/" + a.id)}
              >
                <span class="grow">{a.snapshot.definition.title}</span>
                <span>
                  LIGHTSCATTERING · {t("stat.method." + a.results.method)} · n=
                  {a.snapshot.design.n}
                </span>
                {a.sourceChanged && <span>{t("stat.changed_short")}</span>}
              </button>
            ))
        )}
        <details>
          <summary>{t("stat.help_title")}</summary>
          <p>{t("stat.help")}</p>
          <p>{t("stat.limits")}</p>
          <p>{t("stat.modules")}</p>
        </details>
      </div>
    </div>
  );
}
function AnalysisEditor(p: { cycleId: string }) {
  const files = useFiles();
  const previousId = new URL(location.href).searchParams.get("previous") || "";
  const cycle = useAsync(
    () =>
      p.cycleId
        ? get<CycleView>("/api/cycles/" + p.cycleId)
        : Promise.resolve(null),
    [p.cycleId],
  );
  const units = useAsync(
    () =>
      get<{ id: string; label: string }[]>("/api/experimental-units", {
        headers: statisticsHeaders(),
      }),
    [],
  );
  const [unitLabel, setUnitLabel] = useState("");
  const [picking, setPicking] = useState(false);
  const [def, setDef] = useState<AnalysisDefinition>({
    schema: 1,
    module: "lightscattering",
    title: "",
    cycleId: p.cycleId || undefined,
    variable: "effective_diameter",
    factorAName: "",
    factorBName: "",
    structure: "",
    structureReviewed: false,
    method: "one_way",
    alpha: 0.05,
    postHoc: "none",
    sphericityCorrection: "GG",
    observations: [],
  });
  const [snapshot, setSnapshot] = useState<AnalysisSnapshot | null>(null),
    [result, setResult] = useState<StatisticalResults | null>(null),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const controller = useRef<AbortController | null>(null);
  useEffect(() => () => controller.current?.abort(), []);
  useEffect(() => {
    if (!cycle.data) return;
    const c = cycle.data;
    const observations = (c.items || []).map((m) => {
      const a = c.assignments?.find(
        (a) =>
          a.measurementId === m.id &&
          (a.status === "auto" || a.status === "confirmed"),
      );
      const point = a ? c.points?.find((p) => p.index === a.point) : null;
      return {
        measurementId: m.id,
        factorA: m.group || m.condition || "",
        factorB: point ? `${point.offset} ${point.unit}` : "",
        unitId: m.experimentalUnitId || "",
      };
    });
    setDef((d) => ({
      ...d,
      title: c.config.name,
      factorBName: t("stat.time"),
      method: "two_way",
      observations,
    }));
  }, [cycle.data]);
  useEffect(() => {
    if (!previousId) return;
    let active = true;
    get<StatisticalAnalysis>("/api/statistics/" + previousId, {
      headers: statisticsHeaders(),
    })
      .then((a) => {
        if (active) setDef(a.snapshot.definition);
      })
      .catch((e) => {
        if (active)
          setError(
            errText(
              (e as { code?: string }).code || "statistics.engine_failed",
            ),
          );
      });
    return () => {
      active = false;
    };
  }, [previousId]);
  const change = (patch: Partial<AnalysisDefinition>) => {
    setDef((d) => ({ ...d, ...patch }));
    setSnapshot(null);
    setResult(null);
    setError("");
  };
  const available = [
    ...allMeasurements(
      (files.data || []).filter((f) => f.module === "lightscattering"),
    ).values(),
  ].filter(({ m }) => !p.cycleId || cycle.data?.measurements.includes(m.id));
  const rows = available.filter(({ m }) =>
    def.observations.some((o) => o.measurementId === m.id),
  );
  const assignment = (
    id: string,
    patch: Partial<AnalysisDefinition["observations"][0]>,
  ) =>
    change({
      observations: def.observations.map((o) =>
        o.measurementId === id ? { ...o, ...patch } : o,
      ),
    });
  const prepare = async () => {
    setBusy(true);
    setError("");
    try {
      const s = await post<AnalysisSnapshot>("/api/statistics/prepare", def, {
        headers: statisticsHeaders(),
      });
      setSnapshot(s);
      setResult(null);
    } catch (e) {
      setError(
        errText((e as { code?: string }).code || "statistics.engine_failed"),
      );
    } finally {
      setBusy(false);
    }
  };
  const calculate = async () => {
    if (!snapshot) return;
    const abort = new AbortController();
    controller.current = abort;
    setBusy(true);
    setError("");
    try {
      setResult(await calculateStatistics(snapshot, abort.signal));
    } catch (e) {
      if (!abort.signal.aborted)
        setError(
          errText((e as { code?: string }).code || "statistics.engine_failed"),
        );
    } finally {
      controller.current = null;
      setBusy(false);
    }
  };
  const save = async () => {
    if (!snapshot || !result) return;
    setBusy(true);
    try {
      const a = await post<StatisticalAnalysis>(
        "/api/statistics",
        {
          definition: snapshot.definition,
          receipt: snapshot.receipt,
          results: result,
          previousId,
        },
        { headers: statisticsHeaders() },
      );
      navigate("/ls/statistics/" + a.id);
    } catch (e) {
      setError(
        errText((e as { code?: string }).code || "statistics.engine_failed"),
      );
    } finally {
      setBusy(false);
    }
  };
  const field = (
    key: "title" | "factorAName" | "factorBName",
    label: string,
  ) => (
    <label class="col gap1 grow">
      {t(label)}
      <input
        class="input"
        value={def[key] || ""}
        onInput={(e) => change({ [key]: e.currentTarget.value })}
        maxLength={key === "title" ? 240 : 120}
      />
    </label>
  );
  return (
    <div class="col gap3 statistics-page">
      <div class="row gap2">
        <Button onClick={() => navigate("/ls/statistics")}>
          {t("stat.back")}
        </Button>
        <h2>{t("stat.new")}</h2>
      </div>
      <Notice kind="info">{t("stat.help")}</Notice>
      <fieldset
        disabled={busy}
        class="card pad col gap3"
        style={{ border: "1px solid var(--border)" }}
      >
        <div class="row gap3 wrap">
          {field("title", "stat.name")}
          <label class="col gap1">
            {t("stat.variable")}
            <select
              class="input"
              value={def.variable}
              onChange={(e) => change({ variable: e.currentTarget.value })}
            >
              {PARAMS.map((k) => (
                <option value={k}>{t("param." + k)}</option>
              ))}
            </select>
          </label>
        </div>
        <div class="row gap3 wrap">
          {field("factorAName", "stat.factor_a")}
          {field("factorBName", "stat.factor_b")}
          <label class="col gap1">
            {t("stat.structure")}
            <select
              class="input"
              value={def.structure}
              onChange={(e) => {
                const structure = e.currentTarget
                  .value as AnalysisDefinition["structure"];
                change({
                  structure,
                  structureReviewed: false,
                  method:
                    structure === "repeated"
                      ? "repeated"
                      : def.observations.some((o) => o.factorB)
                        ? "two_way"
                        : "one_way",
                  postHoc: "none",
                  control: "",
                  sphericityCorrection: "GG",
                  effectCI: false,
                });
              }}
            >
              <option value="">{t("stat.choose")}</option>
              <option value="independent">{t("stat.independent")}</option>
              <option value="repeated">{t("stat.repeated")}</option>
            </select>
          </label>
        </div>
        <label class="row gap2">
          <input
            type="checkbox"
            checked={def.structureReviewed}
            onChange={(e) =>
              change({ structureReviewed: e.currentTarget.checked })
            }
          />
          {t("stat.review_structure")}
        </label>
        <div class="row gap3 wrap">
          <label class="col gap1">
            {t("stat.method")}
            <select
              class="input"
              value={def.method}
              onChange={(e) =>
                change({ method: e.currentTarget.value, effectCI: false, postHoc: "none", control: "", sphericityCorrection: e.currentTarget.value === "mixed" ? "" : "GG" })
              }
            >
              {(def.structure === "repeated"
                ? ["repeated", "mixed"]
                : ["one_way", "welch", "two_way"]
              ).map((k) => (
                <option value={k}>{t("stat.method." + k)}</option>
              ))}
            </select>
          </label>
          <label class="col gap1">
            α
            <input
              class="input"
              type="number"
              min="0.001"
              max={def.postHoc === "dunnett" ? "0.499" : "0.999"}
              step="0.001"
              value={def.alpha}
              onInput={(e) => change({ alpha: Number(e.currentTarget.value) })}
            />
          </label>
          <label class="col gap1">
            {t("stat.posthoc")}
            <select
              class="input"
              value={def.postHoc}
              onChange={(e) => change({ postHoc: e.currentTarget.value, control: "" })}
            >
              <option value="none">{t("stat.none")}</option>
              {["one_way", "two_way"].includes(def.method) && (
                <option value="tukey">Tukey HSD</option>
              )}
              {def.method === "one_way" && <option value="dunnett">Dunnett</option>}
            </select>
          </label>
          {def.postHoc === "dunnett" && (
            <label class="col gap1">
              {t("stat.control")}
              <select class="input" aria-label={t("stat.control")} value={def.control || ""} onChange={(e) => change({control:e.currentTarget.value})}>
                <option value="">{t("stat.choose")}</option>
                {[...new Set(def.observations.filter(o => !o.excludeReason).map(o => o.factorA))].filter(x => x.trim()).sort().map(x => <option value={x}>{x}</option>)}
              </select>
              <small>{t("stat.dunnett_scope")}</small>
            </label>
          )}
          {def.method === "repeated" && (
            <label class="col gap1">
              {t("stat.correction")}
              <select
                class="input"
                value={def.sphericityCorrection}
                onChange={(e) =>
                  change({ sphericityCorrection: e.currentTarget.value })
                }
              >
                <option value="GG">Greenhouse–Geisser</option>
                <option value="HF">Huynh–Feldt</option>
              </select>
            </label>
          )}
        </div>
        {def.method === "one_way" && <label class="col gap1">
          <span class="row gap2"><input type="checkbox" aria-label={t("stat.effect_ci")} checked={!!def.effectCI} onChange={e=>change({effectCI:e.currentTarget.checked})}/>{t("stat.effect_ci")}</span>
          <small>{t("stat.effect_ci_scope")}</small>
        </label>}
        {def.method === "mixed" && <Notice kind="info">{t("stat.mixed_scope")}</Notice>}
        <details>
          <summary>{t("stat.units")}</summary>
          <p>{t("stat.units_help")}</p>
          <div class="row gap2">
            <input
              class="input"
              aria-label={t("stat.unit_label")}
              value={unitLabel}
              onInput={(e) => setUnitLabel(e.currentTarget.value)}
            />
            <Button
              onClick={async () => {
                const u = await run(() =>
                  post<{ id: string; label: string }>(
                    "/api/experimental-units",
                    { label: unitLabel },
                    { headers: statisticsHeaders() },
                  ),
                );
                if (u) {
                  units.set([...(units.data || []), u]);
                  setUnitLabel("");
                }
              }}
            >
              {t("stat.create_unit")}
            </Button>
          </div>
        </details>
        <div class="row gap2">
          <Button icon="plus" onClick={() => setPicking(true)}>
            {t("stat.choose_measurements")}
          </Button>
          <span>{t("files.selected", { n: def.observations.length })}</span>
        </div>
        {picking && (
          <MeasurementPicker
            title={t("stat.choose_measurements")}
            initial={def.observations.map((o) => o.measurementId)}
            allowed={available.map(({ m }) => m.id)}
            onClose={() => setPicking(false)}
            onPick={(ids) => {
              setPicking(false);
              change({
                observations: ids.map(
                  (id) =>
                    def.observations.find((o) => o.measurementId === id) || {
                      measurementId: id,
                      factorA:
                        available.find(({ m }) => m.id === id)?.m.group ||
                        available.find(({ m }) => m.id === id)?.m.condition ||
                        "",
                      unitId:
                        available.find(({ m }) => m.id === id)?.m
                          .experimentalUnitId || "",
                    },
                ),
              });
            }}
          />
        )}
        <div style={{ overflowX: "auto" }}>
          <table class="table">
            <thead>
              <tr>
                {[
                  "stat.include",
                  "stat.measurement",
                  "stat.value",
                  "stat.factor_a",
                  "stat.factor_b",
                  "stat.unit",
                  "stat.exclude",
                ].map((k) => (
                  <th>{t(k)}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map(({ m, f }) => {
                const o = def.observations.find(
                  (v) => v.measurementId === m.id,
                );
                return (
                  <tr>
                    <td>
                      <input
                        type="checkbox"
                        aria-label={
                          t("stat.include") + " " + measurementName(m, f)
                        }
                        checked={!!o}
                        onChange={(e) =>
                          change({
                            observations: e.currentTarget.checked
                              ? [
                                  ...def.observations,
                                  {
                                    measurementId: m.id,
                                    factorA: m.group || m.condition || "",
                                    unitId: m.experimentalUnitId || "",
                                  },
                                ]
                              : def.observations.filter(
                                  (v) => v.measurementId !== m.id,
                                ),
                          })
                        }
                      />
                    </td>
                    <td>{measurementName(m, f)}</td>
                    <td>
                      {m.params[def.variable]
                        ? `${m.params[def.variable].raw} ${m.params[def.variable].unit || ""}`
                        : t("stat.missing")}
                    </td>
                    <td>
                      <input
                        class="input"
                        aria-label={
                          t("stat.factor_a") + " " + measurementName(m, f)
                        }
                        disabled={!o}
                        value={o?.factorA || ""}
                        onInput={(e) =>
                          assignment(m.id, { factorA: e.currentTarget.value })
                        }
                        style={{ width: 120 }}
                      />
                    </td>
                    <td>
                      <input
                        class="input"
                        aria-label={
                          t("stat.factor_b") + " " + measurementName(m, f)
                        }
                        disabled={!o}
                        value={o?.factorB || ""}
                        onInput={(e) =>
                          assignment(m.id, { factorB: e.currentTarget.value })
                        }
                        style={{ width: 120 }}
                      />
                    </td>
                    <td>
                      <select
                        class="input"
                        aria-label={
                          t("stat.unit") + " " + measurementName(m, f)
                        }
                        disabled={!o}
                        value={o?.unitId || ""}
                        onChange={(e) =>
                          assignment(m.id, { unitId: e.currentTarget.value })
                        }
                      >
                        <option value="">{t("stat.unassigned")}</option>
                        {units.data?.map((u) => (
                          <option value={u.id}>{u.label}</option>
                        ))}
                      </select>
                    </td>
                    <td>
                      <input
                        class="input"
                        aria-label={
                          t("stat.exclude") + " " + measurementName(m, f)
                        }
                        disabled={!o}
                        value={o?.excludeReason || ""}
                        onInput={(e) =>
                          assignment(m.id, {
                            excludeReason: e.currentTarget.value,
                          })
                        }
                        style={{ width: 140 }}
                      />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
        <Notice kind="warning">{t("stat.limits")}</Notice>
        <Button
          kind="primary"
          disabled={!def.structureReviewed || def.observations.length < 3}
          onClick={prepare}
        >
          {t("stat.prepare")}
        </Button>
      </fieldset>
      {error && <Notice kind="danger">{error}</Notice>}
      {snapshot && (
        <div class="card pad col gap2">
          <b>{t("stat.design")}</b>
          <span>
            n={snapshot.design.n} · {t("stat.missing")}:{" "}
            {snapshot.design.missing} · {t("stat.excluded")}:{" "}
            {snapshot.design.excluded} · {t("stat.recommend")}:{" "}
            {t("stat.method." + snapshot.design.recommended)}
          </span>
          {snapshot.design.warnings.map((w) => (
            <Notice kind="warning">{warnText(w)}</Notice>
          ))}
          <div class="row gap2">
            <Button kind="primary" disabled={busy} onClick={calculate}>
              {busy ? t("stat.running") : t("stat.run")}
            </Button>
            {busy && (
              <Button onClick={() => controller.current?.abort()}>
                {t("ui.cancel")}
              </Button>
            )}
            {result && (
              <Button kind="primary" disabled={busy} onClick={save}>
                {t("stat.save")}
              </Button>
            )}
          </div>
        </div>
      )}
      {snapshot && result && (
        <AnalysisResults snapshot={snapshot} result={result} />
      )}
    </div>
  );
}
const num = (n: number | undefined | null) => (n == null ? "—" : fmtNum(n, 6));
const pvalue = (n: number | undefined | null) =>
  n == null
    ? "—"
    : n === 0
      ? t("stat.p_underflow")
      : n < 0.000001
        ? n.toExponential(4)
        : fmtNum(n, 6);
function AnalysisResults(p: {
  snapshot: AnalysisSnapshot;
  result: StatisticalResults;
}) {
  const { snapshot: s, result: r } = p;
  return (
    <div class="col gap3">
      <div class="card pad">
        <b>{t("stat.derived")}</b>
        <p>
          {t("stat.method." + r.method)} · {t("param." + s.definition.variable)}{" "}
          ({s.unit || "—"}) · α={s.definition.alpha} · {r.model ? t("stat.mixed_tests") : r.ssType}
        </p>
        <small>
          {r.engine} · {r.calculation}
        </small>
      </div>
      {r.warnings.map((w) => (
        <Notice kind="warning">{warnText(w)}</Notice>
      ))}
      <section class="card pad col gap2">
        <h3>{t(r.model ? "stat.mixed_tests" : "stat.anova_table")}</h3>
        <div style={{ overflowX: "auto" }}>
          <table class="table">
            <thead>
              <tr>
                {(r.model ? [t("stat.source"), "df", t("stat.denominator_df"), "F", "p"] : [
                  "Source",
                  "SS",
                  "df",
                  "MS",
                  "F",
                  "p",
                  "η²",
                  "partial η²",
                  "ω²",
                ]).map((x) => (
                  <th>{x}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {r.terms.map((x) => (
                <tr>
                  <td>{x.source}</td>
                  {(r.model ? [x.df, x.denominatorDF, x.f] : [x.ss, x.df, x.ms, x.f]).map((n) => (
                    <td>{num(n)}</td>
                  ))}
                  <td>{pvalue(x.p)}</td>
                  {!r.model && [x.etaSquared, x.partialEtaSquared, x.omegaSquared].map(
                    (n) => (
                      <td>{num(n)}</td>
                    ),
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
      {!!r.effectIntervals?.length && <section class="card pad col gap2">
        <h3>{t("stat.effect_ci")}</h3><p>{t("stat.effect_ci_scope")}</p>
        <table class="table"><thead><tr><th>{t("stat.source")}</th><th>{t("stat.confidence_level")}</th><th>CI</th><th>{t("stat.status")}</th></tr></thead><tbody>
          {r.effectIntervals.map(ci=><tr><td>{ci.source} · η²</td><td>{100*ci.confidenceLevel}%</td><td>{num(ci.lower)} … {num(ci.upper)}</td><td>{t("stat.effect_ci."+ci.status)}</td></tr>)}
        </tbody></table>
        <small>{r.effectIntervals[0].method}</small>
      </section>}
      {r.model && <section class="card pad col gap2">
        <h3>{t("stat.mixed_model")}</h3>
        <p>{t("stat.mixed_scope")}</p>
        <p>{r.model.estimation} · y ~ {r.model.fixed} · {r.model.random} · {r.model.test}</p>
        <p>{t("stat.random_variance")}: {num(r.model.randomVariance)} · {t("stat.residual_variance")}: {num(r.model.residualVariance)} · log likelihood: {num(r.model.logLikelihood)}</p>
        <p>A: {r.model.levelsA.join(" · ")} · B: {r.model.levelsB.join(" · ")}</p>
        {!!r.model.coefficientConfidenceLevel && <p>{t("stat.mixed_coefficient_ci_scope")} · {100*r.model.coefficientConfidenceLevel}%</p>}
        <table class="table"><thead><tr><th>{t("stat.coefficient")}</th><th>{t("stat.estimate")}</th><th>SE</th>{!!r.model.coefficientConfidenceLevel && <><th>df</th><th>CI</th></>}</tr></thead><tbody>
          {r.model.fixedCoefficients.map(v=><tr><td>{v.name}</td><td>{num(v.estimate)}</td><td>{num(v.se)}</td>{!!r.model!.coefficientConfidenceLevel && <><td>{num(v.df)}</td><td>{num(v.lower)} … {num(v.upper)}</td></>}</tr>)}
        </tbody></table>
      </section>}
      <section class="card pad col gap2">
        <h3>{t("stat.groups")}</h3>
        <table class="table">
          <thead>
            <tr>
              <th>{t("stat.group")}</th>
              <th>n</th>
              <th>{t("stat.mean")}</th>
              <th>SD</th>
              <th>SEM</th>
              <th>CI ({100 * (1 - s.definition.alpha)}%)</th>
            </tr>
          </thead>
          <tbody>
            {r.groups.map((g) => (
              <tr>
                <td>
                  {g.factorA} {g.factorB}
                </td>
                <td>{g.n}</td>
                <td>{num(g.mean)}</td>
                <td>{num(g.sd)}</td>
                <td>{num(g.sem)}</td>
                <td>
                  {num(g.ciLower)} … {num(g.ciUpper)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <details>
          <summary>{t("stat.individuals")}</summary>
          {r.groups.map((g) => (
            <p>
              {g.factorA} {g.factorB}: {g.values.map((v) => num(v)).join(" · ")}
            </p>
          ))}
        </details>
      </section>
      {r.comparisons.length > 0 && (
        <section class="card pad">
          <h3>{t("stat.posthoc")}</h3>
          <table class="table">
            <thead>
              <tr>
                <th>{t("stat.contrast")}</th>
                <th>{t("stat.difference")}</th>
                <th>CI</th>
                <th>{t("stat.adjusted_p")}</th>
              </tr>
            </thead>
            <tbody>
              {r.comparisons.map((c) => (
                <tr>
                  <td>
                    {c.context} {c.contrast}
                    <small class="faint"> · {c.correction}</small>
                  </td>
                  <td>{num(c.difference)}</td>
                  <td>
                    {num(c.lower)} … {num(c.upper)}
                  </td>
                  <td>{pvalue(c.adjustedP)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
      <section class="card pad col gap2">
        <h3>{t("stat.diagnostics")}</h3>
        <Notice kind="info">{t("stat.diagnostics_help")}</Notice>
        {r.diagnostics.map((d) => (
          <div class="col gap1">
          <div class="row gap3">
            <b>{t("stat.diagnostic." + d.code)}</b>
            <span>
              {num(d.statistic)} · p={pvalue(d.p)}
            </span>
          </div>
          {d.details && <details><summary>{t("stat.diagnostic_details")}</summary><p>{d.details}</p></details>}
          </div>
        ))}
        {r.corrections && (
          <p>
            {Object.entries(r.corrections)
              .map(([k, v]) => `${k} ε=${num(v)}`)
              .join(" · ")}
          </p>
        )}
        <DiagnosticPlots r={r} />
      </section>
      <details class="card pad">
        <summary>{t("stat.provenance")}</summary>
        <p>SHA-256: {s.sourceHash}</p>
        <p>{t("stat.units_help")}</p>
        {s.sources.map((src) => (
          <p>
            {src.file.name} · {src.file.module} · {src.measurement.parser} ·{" "}
            {src.measurement.spec}
            <br />
            {src.measurement.sourceSheet} {src.measurement.sourceRange}
            <br />
            SHA-256: {src.file.sha256}
          </p>
        ))}
      </details>
    </div>
  );
}
function DiagnosticPlots(p: { r: StatisticalResults }) {
  const r = p.r;
  const min = Math.min(...r.qqObserved),
    max = Math.max(...r.qqObserved),
    xmin = Math.min(...r.qqTheoretical),
    xmax = Math.max(...r.qqTheoretical);
  const span = max - min || 1;
  const bins = Array(10).fill(0);
  for (const v of r.residuals)
    bins[Math.min(9, Math.floor(((v - min) / span) * 10))]++;
  return (
    <div class="row gap3 wrap">
      <figure>
        <figcaption>{t("stat.qq")}</figcaption>
        <svg
          role="img"
          aria-label={t("stat.qq")}
          viewBox="0 0 320 180"
          width="320"
          height="180"
          style={{ maxWidth: "100%" }}
        >
          <path d="M30 10V150H310" fill="none" stroke="currentColor" />
          {r.qqObserved.map((y, i) => (
            <circle
              cx={35 + ((r.qqTheoretical[i] - xmin) / (xmax - xmin || 1)) * 260}
              cy={145 - ((y - min) / span) * 125}
              r="3"
              fill="var(--accent)"
            />
          ))}
        </svg>
        <small>{t("stat.qq_axes")}</small>
      </figure>
      <figure>
        <figcaption>{t("stat.histogram")}</figcaption>
        <svg
          role="img"
          aria-label={t("stat.histogram")}
          viewBox="0 0 320 180"
          width="320"
          height="180"
          style={{ maxWidth: "100%" }}
        >
          <path d="M30 10V150H310" fill="none" stroke="currentColor" />
          {bins.map((n, i) => (
            <rect
              x={35 + i * 26}
              y={150 - (n / Math.max(...bins, 1)) * 125}
              width="24"
              height={(n / Math.max(...bins, 1)) * 125}
              fill="var(--accent)"
            />
          ))}
        </svg>
        <small>
          {num(min)} … {num(max)} · {t("stat.residual_frequency")}
        </small>
      </figure>
    </div>
  );
}
