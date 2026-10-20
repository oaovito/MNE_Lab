"""Development-only independent golden values; no Python runtime in MNE Lab.

Requires SciPy 1.17.0, statsmodels 0.14.5, pingouin 0.5.5, numpy 2.3.5 and
pandas 2.3.3 (statsmodels 0.14.5 does not run with pandas 3).
All fixtures below are invented; never substitute instrument/user files.
"""
import json
from pathlib import Path
import numpy as np
import pandas as pd
from scipy import stats
import statsmodels.api as sm
from statsmodels.formula.api import ols
from statsmodels.stats.anova import AnovaRM
import pingouin as pg

cases = []

def fixture(name, values, a, method, b=None, units=None, posthoc="none", correction="GG"):
    b = b or [""] * len(values)
    units = units or [""] * len(values)
    inp = dict(values=values, factorA=a, factorB=b, unitId=units, alpha=.05,
               method=method, postHoc=posthoc, structure="repeated" if method == "repeated" else "independent", correction=correction)
    frame = pd.DataFrame(dict(y=values, A=a, B=b, unit=units))
    groups = []
    for (av,bv), g in frame.groupby(["A","B"], sort=True):
        v=g.y.to_numpy(); n=len(v); sd=float(np.std(v,ddof=1)) if n>1 else None
        sem=sd/np.sqrt(n) if sd is not None else None
        margin=stats.t.ppf(.975,n-1)*sem if sem is not None else None
        groups.append(dict(factorA=av,factorB=bv,n=n,mean=float(v.mean()),sd=sd,sem=sem,
                           ciLower=float(v.mean()-margin) if margin is not None else None,
                           ciUpper=float(v.mean()+margin) if margin is not None else None))
    terms=[]; comparisons=[]; corrections={}; diagnostics=[]
    arrays=[g.y.to_numpy() for _,g in frame.groupby("A",sort=True)]
    if method in ("one_way","welch"):
        model=ols("y ~ C(A)",frame).fit()
        tab=sm.stats.anova_lm(model,typ=1)
        ss=float(tab.iloc[0].sum_sq); ess=float(tab.iloc[1].sum_sq); df=float(tab.iloc[0].df); edf=float(tab.iloc[1].df)
        if method=="one_way":
            f,p=stats.f_oneway(*arrays)
            terms=[dict(source="A",ss=ss,df=df,ms=ss/df,f=float(f),p=float(p),etaSquared=ss/(ss+ess),partialEtaSquared=ss/(ss+ess),omegaSquared=max(0,(ss-df*ess/edf)/(ss+ess+ess/edf))),dict(source="residual",ss=ess,df=edf,ms=ess/edf)]
        else:
            w=pg.welch_anova(data=frame,dv="y",between="A").iloc[0]
            f,p=stats.f_oneway(*arrays,equal_var=False)
            terms=[dict(source="A",df=float(w.ddof1),f=float(f),p=float(p)),dict(source="residual",df=float(w.ddof2))]
        residual=model.resid.to_numpy()
        if posthoc=="tukey":
            t=stats.tukey_hsd(*arrays); ci=t.confidence_interval(.95); levels=sorted(frame.A.unique())
            for hi in range(1,len(levels)):
                for lo in range(hi):
                    comparisons.append(dict(contrast=levels[hi]+"-"+levels[lo],difference=float(t.statistic[hi,lo]),lower=float(ci.low[hi,lo]),upper=float(ci.high[hi,lo]),adjustedP=float(t.pvalue[hi,lo])))
    elif method=="two_way":
        model=ols("y ~ C(A, Sum) * C(B, Sum)",frame).fit()
        tab=sm.stats.anova_lm(model,typ=3)
        ess=float(tab.loc["Residual","sum_sq"]); edf=float(tab.loc["Residual","df"]); total=float(np.sum((frame.y-frame.y.mean())**2))
        for key,source in [("C(A, Sum)","A"),("C(B, Sum)","B"),("C(A, Sum):C(B, Sum)","A:B")]:
            r=tab.loc[key]; ss=float(r.sum_sq); df=float(r.df)
            terms.append(dict(source=source,ss=ss,df=df,ms=ss/df,f=float(r.F),p=float(r["PR(>F)"]),etaSquared=ss/total,partialEtaSquared=ss/(ss+ess),omegaSquared=max(0,(ss-df*ess/edf)/(total+ess/edf))))
        terms.append(dict(source="residual",ss=ess,df=edf,ms=ess/edf)); residual=model.resid.to_numpy()
        if posthoc=="tukey":
            families=frame.A.nunique()+frame.B.nunique()
            for condition,within in [("B","A"),("A","B")]:
                for level,sub in frame.groupby(condition,sort=True):
                    arrays=[g.y.to_numpy() for _,g in sub.groupby(within,sort=True)]
                    levels=sorted(sub[within].unique())
                    t=stats.tukey_hsd(*arrays);ci=t.confidence_interval(1-.05/families)
                    for hi in range(1,len(levels)):
                        for lo in range(hi):
                            comparisons.append(dict(contrast=levels[hi]+"-"+levels[lo],context=condition+"="+level,difference=float(t.statistic[hi,lo]),lower=float(ci.low[hi,lo]),upper=float(ci.high[hi,lo]),adjustedP=min(1,float(t.pvalue[hi,lo])*families)))
    elif method=="repeated" and frame.A.nunique()==1:
        # This fixture has one within-subject factor and no between factor.
        tab=pg.rm_anova(data=frame,dv="y",within="B",subject="unit",detailed=True)
        r,e=tab.iloc[0],tab.iloc[1]
        independent=AnovaRM(frame,"y","unit",within=["B"]).fit().anova_table.iloc[0]
        assert np.isclose(r.F,independent["F Value"])
        ss=float(r.SS); ess=float(e.SS); df=float(r.DF); edf=float(e.DF)
        total=float(np.sum((frame.y-frame.y.mean())**2))
        terms=[dict(source="B",ss=ss,df=df,ms=float(r.MS),f=float(r.F),p=float(independent["Pr > F"]),etaSquared=ss/total,partialEtaSquared=ss/(ss+ess)),dict(source="residual_within",ss=ess,df=edf,ms=float(e.MS))]
        wide=frame.pivot(index="unit",columns="B",values="y")
        gg=float(pg.epsilon(wide,correction="gg")); hf=float(pg.epsilon(wide,correction="hf"))
        applied=gg if correction=="GG" else min(1,hf)
        corrections=dict(GG=gg,HF=hf,applied=applied)
        terms.append(dict(source="B_"+correction,ss=ss,df=df*applied,f=float(r.F),p=float(stats.f.sf(r.F,df*applied,edf*applied))))
        sph=pg.sphericity(wide)
        diagnostics.append(dict(code="mauchly",statistic=float(sph.W),p=float(sph.pval)))
        residual=ols("y ~ C(unit) + C(B)",frame).fit().resid.to_numpy()
    elif method=="repeated":
        # Balanced between-subject groups (A) with one within factor (B).
        # pingouin mixed_anova supplies the split-plot sums of squares, df,
        # F and p. Its epsilon uses the unpooled covariance of all units, so
        # sphericity is computed from the pooled within-group covariance
        # instead: units centered on their group means at each level of B.
        # GG and Mauchly's W are scale-invariant, so pingouin's functions
        # apply to the centered data; the df-dependent HF and Mauchly p use
        # the residual df of the between-group model (units - groups), as in
        # Huynh & Feldt with Lecoutre's correction and Mauchly's test.
        tab=pg.mixed_anova(data=frame,dv="y",within="B",between="A",subject="unit",correction=False,effsize="np2").set_index("Source")
        total=float(np.sum((frame.y-frame.y.mean())**2))
        err={}
        for key,source,stratum in [("A","A","residual_subject"),("B","B","residual_within"),("Interaction","A:B","residual_within")]:
            row=tab.loc[key]; ms=float(row.MS); f=float(row.F)
            err[stratum]=(ms/f*float(row.DF2),float(row.DF2))
        for key,source,stratum in [("A","A","residual_subject"),("B","B","residual_within"),("Interaction","A:B","residual_within")]:
            row=tab.loc[key]; ss=float(row.SS); ess,edf=err[stratum]
            terms.append(dict(source=source,ss=ss,df=float(row.DF1),ms=float(row.MS),f=float(row.F),p=float(row["p-unc"]),etaSquared=ss/total,partialEtaSquared=ss/(ss+ess)))
        for stratum,(ess,edf) in err.items():
            terms.append(dict(source=stratum,ss=ess,df=edf,ms=ess/edf))
        # Balanced design: statsmodels' sequential sums of squares match.
        check=sm.stats.anova_lm(ols("y ~ C(A) * C(B)",frame).fit(),typ=1)
        for key,source in [("C(A)","A"),("C(B)","B"),("C(A):C(B)","Interaction")]:
            assert np.isclose(check.loc[key,"sum_sq"],tab.loc[source].SS)
        wide=frame.pivot(index="unit",columns="B",values="y")
        unit_groups=frame.groupby("unit").A.first().loc[wide.index]
        centered=wide-wide.groupby(unit_groups).transform("mean")
        n=len(wide)-unit_groups.nunique(); k=wide.shape[1]-1
        gg=float(pg.epsilon(centered,correction="gg"))
        hf=((n+1)*k*gg-2)/(k*(n-k*gg))
        applied=gg if correction=="GG" else min(1,hf)
        corrections=dict(GG=gg,HF=hf,applied=applied)
        edf_within=err["residual_within"][1]
        for key,source in [("B","B"),("Interaction","A:B")]:
            row=tab.loc[key]
            terms.append(dict(source=source+"_"+correction,ss=float(row.SS),df=float(row.DF1)*applied,f=float(row.F),p=float(stats.f.sf(row.F,float(row.DF1)*applied,edf_within*applied))))
        w=float(pg.sphericity(centered).W)
        rho=1-(2*k**2+k+2)/(6*k*n)
        w2=(k+2)*(k-1)*(k-2)*(2*k**3+6*k**2+3*k+2)/(288*(n*k*rho)**2)
        z=-n*rho*np.log(w); dof=k*(k+1)/2-1
        pr1=stats.chi2.sf(z,dof); pr2=stats.chi2.sf(z,dof+4)
        diagnostics.append(dict(code="mauchly",statistic=w,p=float(pr1+w2*(pr2-pr1))))
        residual=ols("y ~ C(unit) + C(B) + C(A):C(B)",frame).fit().resid.to_numpy()
    shapiro=stats.shapiro(residual)
    diagnostics.append(dict(code="shapiro_wilk",statistic=float(shapiro.statistic),p=float(shapiro.pvalue)))
    if method!="repeated":
        variance_arrays=[g.y.to_numpy() for _,g in frame.groupby(["A","B"] if method=="two_way" else "A",sort=True)]
        bf=stats.levene(*variance_arrays,center="median")
        diagnostics.append(dict(code="brown_forsythe",statistic=float(bf.statistic),p=float(bf.pvalue)))
    cases.append(dict(name=name,input=inp,expected=dict(terms=terms,groups=groups,comparisons=comparisons,corrections=corrections,diagnostics=diagnostics)))

