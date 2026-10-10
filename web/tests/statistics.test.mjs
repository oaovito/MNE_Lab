import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import { gunzipSync } from "node:zlib";
import { WebR } from "webr";

const golden = JSON.parse(
  fs.readFileSync(
    new URL("./fixtures/statistics-golden.json", import.meta.url),
  ),
);
const code = fs.readFileSync(
  new URL("../src/lib/statistics-engine.R", import.meta.url),
  "utf8",
);
const r = new WebR({ interactive: false });
await r.init();
await r.evalRVoid(code);
async function calculate(input) {
  const obj = await new r.RList(input);
  await r.objs.globalEnv.bind("mne_input", obj);
  return JSON.parse(await r.evalRString("mne_json(mne_engine(mne_input))"));
}
function near(actual, expected, label, tolerance = 1e-8) {
  assert.ok(
    typeof actual === "number" && Number.isFinite(actual),
    `${label}: not finite`,
  );
  assert.ok(
    Math.abs(actual - expected) <= tolerance * Math.max(1, Math.abs(expected)),
    `${label}: ${actual} != ${expected}`,
  );
}
try {
  for (const c of golden.cases)
    await test(`independent oracle: ${c.name}`, async () => {
      const actual = await calculate(c.input);
      assert.equal(actual.engine, "webR/0.6.0; R/4.6.0");
      for (const family of ["terms", "groups", "comparisons", "diagnostics"])
        for (const expected of c.expected[family]) {
          const id =
            family === "terms"
              ? "source"
              : family === "comparisons"
                ? "contrast"
                : family === "diagnostics"
                  ? "code"
                  : "factorA";
          const match = actual[family].find(
            (v) =>
              v[id] === expected[id] &&
              (family !== "groups" || v.factorB === expected.factorB) &&
              (!expected.context || v.context === expected.context),
          );
          assert.ok(match, `${family} ${expected[id]} missing`);
          // R documents qtukey accuracy to the fourth decimal place. Its extreme
          // small-df confidence limits need a separate tolerance from SS/F/p.
          // https://stat.ethz.ch/R-manual/R-devel/library/stats/html/Tukey.html
          for (const [k, v] of Object.entries(expected))
            if (typeof v === "number")
              near(
                match[k],
                v,
                `${c.name}.${expected[id]}.${k}`,
                family === "comparisons" && ["lower", "upper"].includes(k)
                  ? 1e-4
                  : 1e-8,
              );
        }
      for (const [k, v] of Object.entries(c.expected.corrections))
        near(actual.corrections[k], v, `${c.name}.${k}`);
      assert.equal(actual.residuals.length, c.input.values.length);
      for (const comparison of actual.comparisons) {
        assert.match(comparison.id, /^comparison-\d+$/);
        const left = actual.groups.find(
          (g) =>
            g.factorA === comparison.leftA &&
            (g.factorB || "") === (comparison.leftB || ""),
        );
        const right = actual.groups.find(
          (g) =>
            g.factorA === comparison.rightA &&
            (g.factorB || "") === (comparison.rightB || ""),
        );
        assert.ok(
          left && right,
          `${c.name}: explicit comparison groups missing`,
        );
        near(
          comparison.difference,
          right.mean - left.mean,
          `${c.name}: comparison pair direction`,
        );
      }
    });
  for (const [name, input, error] of [
    [
      "constant",
      { values: [1, 1, 1, 1], factorA: ["A", "A", "B", "B"] },
      "zero_residual_variance",
    ],
    [
      "tiny groups",
      { values: [1, 2, 3], factorA: ["A", "A", "B"] },
      "insufficient_group_size",
    ],
    [
      "non-finite",
      { values: [1, 2, NaN, 4], factorA: ["A", "A", "B", "B"] },
      "invalid_numeric",
    ],
  ])
    await test(`scientific rejection: ${name}`, async () => {
      const n = input.values.length;
      await assert.rejects(
        calculate({
          ...golden.cases[0].input,
          factorB: Array(n).fill(""),
          unitId: Array(n).fill(""),
          ...input,
        }),
        new RegExp(`statistics.${error}`),
      );
    });
  await test("user labels cannot collide when grouping factor pairs", async () => {
    const input = {
      ...golden.cases.find((c) => c.input.method === "two_way").input,
      postHoc: "none",
      factorA: [
        "a.b",
        "a.b",
        "a.b",
        "a.b",
        "a.b",
        "a.b",
        "a",
        "a",
        "a",
        "a",
        "a",
        "a",
      ],
      factorB: [
        "c",
        "c",
        "c",
        "b.c",
        "b.c",
        "b.c",
        "c",
        "c",
        "c",
        "b.c",
        "b.c",
        "b.c",
      ],
    };
    const actual = await calculate(input);
    assert.equal(actual.groups.length, 4);
    assert.ok(actual.groups.every((g) => g.n === 3));
  });
  await test("comparison identities preserve punctuation and four-level Tukey order", async () => {
    const labels = ["A-B", "B-C", "C.D", "D.E"];
    const input = {
      ...golden.cases[0].input,
      values: [1, 2, 4, 5, 6, 9, 10, 12, 14, 18, 20, 23],
      factorA: labels.flatMap((x) => [x, x, x]),
      factorB: Array(12).fill(""),
      unitId: Array(12).fill(""),
    };
    const actual = await calculate(input);
    assert.equal(actual.comparisons.length, 6);
    for (const c of actual.comparisons) {
      const left = actual.groups.find((g) => g.factorA === c.leftA),
        right = actual.groups.find((g) => g.factorA === c.rightA);
      assert.ok(left && right);
      near(c.difference, right.mean - left.mean, c.contrast);
    }
  });
  // Mount the same read-only compressed image used by browser workers. No
  // NODEFS shortcut or package download may supply these calculation tests.
  await r.FS.mkdir("/mne-packages");
  await r.FS.mount("WORKERFS", { packages: [{
    metadata: JSON.parse(fs.readFileSync(new URL("../dist/statistics-engine/packages.metadata.json", import.meta.url))),
    blob: new Uint8Array(gunzipSync(fs.readFileSync(new URL("../dist/statistics-engine/packages.data.gz", import.meta.url)))),
  }] }, "/mne-packages");
  await r.evalRVoid('.libPaths(c("/mne-packages", .libPaths()))');
  const advanced=JSON.parse(fs.readFileSync(new URL("./fixtures/statistics-advanced-golden.json",import.meta.url)));
  for(const c of advanced.cases) await test(`independent Dunnett oracle: ${c.name}`,async()=>{
    const actual=await calculate(c.input);
    assert.equal(actual.engine,"webR/0.6.0; R/4.6.0; multcomp/1.4-30; mvtnorm/1.2-4; Dunnett/1");
    assert.equal(actual.comparisons.length,c.expected.length);
    for(const expected of c.expected) {
      const v=actual.comparisons.find(v=>v.leftA===expected.leftA&&v.rightA===expected.rightA);
      assert.ok(v,'explicit treatment/control identity missing');
      near(v.difference,expected.difference,c.name+'.difference');
      near(v.adjustedP,expected.adjustedP,c.name+'.adjustedP',3e-5);
      near(v.lower,expected.lower,c.name+'.lower',1e-4);
      near(v.upper,expected.upper,c.name+'.upper',1e-4);
      assert.equal(v.correction,'Dunnett two-sided single-step (multivariate t)');
    }
    const diagnostic=actual.diagnostics.find(v=>v.code==='dunnett_integration');
    assert.ok(diagnostic.statistic>=0&&diagnostic.statistic<=1e-5);
    assert.match(diagnostic.details,/seed=1701/);
    const quantile=actual.diagnostics.find(v=>v.code==='dunnett_quantile');
    assert.ok(quantile.statistic>=0&&quantile.statistic<=3e-5);
    assert.ok(actual.diagnostics.find(v=>v.code==='dunnett_confidence_integration').statistic<=1e-5);
    // Re-running after unrelated RNG activity must reproduce every number.
    await r.evalRVoid('runif(100)');
    assert.deepEqual(await calculate(c.input),actual);
  });
  for(const [name,patch,error] of [
    ['missing control',{control:''},'control_required'],
    ['unknown control',{control:'invented-not-present'},'control_required'],
    ['Welch is incompatible',{method:'welch'},'incompatible_posthoc'],
    ['confidence level outside library scope',{alpha:.5},'invalid_definition'],
  ]) await test(`Dunnett rejection: ${name}`,async()=>{
    await assert.rejects(calculate({...advanced.cases[0].input,...patch}),new RegExp(`statistics.${error}`));
  });
  const mixed=JSON.parse(fs.readFileSync(new URL("./fixtures/statistics-mixed-golden.json",import.meta.url)));
  for(const c of mixed.cases) await test(`independent Mixed oracle: ${c.name}`,async()=>{
    const actual=await calculate(c.input);
    assert.equal(actual.engine,"webR/0.6.0; R/4.6.0; nlme/3.1-169; mixed-random-intercept/1");
    assert.equal(actual.ssType,"not_applicable_marginal_Wald_F");
    assert.equal(actual.model.family,"random_intercept");
    assert.equal(actual.model.estimation,"ML");
    assert.equal(actual.model.random,"1|unit");
    assert.equal(actual.model.boundaryTolerance,1e-4);
    assert.deepEqual(actual.model.levelsA,c.expected.model.levelsA);
    assert.deepEqual(actual.model.levelsB,c.expected.model.levelsB);
    for(const k of ['randomVariance','residualVariance'])near(actual.model[k],c.expected.model[k],c.name+'.'+k,1e-5);
    near(actual.model.logLikelihood,c.expected.model.logLikelihood,c.name+'.logLikelihood',1e-6/Math.max(1,Math.abs(c.expected.model.logLikelihood)));
    assert.equal(actual.model.fixedCoefficients.length,c.expected.model.fixedCoefficients.length);
    for(const v of c.expected.model.fixedCoefficients){
      const a=actual.model.fixedCoefficients.find(a=>a.name===v.name);assert.ok(a,v.name);
      for(const k of ['estimate','se'])near(a[k],v[k],c.name+'.'+v.name+'.'+k,1e-5);
    }
    assert.equal(actual.terms.length,c.expected.terms.length);
    for(const t of c.expected.terms){
      const a=actual.terms.find(a=>a.source===t.source);assert.ok(a,t.source);
      assert.equal(a.df,t.df);assert.equal(a.denominatorDF,t.denominatorDF);
      near(a.f,t.f,c.name+'.'+t.source+'.F',1e-5);near(a.p,t.p,c.name+'.'+t.source+'.p',1e-6);
      for(const k of ['ss','ms','etaSquared','partialEtaSquared','omegaSquared'])assert.equal(a[k],undefined);
    }
    assert.equal(actual.residuals.length,c.expected.residuals.length);
    actual.residuals.forEach((v,i)=>near(v,c.expected.residuals[i],c.name+'.residual.'+i,1e-5));
    assert.deepEqual(actual.comparisons,[]);assert.equal(actual.corrections,null);
    assert.ok(actual.diagnostics.some(v=>v.code==='mixed_model'));
    assert.ok(!actual.diagnostics.some(v=>v.code==='mauchly'||v.code==='brown_forsythe'));
    assert.ok(!actual.warnings.includes('statistics.repeated_variance_uses_sphericity'));
    assert.deepEqual(await calculate(c.input),actual);
  });
  const base=mixed.cases[2].input;
  const removeCell=(v)=>Object.fromEntries(Object.entries(v).map(([k,x])=>[k,Array.isArray(x)?x.filter((_,i)=>!(v.factorA[i]==='G2'&&v.factorB[i]==='T3')):x]));
  const boundary={...base,values:base.values.map((_,i)=>{const u=i%15,t=Math.floor(i/15);return 2+Math.floor(u/5)*.7+t*.3+(u%5-2)*.1*[-1,1,-1,1][t]})};
  for(const [name,input,error] of [
    ['unknown unit',{...base,unitId:base.unitId.map((v,i)=>i===0?'':v)},'review_structure'],
    ['duplicate unit/time',{...base,unitId:base.unitId.map((v,i)=>i===1?base.unitId[0]:v)},'incompatible_design'],
    ['unit changes group',{...base,factorA:base.factorA.map((v,i)=>i===15?'G1':v)},'unit_changes_group'],
    ['missing whole factorial cell',removeCell(base),'rank_deficient'],
    ['constant outcome',{...base,values:base.values.map(()=>1)},'mixed_not_estimable'],
    ['near singular random variance',boundary,'mixed_not_estimable'],
    ['incompatible posthoc',{...base,postHoc:'tukey'},'incompatible_posthoc'],
    ['independent structure',{...base,structure:'independent'},'incompatible_design'],
    ['sphericity correction',{...base,correction:'GG'},'incompatible_design'],
  ]) await test(`Mixed rejection: ${name}`,async()=>await assert.rejects(calculate(input),new RegExp(`statistics.${error}`)));
} finally {
  r.close();
}
