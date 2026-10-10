"""Synthetic independent oracle for the documented fixed one-factor eta CI.

SciPy noncentral-F CDF + mature Brent root inversion and MBESS ci.pvaf's
published lambda/(lambda+N) transformation. The raw ncfdtrinc inverse is not
accurate enough for this gate. No real scientific data are used.
"""
import json
from pathlib import Path
import numpy as np
from scipy import stats,optimize
import scipy
cases=[]
for name,target_f,counts,alpha in [
    ('published_reference_design',11.221,[11]*5,.05),
    ('small_sample',13.5,[3]*2,.05),
    ('lower_boundary_zero',1,[10]*3,.05),
    ('upper_not_estimable',.001,[10]*3,.05),
    ('nondefault_alpha_01',11.221,[11]*5,.01),
    ('large_effect',100,[11]*5,.05),
    ('unbalanced',12,[4,7,6,8],.05),
]:
    n=sum(counts);g=len(counts);df1=g-1;df2=n-g
    centers=np.arange(g,dtype=float);centers-=np.average(centers,weights=counts)
    residual=np.concatenate([np.arange(m,dtype=float)-(m-1)/2 for m in counts])
    mse=float(residual@residual/df2);scale=np.sqrt(target_f*mse*df1/np.sum(np.asarray(counts)*centers**2))
    y=100+np.repeat(centers*scale,counts)+residual
    labels=np.repeat([f'G{i}' for i in range(g)],counts)
    samples=[y[labels==f'G{i}'] for i in range(g)];f=float(stats.f_oneway(*samples).statistic)
    assert abs(f-target_f)<1e-9
    def root(prob):
        if stats.f.cdf(f,df1,df2)<prob:return None
        high=1.
        while stats.ncf.cdf(f,df1,df2,high)>prob:high*=2
        return optimize.brentq(lambda nc:stats.ncf.cdf(f,df1,df2,nc)-prob,0,high,xtol=1e-10)
    low=root(1-alpha/2);up=root(alpha/2)
    lower=0. if low is None else low/(low+n);upper=None if up is None else up/(up+n)
    cases.append(dict(name=name,input=dict(values=y.tolist(),factorA=labels.tolist(),factorB=['']*n,unitId=['']*n,alpha=alpha,method='one_way',postHoc='none',structure='independent',effectCI=True,correction='GG'),expected=dict(f=f,lower=lower,upper=upper,lowerAtBoundary=low is None,status='not_estimable' if up is None else 'available',confidenceLevel=1-alpha)))
p=Path(__file__).resolve().parents[1]/'web/tests/fixtures/statistics-effect-ci-golden.json'
p.write_text(json.dumps(dict(oracle=dict(scipy=scipy.__version__,method='noncentral F CDF / Brent root + MBESS documented transformation',endpoint_absolute_tolerance=1e-6),cases=cases),indent=2,allow_nan=False)+'\n')
print('Wrote seven synthetic population eta squared interval oracles, including missing upper bound.')
