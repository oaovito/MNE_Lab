// Product-level local worker validation, with external requests blocked.
import fs from "node:fs";
import path from "node:path";
import {
  api,
  launchBrowser,
  launchURL,
  layoutProblems,
  Report,
  translationKeys,
} from "./lib.mjs";
const [dir, outRoot] = process.argv.slice(2);
const out = path.join(outRoot, "statistics");
fs.mkdirSync(out, { recursive: true });
const report = new Report("statistical-analysis");
const browser = await launchBrowser();
let page;
const messages = [];
try {
  page = await browser.newPage({ viewport: { width: 1366, height: 768 } });
  page.on("pageerror", (e) => messages.push(e.message));
  page.on("console", (m) => {
    if (m.type() === "error") messages.push(m.text());
  });
  await page.goto(await launchURL(dir));
  await page.waitForLoadState("networkidle");
  const state = await api(page, "GET", "/api/state");
  await api(page, "PUT", "/api/profile/settings", {
    language: "en",
    theme: "light",
    onboarding: { tour: true, ls: true, turbo: true },
    whatsNewSeen: state.version,
  });
  await page.reload();
  await page.waitForLoadState("networkidle");
  const portableHint=page.getByRole('button',{name:'Got it',exact:true});
  if(await portableHint.count())await portableHint.click();
  const scope = {
    "X-Account-ID": state.account.id,
    "X-Profile-ID": state.profile.id,
  };
  const initialAnalyses = (
    await page.request
      .get(new URL("/api/statistics", page.url()).href, { headers: scope })
      .then((r) => r.json())
  ).length;
  for (let i = 0; i < 6; i++) {
    const bytes = Buffer.from(
      `Sample ID: Oracle observation ${i + 1}\nEffective Diameter (nm): ${i + 1}.000\n`,
    );
    const headers = {
      ...scope,
      "X-MNE-Lab": "1",
      "X-File-Name": encodeURIComponent(`oracle-stat-${i}.txt`),
      "Content-Type": "application/octet-stream",
    };
    const inspected = await page.request.post(
      new URL("/api/import/inspect", page.url()).href,
      { headers, data: bytes },
    );
    if (!inspected.ok()) throw new Error("synthetic inspection failed");
    const review = await inspected.json();
    const confirmed = await page.request.post(
      new URL("/api/import/confirm", page.url()).href,
      {
        headers: { ...headers, "X-Import-Receipt": review.receipt },
        data: bytes,
      },
    );
    if (!confirmed.ok()) throw new Error("synthetic confirmation failed");
  }
  let external = 0;
  await page.route("**/*", (route) => {
    if (new URL(route.request().url()).origin === new URL(page.url()).origin)
      return route.continue();
    external++;
    return route.abort();
  });
  await page.evaluate(
    () => (
      history.pushState(null, "", "/ls/statistics/new"),
      dispatchEvent(new PopStateEvent("popstate"))
    ),
  );
  await page
    .getByRole("textbox", { name: "Analysis name", exact: true })
    .fill("Synthetic one-way study");
  await page
    .getByRole("combobox", { name: "Experimental structure", exact: true })
    .selectOption("independent");
  await page
    .getByRole("checkbox", {
      name: "I reviewed the experimental units and the independence or repetition of these observations.",
      exact: true,
    })
    .check();
  await page
    .getByRole("button", { name: "Choose measurements", exact: true })
    .click();
  for (let i = 1; i <= 6; i++)
    await page
      .getByRole("checkbox", { name: `Oracle observation ${i}`, exact: true })
      .check();
  await page.getByRole("button", { name: "Apply", exact: true }).click();
  for (let i = 1; i <= 6; i++) {
    await page
      .getByRole("textbox", {
        name: `Factor A Oracle observation ${i}`,
        exact: true,
      })
      .fill(i <= 3 ? "A" : "B");
  }
  await page
    .getByRole("combobox", { name: "Multiple comparisons", exact: true })
    .selectOption("tukey");
  await page
    .getByRole("button", { name: "Review design and sources", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Run analysis locally", exact: true })
    .waitFor();
  report.check(
    "review does not save results",
    (
      await page.request
        .get(new URL("/api/statistics", page.url()).href, { headers: scope })
        .then((r) => r.json())
    ).length === initialAnalyses,
  );
  await page
    .getByRole("button", { name: "Run analysis locally", exact: true })
    .click();
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await page
    .getByRole("button", { name: "Run analysis locally", exact: true })
    .waitFor({ timeout: 15000 });
  report.check(
    "cancellation releases the worker and permits a new calculation",
    (await page
      .getByRole("button", { name: "Save analysis", exact: true })
      .count()) === 0,
  );
  await page
    .getByRole("button", { name: "Run analysis locally", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Save analysis", exact: true })
    .waitFor({ timeout: 120000 });
  report.check(
    "local WASM calculation returns expected F and p",
    (await page.locator("body").innerText()).includes("13.500000") ||
      (await page.locator("body").innerText()).includes("13.5"),
  );
  await page
    .getByRole("heading", { name: "ANOVA table", exact: true })
    .scrollIntoViewIfNeeded();
  await page.screenshot({ path: path.join(out, "calculated.png") });
  await page
    .getByRole("button", { name: "Save analysis", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Create statistical graph", exact: true })
    .waitFor();
  const saved = (
    await page.request
      .get(new URL("/api/statistics", page.url()).href, { headers: scope })
      .then((r) => r.json())
  )[0];
  report.check(
    "saved result preserves quantities and full-precision calculation",
    saved.snapshot.design.n === 6 &&
      Math.abs(saved.results.terms[0].f - 13.5) < 1e-10 &&
      Math.abs(saved.results.terms[0].p - 0.021311641128756727) < 1e-10 &&
      saved.snapshot.observations[0].quantity.raw === "1.000" &&
      saved.snapshot.definition.module === "lightscattering",
    JSON.stringify({
      n: saved.snapshot.design.n,
      f: saved.results.terms[0].f,
      p: saved.results.terms[0].p,
      raw: saved.snapshot.observations[0].quantity.raw,
      module: saved.snapshot.definition.module,
    }),
  );
  report.check(
    "calculation does not request an external service",
    external === 0,
  );
  report.check(
    "diagnostics include real residual plots",
    (await page
      .getByRole("img", { name: "Residual QQ plot", exact: true })
      .count()) === 1 &&
      (await page
        .getByRole("img", { name: "Residual histogram", exact: true })
        .count()) === 1,
  );
  const problems = await layoutProblems(page, translationKeys());
  report.check(
    "analysis result layout has no overflow or untranslated labels",
    problems.length === 0,
    problems.join("; "),
  );
  await page
    .getByRole("button", { name: "Create statistical graph", exact: true })
    .click();
  await page.getByText("Mean error bars", { exact: true }).waitFor();
  report.check(
    "statistical graph is available in shared Graph Engine",
    page.url().includes("/ls/graphs/") &&
      (await page
        .getByRole("button", { name: "Statistical analyses", exact: true })
        .count()) > 0,
  );
  // Select only the checkbox explicitly identifying this saved comparison.
  const choice = page
    .locator("label")
    .filter({ hasText: "B-A · p(adj)=" })
    .getByRole("checkbox");
  await choice.check();
  await page
    .getByRole("combobox", { name: "Mean error bars", exact: true })
    .selectOption("sem");
  const [savedGraphResponse]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/api/graphs'&&r.request().method()==='POST'&&r.status()===200),page.getByRole("button", { name: "Save", exact: true }).click()]);
  const graphID = (await savedGraphResponse.json()).id;
  const graph = await page.request
    .get(new URL("/api/graphs/" + graphID, page.url()).href, { headers: scope })
    .then((r) => r.json());
  const rendered = await page.request.post(
    new URL("/api/graphs/render", page.url()).href,
    {
      headers: { ...scope, "X-MNE-Lab": "1" },
      data: { definition: graph, width: 900, height: 600 },
    },
  );
  const figure = await rendered.json();
  report.check(
    "adjusted comparison and explicit error-bar choice persist",
    graph.annotations?.[0] === saved.results.comparisons[0].id &&
      graph.errorBars === "sem" &&
      JSON.stringify(figure).includes("statistical-annotation"),
    JSON.stringify({
      annotations: graph.annotations,
      errorBars: graph.errorBars,
      status: rendered.status(),
    }),
  );
  await page
    .getByRole("combobox", { name: "Comparison labels", exact: true })
    .selectOption("stars");
  await page
    .getByText(
      "Adjusted p: * <0.05; ** <0.01; *** <0.001; **** <0.0001; ns ≥0.05. Display thresholds do not change analysis alpha.",
      { exact: true },
    )
    .waitFor();
  report.check("stars state their multiplicity-aware thresholds", true);
  await page.locator('.figure-wrap svg text').filter({hasText:/^\*$/}).waitFor();
  report.check('screen displays the selected adjusted significance bracket',true);
  await page.screenshot({ path: path.join(out, "annotated-graph.png") });
  await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/api/graphs'&&r.request().method()==='POST'&&r.status()===200),page.getByRole('button',{name:'Save',exact:true}).click()]);
  for(const [language,theme,width,height]of [['en','light',1366,768],['en','dark',1024,600],['pt-BR','light',1366,768],['pt-BR','dark',1024,600],['es','light',1366,768],['es','dark',1024,600]]){
   await api(page,'PUT','/api/profile/settings',{language,theme,onboarding:{tour:true,ls:true,turbo:true},whatsNewSeen:state.version});
   await page.setViewportSize({width,height});
   await page.goto(new URL('/ls/statistics/'+saved.id,page.url()).href);
   await page.waitForLoadState('networkidle');
   await page.locator('.statistics-page table').first().waitFor();
   const problems=await layoutProblems(page,translationKeys());
   report.check(`saved analysis layout ${language}/${theme}/${width}x${height}`,problems.length===0,problems.join('; '));
   await page.screenshot({path:path.join(out,`saved-${language}-${theme}.png`)});
  }
  report.check(
    "browser has no script or CSP failures",
    messages.length === 0,
    messages.slice(0, 4).join("; "),
  );
} catch (e) {
  report.check(
    "statistical workflow completes",
    false,
    e.message.split("\n")[0],
  );
  fs.writeFileSync(
    path.join(out, "failure.txt"),
    (e.stack || String(e)) + "\n" + messages.join("\n"),
  );
  if (page)
    await page
      .screenshot({ path: path.join(out, "failure.png") })
      .catch(() => {});
} finally {
  await browser.close();
}
process.exit(report.finish(out));
