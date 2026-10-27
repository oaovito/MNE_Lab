// Paired Condition × Time design: the actual bundled R engine against the
// independent Pingouin/Statsmodels/SciPy golden values written by
// scripts/statistics_paired_oracle.py. Every value is invented.
import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import { gunzipSync } from "node:zlib";
import { WebR } from "webr";

const golden = JSON.parse(fs.readFileSync(new URL("./fixtures/statistics-paired-golden.json", import.meta.url)));
const code = fs.readFileSync(new URL("../src/lib/statistics-engine.R", import.meta.url), "utf8");
const r = new WebR({ interactive: false });
await r.init();
await r.evalRVoid(code);
async function calculate(input) {
  await r.objs.globalEnv.bind("mne_input", await new r.RList(input));
  return JSON.parse(await r.evalRString("mne_json(mne_engine(mne_input))"));
}
function near(actual, expected, label, tolerance) {
  assert.ok(typeof actual === "number" && Number.isFinite(actual), `${label}: not finite (${actual})`);
  assert.ok(Math.abs(actual - expected) <= tolerance * Math.max(1, Math.abs(expected)), `${label}: ${actual} != ${expected}`);
}
// A pseudoreplicated two-way ANOVA of the 24 owner measurements would give
// residual df 16 (24 - 8 cells); other pooled analyses of the 24 values give
// 18, 22 or 23. These constants are test sentinels only, never product values.
const PSEUDOREPLICATED_DF = [16, 18, 22, 23];

function compare(c, actual) {
  const e = c.expected, name = c.name, mixed = c.input.method === "mixed";
  const tol = mixed ? 1e-5 : 1e-8, ptol = mixed ? 1e-6 : 1e-8;
  assert.equal(actual.design, "within_within");
  assert.equal(actual.terms.length, e.terms.length, `${name}: term count`);
  for (const t of e.terms) {
    const a = actual.terms.filter((x) => x.source === t.source);
    assert.equal(a.length, 1, `${name}: term ${t.source}`);
    for (const [k, v] of Object.entries(t)) {
      if (k === "source") continue;
      // Integer df are exact; corrected (epsilon-scaled) df are compared numerically.
      if ((k === "df" || k === "denominatorDF") && Number.isInteger(v)) assert.equal(a[0][k], v, `${name}.${t.source}.${k}`);
      else near(a[0][k], v, `${name}.${t.source}.${k}`, k === "p" ? ptol : tol);
    }
  }
  assert.deepEqual(Object.keys(actual.corrections || {}).sort(), Object.keys(e.corrections || {}).sort(), `${name}: correction keys`);
  for (const [k, v] of Object.entries(e.corrections || {})) near(actual.corrections[k], v, `${name}.${k}`, 1e-8);
  for (const d of e.diagnostics) {
    const a = actual.diagnostics.filter((x) => x.code === d.code && (!d.details || x.details === d.details || (x.details || "").startsWith(d.details + "; residual")));
    assert.equal(a.length, 1, `${name}: diagnostic ${d.code} ${d.details || ""}`);
    for (const k of ["statistic", "p"]) if (k in d) near(a[0][k], d[k], `${name}.${d.code}.${k}`, tol);
  }
  assert.equal(actual.groups.length, e.groups.length);
  for (const g of e.groups) {
    const a = actual.groups.find((x) => x.factorA === g.factorA && x.factorB === g.factorB);
    assert.ok(a, `${name}: group ${g.factorA} ${g.factorB}`);
    assert.equal(a.n, g.n);
    assert.deepEqual(a.units, g.units);
    assert.deepEqual(a.readings, g.readings);
    g.values.forEach((v, i) => near(a.values[i], v, `${name}.group.value`, 1e-12));
    for (const k of ["mean", "sd", "sem", "ciLower", "ciUpper"]) near(a[k], g[k], `${name}.group.${k}`, 1e-8);
  }
  assert.equal(actual.comparisons.length, e.comparisons.length, `${name}: comparison count`);
  for (const c2 of e.comparisons) {
    const a = actual.comparisons.filter((x) => ["leftA", "leftB", "rightA", "rightB", "family", "context", "contrast"].every((k) => x[k] === c2[k]));
    assert.equal(a.length, 1, `${name}: comparison ${c2.contrast} ${c2.context}`);
    assert.equal(a[0].n, c2.n);
    assert.match(a[0].correction, new RegExp(`^${c.input.adjustment === "holm" ? "Holm" : "Bonferroni"}; paired t; individual CI$`));
    for (const k of ["difference", "lower", "upper", "statistic", "df", "p", "adjustedP"]) near(a[0][k], c2[k], `${name}.comparison.${k}`, 1e-8);
  }
  assert.equal((actual.simpleEffects || []).length, e.simpleEffects.length);
  for (const s of e.simpleEffects) {
    const a = actual.simpleEffects.find((x) => x.context === s.context);
    assert.ok(a && a.factorA === s.factorA, `${name}: simple effect ${s.context}`);
    assert.equal(a.terms.length, s.terms.length);
    for (const t of s.terms) {
      const at = a.terms.find((x) => x.source === t.source);
      assert.ok(at, `${name}.${s.context}.${t.source}`);
      for (const [k, v] of Object.entries(t)) if (k !== "source") near(at[k], v, `${name}.${s.context}.${t.source}.${k}`, 1e-8);
    }
    for (const [k, v] of Object.entries(s.corrections)) near(a.corrections[k], v, `${name}.${s.context}.${k}`, 1e-8);
  }
  assert.equal(actual.residuals.length, e.cells);
  e.residuals.forEach((v, i) => near(actual.residuals[i], v, `${name}.residual.${i}`, tol));
  // Addendum B 60: the experimental units are the subjects, never the measurements.
  const units = new Set(actual.groups.flatMap((g) => g.units));
  assert.equal(units.size, e.experimentalUnits);
  assert.notEqual(units.size, e.measurements);
  if (e.experimentalUnits === 3)
    for (const t of actual.terms) for (const k of ["df", "denominatorDF"]) assert.ok(!PSEUDOREPLICATED_DF.includes(t[k]), `${name}: pseudoreplicated df ${t.source}.${k}=${t[k]}`);
}

