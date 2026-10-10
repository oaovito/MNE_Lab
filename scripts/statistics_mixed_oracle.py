"""Synthetic independent development references; never called by the product.

Statsmodels 0.14.5 fits Gaussian ML random intercepts. Conditional GLS SEs
use fitted V, not joint likelihood bse_fe. Marginal Wald covariance uses
n/(n-p), matching documented nlme anova.lme adjustSigma=TRUE. Inner/outer
DFs below apply to this restricted categorical, repeated-unit design only.
"""
import json
from pathlib import Path
import numpy as np
import pandas as pd
from scipy import linalg, stats
import scipy, statsmodels
import statsmodels.formula.api as smf

cases=[]
for name,ng,nu,nt,missing,original in [
    ('two_groups_complete',2,6,3,[],True),
    ('two_groups_incomplete',2,6,3,[1,7,26],True),
    ('three_groups_four_times',3,5,4,[],False),
    ('three_groups_incomplete',3,5,4,[1,8,13,22,43,49,51],False),
    ('one_group_incomplete',1,10,4,[2,17,33],False),
]:
    rows=[]
    for t in range(nt):
        for u in range(ng*nu):
            i=t*ng*nu+u
            if i in missing: continue
            g=u//nu
            y=(2+g*1.3+(t+1)*.6+np.sin((u+1)*1.3)+np.cos((u+1)*(t+1)*1.7)*.3) if original else (2+g*1.1+t*.45+np.sin((u+1)*1.27)+np.cos((u+1)*(t+1)*1.71)*.35)
            rows.append(dict(y=float(y),A=f'G{g}',B=f'T{t}',unit=f'u{u:02d}'))
    d=pd.DataFrame(rows);between=ng>1
    formula='y ~ C(A, Sum)*C(B, Sum)' if between else 'y ~ C(B, Sum)'
    fit=smf.mixedlm(formula,d,groups=d.unit).fit(reml=False,method='bfgs',gtol=1e-7,maxiter=10000)
    assert fit.converged
    X=fit.model.exog;n=len(d);p=X.shape[1];units=np.asarray(d.unit)
    rv=float(fit.cov_re.iloc[0,0]);ev=float(fit.scale)
    V=np.eye(n)*ev+(units[:,None]==units[None,:])*rv
    cov=linalg.inv(X.T@linalg.solve(V,X,assume_a='pos'))
    levels={'A':sorted(d.A.unique()),'B':sorted(d.B.unique())}
    coefficients=[]
    for i,k in enumerate(fit.fe_params.index):
        if k=='Intercept': rname='(Intercept)'
        else:
            rname=':'.join(part[2]+str(levels[part[2]].index(part.split('[S.')[1][:-1])+1) for part in k.split(':'))
        coefficients.append(dict(name=rname,estimate=float(fit.fe_params.iloc[i]),se=float(np.sqrt(cov[i,i]))))
    covp=np.zeros((len(fit.params),len(fit.params)));covp[:p,:p]=cov*n/(n-p)
    terms=[]
    for source,key in [('A','C(A, Sum)'),('B','C(B, Sum)'),('A:B','C(A, Sum):C(B, Sum)')]:
        if not between and source!='B':continue
        s=fit.model.data.design_info.term_name_slices[key];cols=list(range(s.start,s.stop))
        K=np.zeros((len(cols),len(fit.params)));K[np.arange(len(cols)),cols]=1
        f=float(fit.wald_test(K,cov_p=covp,use_f=True,scalar=True).statistic)
        denominator=len(d.unit.unique())-ng if source=='A' else n-len(d.unit.unique())-ng*(nt-1)
        terms.append(dict(source=source,df=len(cols),denominatorDF=denominator,f=f,p=float(stats.f.sf(f,len(cols),denominator))))
    cases.append(dict(name=name,input=dict(values=d.y.tolist(),factorA=d.A.tolist(),factorB=d.B.tolist(),unitId=d.unit.tolist(),alpha=.05,method='mixed',postHoc='none',structure='repeated',correction=''),expected=dict(terms=terms,model=dict(randomVariance=rv,residualVariance=ev,logLikelihood=float(fit.llf),fixedCoefficients=coefficients,levelsA=levels['A'],levelsB=levels['B']),residuals=fit.resid.tolist())))
target=Path(__file__).resolve().parents[1]/'web/tests/fixtures/statistics-mixed-golden.json'
target.write_text(json.dumps(dict(oracle=dict(scipy=scipy.__version__,statsmodels=statsmodels.__version__,numpy=np.__version__,parameter_relative_tolerance=1e-5,likelihood_absolute_tolerance=1e-6,probability_absolute_tolerance=1e-6),cases=cases),indent=2,allow_nan=False)+'\n')
print('Wrote five synthetic ML random-intercept oracles, including incomplete and one-group designs.')