values=[2,3,4,6,7,8,4,5,7,8,10,11]
fixture("balanced_one_way",values,["A"]*6+["B"]*6,"one_way",posthoc="tukey")
fixture("unequal_variance_welch",[1,2,3,2,1,4,4,10,18,24,35],["A"]*6+["B"]*5,"welch")
fixture("unbalanced_decimal_tukey",[1.234,2.345,3.456,4.567,8.765,9.876,10.321,12.345,14.567,15.678,18.765,20.123],["A"]*4+["B"]*3+["C"]*5,"one_way",posthoc="tukey")
fixture("balanced_two_way",values,["A"]*6+["B"]*6,"two_way",["early"]*3+["late"]*3+["early"]*3+["late"]*3,posthoc="tukey")
fixture("unbalanced_interaction",[1,2,4,6,7,9,10,4,5,8,14,18,20,24],["A"]*7+["B"]*7,"two_way",["early"]*3+["late"]*4+["early"]*3+["late"]*4)
fixture("repeated_sphericity",[1,2,4,5,6,8,3,5,6,8,9,12,4,7,8,10,13,16],["A"]*18,"repeated",["t1"]*6+["t2"]*6+["t3"]*6,[f"unit-{i%6}" for i in range(18)])
fixture("repeated_huynh_feldt",[1,2,4,5,6,8,3,5,6,8,9,12,4,7,8,10,13,16],["A"]*18,"repeated",["t1"]*6+["t2"]*6+["t3"]*6,[f"unit-{i%6}" for i in range(18)],correction="HF")
between_units=[f"unit-{i}" for i in range(8)]
between_rows=[(round(2+.9*j+(1.3*j if i>=4 else 0)+np.sin(1.7*(i*3+j+1))*1.1+.4*i,3),"control" if i<4 else "treated",t,u) for i,u in enumerate(between_units) for j,t in enumerate(["t1","t2","t3"])]
for name,correction in [("repeated_between_groups_gg","GG"),("repeated_between_groups_hf","HF")]:
    fixture(name,[r[0] for r in between_rows],[r[1] for r in between_rows],"repeated",[r[2] for r in between_rows],[r[3] for r in between_rows],correction=correction)
target=Path(__file__).resolve().parents[1]/"web/tests/fixtures/statistics-golden.json"
target.parent.mkdir(exist_ok=True)
target.write_text(json.dumps(dict(oracle={"scipy":stats.__version__ if hasattr(stats,"__version__") else "1.17.0","statsmodels":"0.14.5","pingouin":"0.5.5","numpy":np.__version__},cases=cases),indent=2,allow_nan=False)+"\n")
print(f"Wrote {len(cases)} synthetic independent cases to {target.name}")
