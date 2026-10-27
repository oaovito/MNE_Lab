"""Development-only independent references for the paired Condition x Time design.

Both factors vary within each experimental unit (A = condition, B = time).
Never imported by the product. Every value below is invented: units, labels
and responses come from a seeded generator, never from instrument or user
files. Writes web/tests/fixtures/statistics-paired-golden.json.

Classical repeated measures: Pingouin rm_anova (two within factors) supplies
SS, df, F, p, generalized eta squared, Greenhouse-Geisser epsilon and Mauchly;
Statsmodels AnovaRM cross-checks F/df/p and an OLS strata decomposition supplies
the error-stratum sums of squares. Huynh-Feldt uses the formula documented by
Pingouin (Lecoutre correction), before its clipping at one. Corrected p values
come from SciPy's F distribution.

Mixed model: Statsmodels MixedLM, REML, unit random intercept plus a
unit-by-condition variance component (the nested 1|unit/A structure).
Conditional GLS covariance and Wald F use SciPy; denominator df follow the
documented nlme containment rule (Pinheiro and Bates 2000, section 4.2),
written out here independently of the engine.

Simple effects: SciPy ttest_rel and Statsmodels multipletests (Holm or
Bonferroni) per family; Pingouin rm_anova for B within each level of A.
Technical readings in one unit x A x B cell are averaged with pandas first.
"""
import json
from pathlib import Path

import numpy as np
import pandas as pd
import pingouin as pg
import scipy
import statsmodels
import statsmodels.formula.api as smf
from scipy import linalg, stats
from statsmodels.stats.anova import AnovaRM, anova_lm
from statsmodels.stats.multitest import multipletests

ALPHA = 0.05
CONDITIONS = ["F-SR", "C-SR"]
TIMES = ["day 00", "day 07", "day 14", "day 21"]
TIME_EFFECT = {"F-SR": [0.0, 4.0, 7.0, 12.0], "C-SR": [0.0, 2.0, 3.5, 5.0]}
CONDITION_EFFECT = {"F-SR": 0.0, "C-SR": -9.0}


def simulate(n_units, seed, unit_sd=12.0, pair_sd=3.0, error_sd=2.5, remove_pair_effect=False):
    """Invented effective diameters in nm, listed by condition, time and unit."""
    rng = np.random.default_rng(seed)
    units = [f"unit-{chr(65 + i)}" for i in range(n_units)]
    unit_effect = {u: rng.normal(0, unit_sd) for u in units}
    pair_effect = {(u, c): rng.normal(0, pair_sd) for u in units for c in CONDITIONS}
    rows = []
    for c in CONDITIONS:
        for j, t in enumerate(TIMES):
            for u in units:
                y = 185 + unit_effect[u] + pair_effect[(u, c)] + CONDITION_EFFECT[c] + TIME_EFFECT[c][j] + rng.normal(0, error_sd)
                rows.append(dict(y=y, A=c, B=t, unit=u))
    frame = pd.DataFrame(rows)
    if remove_pair_effect:
        # Remove every unit-by-condition deviation: the nested variance
        # component then has no support (a boundary fit).
        uc = frame.groupby(["unit", "A"]).y.transform("mean")
        u = frame.groupby("unit").y.transform("mean")
        c = frame.groupby("A").y.transform("mean")
        frame["y"] = frame.y - (uc - u - c + frame.y.mean())
    frame["y"] = frame.y.round(1)
    return frame


def engine_input(frame, method, post_hoc="none", adjustment="", correction="GG"):
    return dict(values=frame.y.tolist(), factorA=frame.A.tolist(), factorB=frame.B.tolist(), unitId=frame.unit.tolist(),
                alpha=ALPHA, method=method, postHoc=post_hoc, adjustment=adjustment, structure="repeated",
                correction=correction if method == "repeated" else "", design="within_within", control="")


def aggregate(frame):
    """D4: one observation per unit x A x B cell, in order of first appearance."""
    keys = ["unit", "A", "B"]
    grouped = frame.groupby(keys, sort=False)
    out = grouped.y.mean().reset_index()
    out["readings"] = grouped.y.size().to_numpy()
    return out