try {
  for (const c of golden.cases.filter((c) => !c.ciOnly))
    await test(`independent paired oracle: ${c.name}`, async () => {
      const actual = await calculate(c.input);
      assert.equal(actual.engine, "webR/0.6.0; R/4.6.0; paired-rm/1" + (c.input.postHoc === "simple_effects" ? "; paired-simple-effects/1" : ""));
      assert.equal(actual.ssType, "repeated_within_within_strata");
      compare(c, actual);
      assert.deepEqual(await calculate(c.input), actual);
    });
  await test("B§60: 24 measurements of 3 units are 3 subjects, never 24", async () => {
    const c = golden.cases.find((c) => c.name === "owner_3x2x4_repeated_gg_holm");
    assert.equal(c.input.values.length, 24);
    const actual = await calculate(c.input);
    assert.equal(new Set(actual.groups.flatMap((g) => g.units)).size, 3);
    assert.ok(actual.groups.every((g) => g.n === 3));
    const oracleA = c.expected.terms.find((t) => t.source === "A");
    const engineA = actual.terms.find((t) => t.source === "A");
    assert.equal(engineA.denominatorDF, oracleA.denominatorDF);
    assert.equal(engineA.denominatorDF, actual.terms.find((t) => t.source === "residual_unit:A").df);
    for (const t of actual.terms) assert.ok(![16, 18].includes(t.denominatorDF) && ![16, 18].includes(t.df), `pseudoreplicated df in ${t.source}`);
    assert.ok(actual.diagnostics.some((d) => d.code === "mauchly_not_estimable"));
    // Untestable sphericity is said; the conservative reading is the GG, HF
    // and lower-bound results, never an automatic switch to the mixed model,
    // whose pooled within-unit error assumes more (compound symmetry).
    assert.ok(actual.warnings.includes("statistics.sphericity_not_testable"));
    assert.ok(!actual.warnings.includes("statistics.mixed_model_recommended"));
    for (const s of ["B_GG", "B_LB", "A:B_GG", "A:B_LB"]) assert.ok(actual.terms.some((t) => t.source === s), s);
  });
  await test("technical readings are averaged per cell and never counted", async () => {
    const c = golden.cases.find((c) => c.name === "technical_readings_repeated_gg_holm");
    const actual = await calculate(c.input);
    assert.equal(c.input.values.length, 28);
    assert.equal(actual.residuals.length, 24);
    assert.ok(actual.groups.every((g) => g.n === 3));
    assert.equal(actual.groups.reduce((n, g) => n + g.readings.reduce((a, b) => a + b, 0), 0), 28);
    assert.ok(actual.warnings.includes("statistics.technical_readings_averaged"));
    near(actual.diagnostics.find((d) => d.code === "technical_readings_averaged").statistic, 3, "reading cells", 0);
  });
  await test("labels never collide when readings are grouped", async () => {
    const base = golden.cases.find((c) => c.name === "owner_3x2x4_repeated_hf").input;
    const relabel = { "unit-A": "a.b", "unit-B": "a", "unit-C": "b.c" };
    const actual = await calculate({ ...base, unitId: base.unitId.map((u) => relabel[u]) });
    assert.equal(new Set(actual.groups.flatMap((g) => g.units)).size, 3);
    assert.equal(actual.residuals.length, 24);
  });
  await test("time levels follow the order the core gives, never text order", async () => {
    const base = golden.cases.find((c) => c.name === "owner_3x2x4_repeated_gg_holm").input;
    const days = [...new Set(base.factorB)].sort();
    const relabel = Object.fromEntries(days.map((d, i) => [d, ["0", "7", "14", "21"][i]]));
    const numeric = { ...base, factorB: base.factorB.map((b) => relabel[b]) };
    const ordered = await calculate({ ...numeric, levelsA: ["F-SR", "C-SR"].filter((a) => base.factorA.includes(a)).length === 2 ? ["F-SR", "C-SR"] : [...new Set(base.factorA)], levelsB: ["0", "7", "14", "21"] });
    assert.deepEqual([...new Set(ordered.groups.map((g) => g.factorB))], ["0", "7", "14", "21"]);
    const within = ordered.comparisons.filter((c) => c.family === "B_within_A").slice(0, 6).map((c) => c.contrast);
    assert.deepEqual(within, ["7-0", "14-0", "14-7", "21-0", "21-7", "21-14"]);
    // The order arranges levels only: every test is the same.
    const text = await calculate(numeric);
    for (const t of text.terms) near(ordered.terms.find((x) => x.source === t.source)[t.f === undefined ? "ss" : "f"], t.f === undefined ? t.ss : t.f, t.source, 1e-10);
    await assert.rejects(calculate({ ...numeric, levelsB: ["0", "7", "14"] }), /statistics.invalid_definition/);
  });
  await test("the interaction warning reads the correction that was applied", async () => {
    // Invented 4 units x 2 conditions x 4 times: A:B uncorrected p = 0.035,
    // Greenhouse-Geisser p = 0.088.
    const y = [99.4, 100.1, 101.2, 103.8, 92.2, 97, 95, 99.7, 99.3, 101.8, 102.6, 103, 94, 95.4, 99.3, 103, 100.6, 101.1, 103.2, 102, 95.8, 96.4, 100.3, 105.3, 98.6, 98, 102, 103, 94.8, 95.8, 95.2, 102.1];
    const units = ["u1", "u2", "u3", "u4"].flatMap((u) => Array(8).fill(u));
    const a = units.map((_, i) => (i % 8 < 4 ? "F" : "C"));
    const b = units.map((_, i) => ["0", "7", "14", "21"][i % 4]);
    const input = { values: y, factorA: a, factorB: b, unitId: units, alpha: 0.05, method: "repeated", postHoc: "none", effectCI: false, control: "", structure: "repeated", correction: "GG", design: "within_within", adjustment: "", levelsA: ["F", "C"], levelsB: ["0", "7", "14", "21"] };
    const gg = await calculate(input);
    const p = (s) => gg.terms.find((t) => t.source === s).p;
    assert.ok(p("A:B") < 0.05 && p("A:B_GG") > 0.05, `${p("A:B")} ${p("A:B_GG")}`);
    assert.ok(!gg.warnings.includes("statistics.interaction_requires_simple_effects"));
  });
  for (const c of golden.rejections.filter((c) => !c.ciOnly))
    await test(`paired rejection: ${c.name}`, async () => await assert.rejects(calculate(c.input), new RegExp(`statistics.${c.error}`)));
  await test("original designs refuse paired-only options", async () => {
    const base = golden.cases[0].input;
    await assert.rejects(calculate({ ...base, design: "nested" }), /statistics.invalid_definition/);
    const legacy = { ...base, design: "", method: "repeated", factorA: base.factorA.map(() => "A"), postHoc: "simple_effects", adjustment: "holm" };
    await assert.rejects(calculate(legacy), /statistics.incompatible_posthoc/);
  });
  // nlme is a bundled package. These run against the same read-only local
  // package image as browser workers; no download may supply them.
  await r.FS.mkdir("/mne-packages");
  await r.FS.mount("WORKERFS", { packages: [{
    metadata: JSON.parse(fs.readFileSync(new URL("../dist/statistics-engine/packages.metadata.json", import.meta.url))),
    blob: new Uint8Array(gunzipSync(fs.readFileSync(new URL("../dist/statistics-engine/packages.data.gz", import.meta.url)))),
  }] }, "/mne-packages");
  await r.evalRVoid('.libPaths(c("/mne-packages", .libPaths()))');
  for (const c of golden.cases.filter((c) => c.ciOnly))
    await test(`independent paired Mixed oracle: ${c.name}`, async () => {
      const actual = await calculate(c.input);
      assert.equal(actual.engine, "webR/0.6.0; R/4.6.0; nlme/3.1-169; mixed-within2/1" + (c.input.postHoc === "simple_effects" ? "; paired-simple-effects/1" : ""));
      assert.equal(actual.ssType, "not_applicable_marginal_Wald_F");
      const m = actual.model, e = c.expected.model;
      assert.equal(m.family, "nested_random_intercepts");
      assert.equal(m.estimation, "REML");
      assert.equal(m.random, "1|unit/A");
      // The pooled within-unit error is stated with every paired mixed result.
      assert.ok(actual.warnings.includes("statistics.paired_mixed_pooled_error"));
      assert.ok(!actual.warnings.includes("statistics.mixed_model_recommended"));
      assert.deepEqual(m.levelsA, e.levelsA);
      assert.deepEqual(m.levelsB, e.levelsB);
      for (const k of ["randomVariance", "residualVariance"]) near(m[k], e[k], `${c.name}.${k}`, 1e-5);
      e.randomComponents.forEach((v, i) => { assert.equal(m.randomComponents[i].level, v.level); near(m.randomComponents[i].variance, v.variance, `${c.name}.${v.level}`, 1e-5); });
      near(m.logLikelihood, e.logLikelihood, `${c.name}.logLikelihood`, 1e-6 / Math.max(1, Math.abs(e.logLikelihood)));
      assert.equal(m.fixedCoefficients.length, e.fixedCoefficients.length);
      for (const v of e.fixedCoefficients) {
        const a = m.fixedCoefficients.find((x) => x.name === v.name);
        assert.ok(a, v.name);
        assert.equal(a.df, v.df);
        for (const k of ["estimate", "se", "lower", "upper"]) near(a[k], v[k], `${c.name}.${v.name}.${k}`, 1e-5);
      }
      for (const t of actual.terms) for (const k of ["ss", "ms", "etaSquared", "partialEtaSquared", "generalizedEtaSquared"]) assert.equal(t[k], undefined);
      assert.equal(actual.corrections, null);
      compare(c, actual);
      assert.deepEqual(await calculate(c.input), actual);
    });
  for (const c of golden.rejections.filter((c) => c.ciOnly))
    await test(`paired Mixed rejection: ${c.name}`, async () => await assert.rejects(calculate(c.input), new RegExp(`statistics.${c.error}`)));
} finally {
  r.close();
}
