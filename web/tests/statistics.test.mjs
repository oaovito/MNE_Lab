import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
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
} finally {
  r.close();
}