def groups_of(agg):
    groups = []
    for a in sorted(agg.A.unique()):
        for b in sorted(agg.B.unique()):
            g = agg[(agg.A == a) & (agg.B == b)]
            if not len(g):
                continue
            v = g.y.to_numpy()
            n = len(v)
            sd = float(np.std(v, ddof=1))
            sem = sd / np.sqrt(n)
            margin = stats.t.ppf(1 - ALPHA / 2, n - 1) * sem
            groups.append(dict(factorA=a, factorB=b, n=n, units=g.unit.tolist(), readings=[int(x) for x in g.readings],
                               values=[float(x) for x in v], mean=float(v.mean()), sd=sd, sem=sem,
                               ciLower=float(v.mean() - margin), ciUpper=float(v.mean() + margin)))
    return groups


def huynh_feldt(gg, n_units, dof):
    # Pingouin's documented Huynh-Feldt formula, before clipping at one.
    return (n_units * dof * gg - 2) / (dof * (n_units - 1 - dof * gg))


def corrected_terms(name, f, df, ddf, gg, hf):
    out = []
    for kind, eps in [("GG", gg), ("HF", None if hf is None else min(1.0, hf)), ("LB", 1.0 / df)]:
        if eps is not None:
            out.append(dict(source=f"{name}_{kind}", df=df * eps, denominatorDF=ddf * eps, f=f, p=float(stats.f.sf(f, df * eps, ddf * eps)), epsilon=eps))
    return out


def classical(agg):
    n_units = agg.unit.nunique()
    total = float(np.sum((agg.y - agg.y.mean()) ** 2))
    table = pg.rm_anova(data=agg, dv="y", within=["A", "B"], subject="unit", detailed=True).set_index("Source")
    check = AnovaRM(agg, "y", "unit", within=["A", "B"]).fit().anova_table
    strata = anova_lm(smf.ols("y ~ C(unit) + C(A)*C(B) + C(unit):C(A) + C(unit):C(B)", agg).fit(), typ=1)
    errors = {"A": ("residual_unit:A", "C(unit):C(A)"), "B": ("residual_unit:B", "C(unit):C(B)"), "A:B": ("residual_unit:A:B", "Residual")}
    error_total = float(strata.loc["C(unit)", "sum_sq"] + strata.loc["C(unit):C(A)", "sum_sq"] + strata.loc["C(unit):C(B)", "sum_sq"] + strata.loc["Residual", "sum_sq"])
    terms = [dict(source="residual_subject", ss=float(strata.loc["C(unit)", "sum_sq"]), df=float(strata.loc["C(unit)", "df"]), ms=float(strata.loc["C(unit)", "mean_sq"]))]
    corrected, corrections, diagnostics = [], {}, []
    for name, key in [("A", "A"), ("B", "B"), ("A:B", "A * B")]:
        r = table.loc[key]
        stratum, row = errors[name]
        ess, edf = float(strata.loc[row, "sum_sq"]), float(strata.loc[row, "df"])
        ss, df, ddf, f = float(r.SS), float(r.ddof1), float(r.ddof2), float(r.F)
        assert ddf == edf and np.isclose(ss / df / (ess / edf), f, rtol=1e-10)
        assert np.isclose(check.loc[name, "F Value"], f) and check.loc[name, "Den DF"] == ddf and np.isclose(check.loc[name, "Pr > F"], r.p_unc)
        assert np.isclose(r.ng2, ss / (ss + error_total))
        terms.append(dict(source=name, ss=ss, df=df, denominatorDF=ddf, ms=float(r.MS), f=f, p=float(r.p_unc), etaSquared=ss / total,
                          partialEtaSquared=ss / (ss + ess), generalizedEtaSquared=float(r.ng2)))
        terms.append(dict(source=stratum, ss=ess, df=edf, ms=ess / edf))
        if df > 1:
            within = "B" if name == "B" else ["A", "B"] if name == "A:B" else "A"
            gg = float(pg.epsilon(agg, dv="y", within=within, subject="unit", correction="gg"))
            assert np.isclose(gg, r.eps)
            hf = huynh_feldt(gg, n_units, df)
            hf = hf if np.isfinite(hf) and hf > 0 else None
            lb = float(pg.epsilon(agg, dv="y", within=within, subject="unit", correction="lb"))
            assert np.isclose(lb, 1 / df)
            corrected += corrected_terms(name, f, df, ddf, gg, hf)
            corrections[f"GG:{name}"] = gg
            corrections[f"LB:{name}"] = lb
            if hf is not None:
                corrections[f"HF:{name}"] = hf
            if n_units - 1 >= df:
                diagnostics.append(dict(code="mauchly", details=name, statistic=float(r.W_spher), p=float(r.p_spher)))
            else:
                diagnostics.append(dict(code="mauchly_not_estimable", details=name))
    residual = smf.ols("y ~ C(unit)*C(A) + C(unit)*C(B) + C(A):C(B)", agg).fit().resid.to_numpy()
    return terms + corrected, corrections, diagnostics, residual


