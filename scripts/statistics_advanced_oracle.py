"""Synthetic development oracles only; not shipped or called by MNE Lab.

SciPy 1.17.0 DunnettResult supplies the studentized multivariate-t model.
Its cdf and Brent root solver independently refine the simultaneous quantile.
The pinned _rho/_df/_std attributes are development references, not product API.
References: scipy.stats.dunnett, multivariate_t; multcomp glht/confint manuals.
"""
import json
from pathlib import Path
import numpy as np
import scipy
from scipy import stats, optimize

cases=[]
for name,counts,alpha in [('balanced_control_not_first',[5,5,5],.05),('unbalanced_three_treatments',[7,4,6,5],.05),('single_treatment',[4,6],.05),('nondefault_alpha_01',[5,5,5],.01)]:
    labels=['control-with-hyphen','A.B','treatment-C','D'][:len(counts)]
    a=np.repeat(labels,counts); index=np.arange(1,len(a)+1)
    y=index*.07+np.sin(index*1.7)+np.repeat(np.arange(1,len(counts)+1),counts)*.43
    control=labels[0]; treatments=sorted(labels[1:])
    samples=[y[a==label] for label in treatments]
    ref=stats.dunnett(*samples,control=y[a==control],rng=1701)
    def tail(t):
        t=np.broadcast_to(t,len(treatments))
        return 1-stats.multivariate_t.cdf(t,shape=ref._rho,df=ref._df,lower_limit=-t,maxpts=1000000,random_state=np.random.default_rng(1701))
    critical=optimize.brentq(lambda t:tail(t)-alpha,1,10,xtol=1e-10)
    means=np.array([v.mean()-y[a==control].mean() for v in samples])
    se=ref._std*np.sqrt(1/ref._n_samples+1/ref._n_control)
    comparisons=[]
    for i,label in enumerate(treatments):
        p=tail(abs(ref.statistic[i]))
        assert abs(p-ref.pvalue[i])<5e-4
        comparisons.append(dict(leftA=control,rightA=label,difference=float(means[i]),lower=float(means[i]-critical*se[i]),upper=float(means[i]+critical*se[i]),adjustedP=float(p)))
    cases.append(dict(name=name,input=dict(values=y.tolist(),factorA=a.tolist(),factorB=['']*len(a),unitId=['']*len(a),alpha=alpha,method='one_way',postHoc='dunnett',structure='independent',control=control,correction='GG'),expected=comparisons))
target=Path(__file__).resolve().parents[1]/'web/tests/fixtures/statistics-advanced-golden.json'
target.write_text(json.dumps(dict(oracle={'scipy':scipy.__version__,'integration_maxpts':1000000,'seed':1701,'p_tolerance':3e-5,'ci_tolerance':1e-4},cases=cases),indent=2,allow_nan=False)+'\n')
print('Wrote 4 synthetic Dunnett joint-distribution oracles with independent simultaneous CIs, including alpha=.01.')
