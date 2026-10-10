# Calculations use the pinned bundled R stats library. No package download,
# source evaluation, unit conversion, imputation or outlier removal occurs.
mne_array <- function(x) structure(x, class = c("mne_array", class(x)))
mne_json <- function(x) {
  if (is.null(x)) return("null")
  array <- inherits(x, "mne_array")
  if (array || (is.atomic(x) && length(x) != 1L) || (is.list(x) && is.null(names(x)))) {
    return(paste0("[", paste(vapply(seq_along(x), function(i) mne_json(x[[i]]), ""), collapse=","), "]"))
  }
  if (is.list(x)) return(paste0("{", paste(paste0(encodeString(names(x), quote='"'), ":", vapply(x, mne_json, "")), collapse=","), "}"))
  if (is.logical(x)) return(if (is.na(x)) "null" else if (x) "true" else "false")
  if (is.numeric(x)) return(if (is.finite(x)) format(x, digits=17, scientific=TRUE, trim=TRUE, decimal.mark=".") else "null")
  encodeString(as.character(x), quote='"')
}
mne_diagnostic <- function(code, statistic=NULL, p=NULL, details=NULL) list(code=code, statistic=statistic, p=p, details=details)
# Restricted source-subset MBESS 5.0.1 estimator; upstream bytes/attribution
# are in vendor/mbess. The wrapper bounds work and reports numerical failures.
mne_effect_interval <- function(f,df1,df2,n,alpha) {
  interval<-list(source="A",effect="population_eta_squared",confidenceLevel=1-alpha,method="MBESS 5.0.1 ci.pvaf / conf.limits.ncf",status="not_estimable")
  if(!is.finite(f) || f<=0 || !exists("ci.pvaf",mode="function") || !exists("conf.limits.ncf",mode="function"))return(list(interval=interval))
  helpers<-new.env(parent=globalenv())
  helpers$ci.pvaf<-ci.pvaf;environment(helpers$ci.pvaf)<-helpers
  helpers$conf.limits.ncf<-conf.limits.ncf;environment(helpers$conf.limits.ncf)<-helpers
  evaluations<-0L
  helpers$pf<-function(...) {
    evaluations<<-evaluations+1L
    if(evaluations>10000L)stop("statistics.effect_ci_not_estimable")
    stats::pf(...)
  }
  ciWarnings<-character()
  fit<-withCallingHandlers(tryCatch(helpers$ci.pvaf(F.value=f,df.1=df1,df.2=df2,N=n,conf.level=1-alpha,tol=1e-9),error=function(e)NULL),warning=function(w){ciWarnings<<-c(ciWarnings,conditionMessage(w));invokeRestart("muffleWarning")})
  if(is.null(fit) || length(ciWarnings))return(list(interval=interval))
  lower<-fit$Lower.Limit.Proportion.of.Variance.Accounted.for
  upper<-fit$Upper.Limit.Proportion.of.Variance.Accounted.for
  if(length(lower)!=1L || !is.finite(lower) || lower<0 || lower>=1)return(list(interval=interval))
  interval$lower<-lower;interval$lowerAtBoundary<-lower==0
  if(length(upper)!=1L || !is.finite(upper) || upper<lower || upper>=1)return(list(interval=interval))
  error<-abs(stats::pf(f,df1,df2,ncp=n*upper/(1-upper))-alpha/2)
  if(lower>0)error<-max(error,abs(stats::pf(f,df1,df2,ncp=n*lower/(1-lower))-(1-alpha/2)))
  if(!is.finite(error) || error>2e-9)return(list(interval=interval))
  interval$upper<-upper;interval$status<-"available"
  list(interval=interval,cdfError=error,tailCoverage=fit$Actual.Coverage)
}
mne_mixed <- function(d,alpha) {
  if(any(as.character(d$unit)==""))stop("statistics.review_structure")
  if(nlevels(d$B)<2L || nlevels(d$unit)<3L || any(duplicated(d[c("unit","B")])))stop("statistics.incompatible_design")
  if(any(vapply(split(as.character(d$A),d$unit),function(x)length(unique(x))!=1L,TRUE)))stop("statistics.unit_changes_group")
  if(any(table(d$unit)<2L))stop("statistics.incompatible_design")
  if(!requireNamespace("nlme",quietly=TRUE))stop("statistics.method_unavailable")
  between<-nlevels(d$A)>1L
  unitGroups<-vapply(split(as.character(d$A),d$unit),function(x)x[1],"")
  if((between && any(table(unitGroups)<2L)) || nrow(d)-nlevels(d$unit)-nlevels(d$A)*(nlevels(d$B)-1L)<=0L)stop("statistics.incompatible_design")
  formula<-if(between)y~A*B else y~B
  contrasts<-if(between)list(A=contr.sum,B=contr.sum) else list(B=contr.sum)
  X<-model.matrix(formula,data=d,contrasts.arg=contrasts)
  if(qr(X)$rank!=ncol(X))stop("statistics.rank_deficient")
  fitWarnings<-character()
  fit<-withCallingHandlers(tryCatch(nlme::lme(formula,random=~1|unit,data=d,method="ML",contrasts=contrasts,na.action=na.fail,control=nlme::lmeControl(returnObject=FALSE)),error=function(e)stop("statistics.mixed_not_estimable")),warning=function(w){fitWarnings<<-c(fitWarnings,conditionMessage(w));invokeRestart("muffleWarning")})
  if(length(fitWarnings) || !is.matrix(fit$apVar) || any(!is.finite(fit$apVar)))stop("statistics.mixed_not_estimable")
  tryCatch(nlme::intervals(fit,which="var-cov"),error=function(e)stop("statistics.mixed_not_estimable"))
  randomVariance<-as.numeric(nlme::getVarCov(fit,type="random.effects")[1,1]);residualVariance<-fit$sigma^2
  # Scalar theta = SD(unit)/SD(residual); lme4 1.1-37 isSingular default.
  # This numerical boundary guard is explicit, never a scientific unit rule.
  if(!is.finite(randomVariance) || !is.finite(residualVariance) || randomVariance<=0 || residualVariance<=0 || sqrt(randomVariance/residualVariance)<1e-4)stop("statistics.mixed_not_estimable")
  tab<-nlme::anova.lme(fit,type="marginal",adjustSigma=TRUE)
  tab<-tab[rownames(tab)!="(Intercept)",,drop=FALSE]
  if(any(tab$numDF<=0) || any(tab$denDF<=0) || any(!is.finite(as.matrix(tab))))stop("statistics.mixed_not_estimable")
  coefficientCI<-tryCatch(nlme::intervals(fit,which="fixed",level=1-alpha)$fixed,error=function(e)stop("statistics.mixed_not_estimable"))
  if(any(!is.finite(coefficientCI)) || any(!is.finite(fit$fixDF$X)) || any(fit$fixDF$X<=0))stop("statistics.mixed_not_estimable")
  coefficients<-nlme::fixef(fit);se<-sqrt(diag(vcov(fit)))
  if(any(!is.finite(coefficients)) || any(!is.finite(se)) || any(se<=0))stop("statistics.mixed_not_estimable")
  coefficientResults<-mne_array(lapply(seq_along(coefficients),function(i)list(
    name=names(coefficients)[i],estimate=unname(coefficients[i]),se=unname(se[i]),
    df=unname(fit$fixDF$X[i]),lower=coefficientCI[i,"lower"],upper=coefficientCI[i,"upper"])))
  model<-list(family="random_intercept",fixed=if(between)"A*B" else "B",
    estimation="ML",random="1|unit",residualCovariance="homoscedastic conditional errors",
    test="marginal Wald F; sum contrasts; adjustSigma=TRUE",
    levelsA=mne_array(levels(d$A)),levelsB=mne_array(levels(d$B)),
    randomVariance=randomVariance,residualVariance=residualVariance,
    logLikelihood=as.numeric(logLik(fit)),boundaryTolerance=1e-4,
    coefficientConfidenceLevel=1-alpha,
    coefficientIntervalMethod="nlme::intervals.lme fixed; Student t; conditional GLS; approximate; individual",
    fixedCoefficients=coefficientResults)
  terms<-mne_array(lapply(seq_len(nrow(tab)),function(i)list(source=rownames(tab)[i],
    df=tab[i,"numDF"],denominatorDF=tab[i,"denDF"],f=tab[i,"F-value"],p=tab[i,"p-value"])))
  list(terms=terms,model=model,residuals=as.numeric(residuals(fit,type="response")))

}
mne_engine <- function(input) {
  d <- data.frame(y=as.numeric(input$values), A=factor(input$factorA), B=factor(input$factorB), unit=factor(input$unitId))
  alpha <- as.numeric(input$alpha)
  if (nrow(d)<3L || any(!is.finite(d$y))) stop("statistics.invalid_numeric")
  method <- as.character(input$method)
  warnings <- character()
  terms <- list(); comparisons <- list(); diagnostics <- list(); corrections <- list()
  # Numeric level pairs avoid collisions in user labels such as ("a.b","c")
  # and ("a","b.c"); scientific group identities never depend on separators.
  cell <- factor(paste(as.integer(d$A),as.integer(d$B),sep=":"))
  groups <- lapply(split(d, cell), function(g) {
    n<-nrow(g); s<-if(n>1L) sd(g$y) else NA_real_; sem<-s/sqrt(n)
    critical<-if(n>1L) qt(1-alpha/2,n-1L) else NA_real_
    list(factorA=as.character(g$A[1]),factorB=as.character(g$B[1]),n=n,values=mne_array(g$y),mean=mean(g$y),sd=s,sem=sem,ciLower=mean(g$y)-critical*sem,ciUpper=mean(g$y)+critical*sem)
  })
  term <- function(name,ss,df,ms,f,p,errorSS,errorDF,totalSS,omega=TRUE) {
    list(source=name,ss=ss,df=df,ms=ms,f=f,p=p,etaSquared=if(totalSS>0)ss/totalSS else NULL,
         partialEtaSquared=if(ss+errorSS>0)ss/(ss+errorSS) else NULL,
         omegaSquared=if(omega && totalSS>0)max(0,(ss-df*errorSS/errorDF)/(totalSS+errorSS/errorDF)) else NULL)
  }
  append_table <- function(tab, residuals, totalSS, repeated=FALSE) {
    errorRow<-which(trimws(rownames(tab))=="Residuals")
    if(length(errorRow)!=1L) return(list())
    ess<-as.numeric(tab[errorRow,"Sum Sq"]); edf<-as.numeric(tab[errorRow,"Df"])
    if(edf<=0 || !is.finite(ess) || ess<=0) stop("statistics.zero_residual_variance")
    result<-list()
    for(i in seq_len(nrow(tab))) {
      name<-trimws(rownames(tab)[i]); ss<-as.numeric(tab[i,"Sum Sq"]); df<-as.numeric(tab[i,"Df"])
      if(name=="Residuals") result[[length(result)+1L]]<-list(source="residual",ss=ss,df=df,ms=ss/df)
      else result[[length(result)+1L]]<-term(name,ss,df,ss/df,as.numeric(tab[i,"F value"]),as.numeric(tab[i,"Pr(>F)"]),ess,edf,totalSS,!repeated)
    }
    result
  }
  totalSS<-sum((d$y-mean(d$y))^2)
  ssType<-"I"
  mixedModel<-NULL
  effectIntervals<-NULL
  if(method=="one_way" || method=="welch") {
    if(nlevels(d$A)<2L || any(table(d$A)<2L)) stop("statistics.insufficient_group_size")
    fit<-lm(y~A,data=d); residual<-residuals(fit)
    tab<-anova(fit)
    if(method=="one_way") terms<-append_table(tab,residual,totalSS)
    else {
      if(any(vapply(split(d$y,d$A),var,0)<=0)) stop("statistics.zero_group_variance")
      w<-oneway.test(y~A,data=d,var.equal=FALSE)
      terms<-list(list(source="A",df=unname(w$parameter[1]),f=unname(w$statistic),p=w$p.value),list(source="residual",df=unname(w$parameter[2])))
      ssType<-"not_applicable_welch"; warnings<-c(warnings,"statistics.welch_effect_size_not_classical")
    }
  } else if(method=="two_way") {
    if(nlevels(d$A)<2L || nlevels(d$B)<2L || any(table(d$A,d$B)<1L)) stop("statistics.incomplete_factorial")
    fit<-lm(y~A*B,data=d,contrasts=list(A=contr.sum,B=contr.sum))
    if(fit$rank!=length(coef(fit)) || df.residual(fit)<=0) stop("statistics.rank_deficient")
    residual<-residuals(fit); ess<-sum(residual^2); edf<-df.residual(fit)
    if(ess<=0) stop("statistics.zero_residual_variance")
    # stats::drop1 tests every specified term against the full model, using
    # sum contrasts. Explicit Type III tests retain the interaction term.
    tab<-drop1(fit,scope=~A+B+A:B,test="F")
    for(name in c("A","B","A:B")) {
      ss<-as.numeric(tab[name,"Sum of Sq"]);df<-as.numeric(tab[name,"Df"])
      terms[[length(terms)+1L]]<-term(name,ss,df,ss/df,as.numeric(tab[name,"F value"]),as.numeric(tab[name,"Pr(>F)"]),ess,edf,totalSS)
    }
    terms[[length(terms)+1L]]<-list(source="residual",ss=ess,df=edf,ms=ess/edf)
    ssType<-"III_sum_contrasts"
  } else if(method=="repeated") {
    if(nlevels(d$B)<2L || any(table(d$unit,d$B)!=1L) || any(as.character(d$unit)=="")) stop("statistics.incomplete_repeated")
    if(any(vapply(split(as.character(d$A),d$unit),function(x)length(unique(x))!=1L,TRUE))) stop("statistics.unit_changes_group")
    between<-nlevels(d$A)>1L
    formula<-if(between)y~A*B+Error(unit/B) else y~B+Error(unit/B)
    fit<-aov(formula,data=d); summaries<-summary(fit)
    for(stratum in names(summaries)) {
      stratumTerms<-append_table(summaries[[stratum]][[1]],NULL,totalSS,TRUE)
      for(i in seq_along(stratumTerms)) if(stratumTerms[[i]]$source=="residual")
        stratumTerms[[i]]$source<-if(grepl("unit:B",stratum,fixed=TRUE))"residual_within" else "residual_subject"
      terms<-c(terms,stratumTerms)
    }
    # Diagnostics use residuals of the equivalent subject-blocked model.
    residual<-residuals(lm(if(between)y~unit+B+A:B else y~unit+B,data=d))
    wide<-tapply(d$y,list(d$unit,d$B),identity)
    unitGroups<-vapply(split(as.character(d$A),d$unit),function(x)x[1],"")
    mlm<-if(between)lm(wide~factor(unitGroups)) else lm(wide~1)
    if(nlevels(d$B)>2L) {
      sph<-tryCatch(stats:::sphericity(SSD(mlm),X=~1),error=function(e)NULL)
      mauchly<-tryCatch(mauchly.test(mlm,X=~1),error=function(e)NULL)
      if(is.null(sph) || is.null(mauchly) || !is.finite(sph$GG.eps) || !is.finite(sph$HF.eps) || !is.finite(mauchly$p.value)) stop("statistics.sphericity_not_estimable")
      else {
        diagnostics[[length(diagnostics)+1L]]<-mne_diagnostic("mauchly",unname(mauchly$statistic),mauchly$p.value)
        corrections<-list(GG=sph$GG.eps,HF=sph$HF.eps,applied=if(input$correction=="HF")min(1,sph$HF.eps) else sph$GG.eps)
        # Report full precision library epsilons; corrected df/p are explicit
        # additional terms. Original uncorrected terms remain available.
        for(i in seq_along(terms)) if(terms[[i]]$source %in% c("B","A:B")) {
          original<-terms[[i]];epsilon<-corrections$applied
          residualTerm<-Filter(function(t)t$source=="residual_within",terms)[[1]]
          corrected<-original;corrected$source<-paste0(original$source,"_",input$correction)
          corrected$df<-original$df*epsilon
          corrected$ms<-NULL
          corrected$p<-pf(original$f,corrected$df,residualTerm$df*epsilon,lower.tail=FALSE)
          terms[[length(terms)+1L]]<-corrected
        }
      }
    }
    ssType<-"repeated_subject_strata"
  } else if(method=="mixed") {
    if(input$postHoc!="none")stop("statistics.incompatible_posthoc")
    if(input$structure!="repeated" || input$correction!="")stop("statistics.incompatible_design")
    mixed<-mne_mixed(d,alpha);terms<-mixed$terms;residual<-mixed$residuals;mixedModel<-mixed$model
    ssType<-"not_applicable_marginal_Wald_F"
    diagnostics[[length(diagnostics)+1L]]<-mne_diagnostic("mixed_model",details="ML random intercept per explicit unit; categorical B within unit; A between units; sum contrasts; marginal Wald F with nlme inner/outer denominator df; adjustSigma=TRUE; conditional GLS SE; response residuals; relative SD boundary tolerance=1e-4")
  } else stop("statistics.method_unavailable")
  if(length(residual)!=nrow(d) || any(!is.finite(residual)))stop("statistics.invalid_numeric")
  if(isTRUE(input$effectCI)) {
    if(method!="one_way")stop("statistics.incompatible_design")
    effect<-mne_effect_interval(terms[[1]]$f,terms[[1]]$df,terms[[2]]$df,nrow(d),alpha)
    effectIntervals<-mne_array(list(effect$interval))
    if(effect$interval$status!="available")warnings<-c(warnings,"statistics.effect_ci_not_estimable")
    else {
      diagnostics[[length(diagnostics)+1L]]<-mne_diagnostic("effect_ci_precision",statistic=effect$cdfError,details="MBESS 5.0.1 source functions; fixed one-factor population eta squared; noncentral F; equal tails; tol=1e-9; direct CDF residual target=2e-9; maximum 10000 upstream pf calls; source lower NA becomes zero; no upper limit fabricated")
      diagnostics[[length(diagnostics)+1L]]<-mne_diagnostic("effect_ci_tail_coverage",statistic=effect$tailCoverage,details="MBESS Actual.Coverage reports the noncentral-F endpoint tail-probability sum, not an empirical coverage validation; lower boundary zero can make this conservative relative to nominal confidence level")
    }
  }
  diagnostics[[length(diagnostics)+1L]]<-mne_diagnostic("independence_user_review",details=input$structure)
  if(length(residual)>=3L && length(residual)<=5000L && sd(residual)>0) {
    shapiro<-shapiro.test(residual)
    diagnostics[[length(diagnostics)+1L]]<-mne_diagnostic("shapiro_wilk",unname(shapiro$statistic),shapiro$p.value)
  } else warnings<-c(warnings,"statistics.shapiro_not_applicable")
  # Brown-Forsythe is the median-centred robust variant of Levene's test.
  varianceGroup<-if(method=="two_way")cell else d$A
  deviations<-abs(d$y-ave(d$y,varianceGroup,FUN=median))
  bf<-tryCatch(anova(lm(deviations~varianceGroup)),error=function(e)NULL)
  if(method=="mixed") {
    warnings<-c(warnings,"statistics.mixed_covariance_assumption")
  } else if(method=="repeated") {
    warnings<-c(warnings,"statistics.repeated_variance_uses_sphericity")
  } else if(!is.null(bf) && is.finite(bf[1,"Pr(>F)"])) {
    diagnostics[[length(diagnostics)+1L]]<-mne_diagnostic("brown_forsythe",as.numeric(bf[1,"F value"]),as.numeric(bf[1,"Pr(>F)"]))
    if(bf[1,"Pr(>F)"]<alpha && method=="one_way")warnings<-c(warnings,"statistics.consider_welch")
  } else warnings<-c(warnings,"statistics.variance_test_not_estimable")
  qq<-qqnorm(residual,plot.it=FALSE)
  add_tukey<-function(data,context="",familyCount=1L,fixedA=NULL,fixedB="") {
    if(nlevels(droplevels(data$A))<2L) return(list())
    model<-aov(y~A,data=data)
    if(df.residual(model)<=0 || !is.finite(deviance(model)) || deviance(model)<=0) {
      diagnostics[[length(diagnostics)+1L]]<<-mne_diagnostic("posthoc_family_not_estimable",details=context)
      warnings<<-c(warnings,"statistics.posthoc_family_not_estimable")
      return(list())
    }
    table<-TukeyHSD(model,"A",conf.level=1-alpha/familyCount)$A
    # For exactly two levels the studentized range is sqrt(2)*abs(t).
    # Use R's mature t distribution for this exact identity, avoiding the
    # documented low-df/extreme-quantile ptukey/qtukey approximation error.
    # Families with >2 levels retain TukeyHSD; no independent pairwise tests
    # substitute for the multiple-comparison distribution.
    if(nlevels(droplevels(data$A))==2L) {
      counts<-as.numeric(table(data$A));counts<-counts[counts>0]
      edf<-df.residual(model)
      se<-sqrt(deviance(model)/edf*sum(1/counts))
      critical<-qt(1-alpha/(2*familyCount),edf)
      table[1,"lwr"]<-table[1,"diff"]-critical*se
      table[1,"upr"]<-table[1,"diff"]+critical*se
      table[1,"p adj"]<-2*pt(abs(table[1,"diff"])/se,edf,lower.tail=FALSE)
    }
    # TukeyHSD's lower triangle is column-major: each lower level is paired
    # with every later level. Store identities separately from display labels;
    # hyphens or other punctuation in labels must never be parsed as pairs.
    levels<-levels(droplevels(data$A));pairs<-combn(levels,2L)
    lapply(seq_len(nrow(table)),function(i)list(leftA=if(is.null(fixedA))pairs[1,i] else fixedA,leftB=if(is.null(fixedA))fixedB else pairs[1,i],rightA=if(is.null(fixedA))pairs[2,i] else fixedA,rightB=if(is.null(fixedA))fixedB else pairs[2,i],contrast=rownames(table)[i],context=context,difference=table[i,"diff"],lower=table[i,"lwr"],upper=table[i,"upr"],adjustedP=min(1,table[i,"p adj"]*familyCount),correction=if(familyCount>1L)"Tukey HSD + Bonferroni across simple-effect families" else "Tukey HSD"))
  }
  if(input$postHoc=="tukey") {
    if(method=="one_way") comparisons<-add_tukey(d)
    else if(method=="two_way") {
      # Simple effects keep interaction visible. Multiplicity covers both
      # sets of conditioning families, rather than independent t-tests.
      families<-nlevels(d$A)+nlevels(d$B)
      for(level in levels(d$B))comparisons<-c(comparisons,add_tukey(d[d$B==level,],paste0("B=",level),families,fixedB=level))
      for(level in levels(d$A)) {
        sub<-d[d$A==level,];sub$A<-droplevels(sub$B)
        comparisons<-c(comparisons,add_tukey(sub,paste0("A=",level),families,fixedA=level))
      }
    } else stop("statistics.incompatible_posthoc")
  } else if(input$postHoc=="dunnett") {
    if(method!="one_way") stop("statistics.incompatible_posthoc")
    if(alpha>=0.5) stop("statistics.invalid_definition")
    control<-as.character(input$control)
    if(length(control)!=1L || !control %in% levels(d$A)) stop("statistics.control_required")
    if(nlevels(d$A)>9L) stop("statistics.dunnett_family_limit")
    if(!requireNamespace("multcomp",quietly=TRUE) || !requireNamespace("mvtnorm",quietly=TRUE)) stop("statistics.method_unavailable")
    # The control is explicit. Contrast identities are separate from labels:
    # every treatment-minus-control coefficient is a simultaneous contrast.
    sub<-d;sub$A<-relevel(sub$A,ref=control)
    model<-lm(y~A,data=sub,contrasts=list(A=contr.treatment))
    treatments<-levels(sub$A)[-1L]
    K<-diag(length(coef(model)))[-1L,,drop=FALSE]
    test<-multcomp::glht(model,linfct=K)
    # Genz-Bretz integration is randomized. Fix its seed and bound the work;
    # reject inaccurate integration instead of returning an unqualified p.
    set.seed(1701)
    result<-summary(test,test=multcomp::adjusted("single-step",maxpts=100000,abseps=1e-5,releps=0))$test
    error<-attr(result$pvalues,"error")
    if(length(error)!=1L || !is.finite(error) || error>1e-5) stop("statistics.integration_not_converged")
    set.seed(1701)
    ciWarnings<-character()
    # mvtnorm 1.2-4 does not return the estim.prec attribute expected by
    # multcomp's quantile wrapper. Use its mature qmvt result directly, check
    # completion, then independently evaluate the achieved family coverage.
    correlation<-cov2cor(vcov(test))
    algorithm<-mvtnorm::GenzBretz(maxpts=100000,abseps=1e-5,releps=0)
    qroot<-withCallingHandlers(mvtnorm::qmvt(1-alpha,df=test$df,corr=correlation,tail="both.tails",algorithm=algorithm,ptol=1e-3,maxiter=100,seed=1701),warning=function(w){ciWarnings<<-c(ciWarnings,conditionMessage(w));invokeRestart("muffleWarning")})
    if(length(treatments)>1L && !identical(attr(qroot,"message"),"Normal Completion"))stop("statistics.integration_not_converged")
    critical<-qroot$quantile
    set.seed(1701)
    coverage<-mvtnorm::pmvt(lower=rep(-critical,length(treatments)),upper=rep(critical,length(treatments)),df=test$df,corr=correlation,algorithm=algorithm)
    ciError<-abs(as.numeric(coverage)-(1-alpha));coverageError<-attr(coverage,"error")
    ci<-confint(test,level=1-alpha,calpha=as.numeric(critical))$confint
    if(length(ciWarnings) || any(!is.finite(ci)) || !is.finite(ciError) || ciError>3e-5 || length(coverageError)!=1L || !is.finite(coverageError) || coverageError>1e-5) stop("statistics.integration_not_converged")
    comparisons<-lapply(seq_along(treatments),function(i)list(leftA=control,leftB="",rightA=treatments[i],rightB="",contrast=paste0(treatments[i],"-",control),difference=unname(coef(test)[i]),lower=ci[i,"lwr"],upper=ci[i,"upr"],adjustedP=unname(result$pvalues[i]),correction="Dunnett two-sided single-step (multivariate t)"))
    diagnostics[[length(diagnostics)+1L]]<-mne_diagnostic("dunnett_integration",statistic=error,details="multcomp 1.4-30 / mvtnorm 1.2-4; two-sided single-step; Genz-Bretz; seed=1701; maxpts=100000; absolute integration target=1e-5; explicit control; simultaneous confidence intervals")
    diagnostics[[length(diagnostics)+1L]]<-mne_diagnostic("dunnett_quantile",statistic=ciError,details="absolute achieved family coverage difference from 1-alpha; qmvt ptol=1e-3; maxiter=100; direct pmvt coverage check target=3e-5; a single treatment uses the exact Student-t quantile")
    diagnostics[[length(diagnostics)+1L]]<-mne_diagnostic("dunnett_confidence_integration",statistic=coverageError,details="reported Genz-Bretz error in direct simultaneous-CI coverage check; maxpts=100000; abseps=1e-5; seed=1701")
  }
  for(i in seq_along(comparisons))comparisons[[i]]$id<-paste0("comparison-",i)
  if(method=="two_way" && any(vapply(terms,function(t)t$source=="A:B" && !is.null(t$p) && t$p<alpha,TRUE)))warnings<-c(warnings,"statistics.interaction_requires_simple_effects")
  engine<-"webR/0.6.0; R/4.6.0"
  if(method=="mixed")engine<-paste0(engine,"; nlme/3.1-169; mixed-random-intercept/2")
  if(input$postHoc=="dunnett")engine<-paste0(engine,"; multcomp/1.4-30; mvtnorm/1.2-4; Dunnett/1")
  if(isTRUE(input$effectCI))engine<-paste0(engine,"; MBESS-source/5.0.1; eta2-CI/1")
  list(engine=engine,calculation="mnelab-statistics/1",method=method,ssType=ssType,terms=mne_array(terms),groups=mne_array(groups),comparisons=mne_array(comparisons),diagnostics=mne_array(diagnostics),residuals=mne_array(as.numeric(residual)),qqTheoretical=mne_array(as.numeric(qq$x)),qqObserved=mne_array(as.numeric(qq$y)),warnings=mne_array(warnings),corrections=if(length(corrections))corrections else NULL,model=mixedModel,effectIntervals=effectIntervals)
}