def one_way_within(sub, context):
    """Repeated-measures ANOVA of B inside one level of A."""
    n_units = sub.unit.nunique()
    table = pg.rm_anova(data=sub, dv="y", within="B", subject="unit", detailed=True)
    r, e = table.iloc[0], table.iloc[1]
    check = AnovaRM(sub, "y", "unit", within=["B"]).fit().anova_table.iloc[0]
    strata = anova_lm(smf.ols("y ~ C(unit) + C(B)", sub).fit(), typ=1)
    ss, df, f = float(r.SS), float(r.DF), float(r.F)
    ess, edf = float(e.SS), float(e.DF)
    subject = float(strata.loc["C(unit)", "sum_sq"])
    assert np.isclose(check["F Value"], f) and check["Den DF"] == edf and np.isclose(strata.loc["Residual", "sum_sq"], ess)
    terms = [dict(source="residual_subject", ss=subject, df=float(strata.loc["C(unit)", "df"]), ms=float(strata.loc["C(unit)", "mean_sq"])),
             dict(source="B", ss=ss, df=df, denominatorDF=edf, ms=float(r.MS), f=f, p=float(check["Pr > F"]),
                  etaSquared=ss / float(np.sum((sub.y - sub.y.mean()) ** 2)), partialEtaSquared=ss / (ss + ess), generalizedEtaSquared=ss / (ss + subject + ess)),
             dict(source="residual_unit:B", ss=ess, df=edf, ms=ess / edf)]
    corrections, diagnostics = {}, []
    if df > 1:
        gg = float(pg.epsilon(sub, dv="y", within="B", subject="unit", correction="gg"))
        hf = huynh_feldt(gg, n_units, df)
        hf = hf if np.isfinite(hf) and hf > 0 else None
        terms += corrected_terms("B", f, df, edf, gg, hf)
        corrections = {"GG:B": gg, "LB:B": 1 / df}
        if hf is not None:
            corrections["HF:B"] = hf
        if n_units - 1 >= df:
            sph = pg.sphericity(sub, dv="y", within="B", subject="unit")
            diagnostics.append(dict(code="mauchly", details="B; " + context, statistic=float(sph.W), p=float(sph.pval)))
        else:
            diagnostics.append(dict(code="mauchly_not_estimable", details="B; " + context))
    return dict(context=context, factorA=sub.A.iloc[0], terms=terms, corrections=corrections), diagnostics


def simple_effects(agg, adjustment):
    units = list(dict.fromkeys(agg.unit))

    def cell(a, b):
        g = agg[(agg.A == a) & (agg.B == b)].set_index("unit").y
        return g.loc[units].to_numpy()

    def family(pairs, name):
        tests = [stats.ttest_rel(cell(ra, rb), cell(la, lb)) for la, lb, ra, rb, _, _ in pairs]
        adjusted = multipletests([t.pvalue for t in tests], method=adjustment)[1]
        out = []
        for (la, lb, ra, rb, contrast, context), t, p_adj in zip(pairs, tests, adjusted):
            ci = t.confidence_interval(1 - ALPHA)
            out.append(dict(leftA=la, leftB=lb, rightA=ra, rightB=rb, contrast=contrast, context=context, family=name,
                            difference=float(np.mean(cell(ra, rb) - cell(la, lb))), lower=float(ci.low), upper=float(ci.high),
                            statistic=float(t.statistic), df=float(t.df), p=float(t.pvalue), n=len(units), adjustedP=float(p_adj)))
        return out

    la_, lb_ = sorted(agg.A.unique()), sorted(agg.B.unique())
    pairs = [(la_[i], b, la_[j], b, f"{la_[j]}-{la_[i]}", f"B={b}") for b in lb_ for j in range(1, len(la_)) for i in range(j)]
    comparisons = family(pairs, "A_within_B")
    simple, diagnostics = [], []
    for a in la_:
        effect, diag = one_way_within(agg[agg.A == a].reset_index(drop=True), f"A={a}")
        simple.append(effect)
        diagnostics += diag
        pairs = [(a, lb_[i], a, lb_[j], f"{lb_[j]}-{lb_[i]}", f"A={a}") for j in range(1, len(lb_)) for i in range(j)]
        comparisons += family(pairs, "B_within_A")
    return comparisons, simple, diagnostics


def mixed(agg):
    fit = smf.mixedlm("y ~ C(A, Sum)*C(B, Sum)", agg, groups=agg.unit, re_formula="1", vc_formula={"pair": "0 + C(A)"}).fit(
        reml=True, method="bfgs", gtol=1e-8, maxiter=20000)
    assert fit.converged
    X = fit.model.exog
    n, p = X.shape
    unit = agg.unit.to_numpy()
    pair = (agg.unit + "\x00" + agg.A).to_numpy()
    rv, cv, ev = float(fit.cov_re.iloc[0, 0]), float(fit.vcomp[0]), float(fit.scale)
    V = np.eye(n) * ev + (unit[:, None] == unit[None, :]) * rv + (pair[:, None] == pair[None, :]) * cv
    cov = linalg.inv(X.T @ linalg.solve(V, X, assume_a="pos"))
    levels = {"A": sorted(agg.A.unique()), "B": sorted(agg.B.unique())}
    # nlme containment df, written out independently (Pinheiro and Bates 2000, 4.2).
    n_units, n_pairs = agg.unit.nunique(), len(set(pair))
    a, b = len(levels["A"]), len(levels["B"])
    outer = n_units - 1
    middle = n_pairs - n_units - (a - 1)
    inner = n - n_pairs - (b - 1) - (a - 1) * (b - 1)
    coefficients = []
    for i, k in enumerate(fit.fe_params.index):
        if k == "Intercept":
            name, df = "(Intercept)", max(outer, middle, inner)
        else:
            name = ":".join(part[2] + str(levels[part[2]].index(part.split("[S.")[1][:-1]) + 1) for part in k.split(":"))
            df = middle if name.startswith("A") and ":" not in name else inner
        se = float(np.sqrt(cov[i, i]))
        critical = stats.t.ppf(1 - ALPHA / 2, df)
        est = float(fit.fe_params.iloc[i])
        coefficients.append(dict(name=name, estimate=est, se=se, df=df, lower=est - critical * se, upper=est + critical * se))
    covp = np.zeros((len(fit.params), len(fit.params)))
    covp[:p, :p] = cov
    terms = []
    for source, key in [("A", "C(A, Sum)"), ("B", "C(B, Sum)"), ("A:B", "C(A, Sum):C(B, Sum)")]:
        parts = key.split(":")
        cols = [i for i, k in enumerate(fit.fe_params.index) if [x.split("[")[0] for x in k.split(":")] == parts]
        K = np.zeros((len(cols), len(fit.params)))
        K[np.arange(len(cols)), cols] = 1
        f = float(fit.wald_test(K, cov_p=covp, use_f=True, scalar=True).statistic)
        denominator = middle if source == "A" else inner
        terms.append(dict(source=source, df=len(cols), denominatorDF=denominator, f=f, p=float(stats.f.sf(f, len(cols), denominator))))
    model = dict(randomVariance=rv, randomComponents=[dict(level="unit", variance=rv), dict(level="unit:A", variance=cv)], residualVariance=ev,
                 logLikelihood=float(fit.llf), fixedCoefficients=coefficients, levelsA=levels["A"], levelsB=levels["B"])
    return terms, model, fit.resid.to_numpy()


cases, rejections = [], []


def case(name, frame, method, post_hoc="none", adjustment="", correction="GG", ci_only=False):
    agg = aggregate(frame)
    expected = dict(groups=groups_of(agg), experimentalUnits=int(agg.unit.nunique()), measurements=len(frame), cells=len(agg))
    if method == "repeated":
        terms, corrections, diagnostics, residual = classical(agg)
        for k in [k for k in corrections if k.startswith("GG:") or k.startswith("HF:")]:
            pass
        applied = {}
        for k, v in corrections.items():
            kind, term = k.split(":", 1)
            if kind == correction:
                applied[f"applied:{term}"] = min(1.0, v) if kind == "HF" else v
        expected.update(terms=terms, corrections={**corrections, **applied}, diagnostics=diagnostics)
    else:
        terms, model, residual = mixed(agg)
        expected.update(terms=terms, model=model, diagnostics=[])
    expected["residuals"] = [float(x) for x in residual]
    shapiro = stats.shapiro(residual)
    expected["diagnostics"].append(dict(code="shapiro_wilk", statistic=float(shapiro.statistic), p=float(shapiro.pvalue)))
    if (agg.readings > 1).any():
        expected["diagnostics"].append(dict(code="technical_readings_averaged", statistic=float((agg.readings > 1).sum())))
    if post_hoc == "simple_effects":
        comparisons, simple, diagnostics = simple_effects(agg, adjustment)
        expected.update(comparisons=comparisons, simpleEffects=simple)
        expected["diagnostics"] += diagnostics
    else:
        expected.update(comparisons=[], simpleEffects=[])
    cases.append(dict(name=name, ciOnly=ci_only, input=engine_input(frame, method, post_hoc, adjustment, correction), expected=expected))


owner = simulate(3, 20261021)
case("owner_3x2x4_repeated_gg_holm", owner, "repeated", "simple_effects", "holm", "GG")
case("owner_3x2x4_repeated_hf", owner, "repeated", "none", "", "HF")
six = simulate(6, 7340)
case("six_units_repeated_hf_bonferroni", six, "repeated", "simple_effects", "bonferroni", "HF")
# Technical readings: extra readings in three cells of the owner design,
# listed after the other measurements; R must average them per cell.
extra = owner.iloc[[0, 9, 17]].copy()
extra["y"] = (extra.y + np.array([0.6, -0.4, 0.3])).round(1)
extra2 = owner.iloc[[0]].copy()
extra2["y"] = (extra2.y - 0.8).round(1)
readings = pd.concat([owner, extra, extra2], ignore_index=True)
case("technical_readings_repeated_gg_holm", readings, "repeated", "simple_effects", "holm", "GG")
shuffled = six.sample(frac=1, random_state=np.random.RandomState(11)).reset_index(drop=True)
case("six_units_shuffled_rows_repeated_gg", shuffled, "repeated", "none", "", "GG")
# nlme is a bundled package: these run only where the package image exists.
case("owner_3x2x4_mixed_holm", owner, "mixed", "simple_effects", "holm", ci_only=True)
case("six_units_mixed", six, "mixed", ci_only=True)
incomplete = six.drop(index=[3, 13, 30, 41]).reset_index(drop=True)
case("six_units_incomplete_mixed", incomplete, "mixed", ci_only=True)
case("technical_readings_mixed", readings, "mixed", ci_only=True)

boundary = simulate(6, 99, remove_pair_effect=True)
fit = smf.mixedlm("y ~ C(A, Sum)*C(B, Sum)", aggregate(boundary), groups=aggregate(boundary).unit, re_formula="1",
                  vc_formula={"pair": "0 + C(A)"}).fit(reml=True, method="bfgs", gtol=1e-8, maxiter=20000)
rejections.append(dict(name="boundary_unit_condition_variance", ciOnly=True, input=engine_input(boundary, "mixed"), error="mixed_not_estimable",
                       oracle=dict(unitConditionVariance=float(fit.vcomp[0]), residualVariance=float(fit.scale))))
rejections.append(dict(name="incomplete_classical_refused", ciOnly=False, input=engine_input(incomplete, "repeated"), error="incomplete_repeated"))
rejections.append(dict(name="incomplete_simple_effects_refused", ciOnly=False, input=engine_input(incomplete, "mixed", "simple_effects", "holm"), error="incomplete_repeated"))
unknown = engine_input(owner, "repeated")
unknown["unitId"] = [""] + unknown["unitId"][1:]
rejections.append(dict(name="unknown_unit", ciOnly=False, input=unknown, error="review_structure"))
rejections.append(dict(name="tukey_not_offered", ciOnly=False, input=engine_input(owner, "repeated", "tukey"), error="incompatible_posthoc"))
rejections.append(dict(name="simple_effects_need_adjustment", ciOnly=False, input=engine_input(owner, "repeated", "simple_effects", ""), error="invalid_definition"))
independent = engine_input(owner, "repeated")
independent["structure"] = "independent"
rejections.append(dict(name="independent_structure", ciOnly=False, input=independent, error="review_structure"))

target = Path(__file__).resolve().parents[1] / "web/tests/fixtures/statistics-paired-golden.json"
target.write_text(json.dumps(dict(
    oracle=dict(scipy=scipy.__version__, statsmodels=statsmodels.__version__, pingouin=pg.__version__, pandas=pd.__version__, numpy=np.__version__,
                classical_tolerance=1e-8, mixed_parameter_relative_tolerance=1e-5, probability_absolute_tolerance=1e-6, df="exact"),
    note="Synthetic invented values; no instrument or user data.",
    cases=cases, rejections=rejections), indent=1, allow_nan=False) + "\n")
print(f"Wrote {len(cases)} synthetic paired cases and {len(rejections)} rejections to {target.name}")
