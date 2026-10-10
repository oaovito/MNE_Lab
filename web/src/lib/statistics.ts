import { WebR, ChannelType } from "webr";
import code from "./statistics-engine.R";

export type AnalysisDefinition = {
  schema: number;
  module: "lightscattering";
  title: string;
  cycleId?: string;
  variable: string;
  factorAName: string;
  factorBName?: string;
  structure: "independent" | "repeated" | "";
  structureReviewed: boolean;
  method: string;
  alpha: number;
  postHoc: string;
  control?: string;
  sphericityCorrection?: string;
  observations: {
    measurementId: string;
    factorA: string;
    factorB?: string;
    unitId?: string;
    excludeReason?: string;
  }[];
};
export type AnalysisSnapshot = {
  definition: AnalysisDefinition;
  sourceHash: string;
  unit: string;
  receipt?: string;
  accountId: string;
  profileId: string;
  sources: {
    measurement: {
      id: string;
      parser: string;
      spec: string;
      sourceSheet?: string;
      sourceRange?: string;
    };
    file: { name: string; sha256: string; module: string };
  }[];
  observations: (AnalysisDefinition["observations"][0] & {
    fileId: string;
    sampleId?: string;
    replicate?: number;
    quantity?: { value: number; raw: string; unit?: string };
    missing: boolean;
  })[];
  design: {
    n: number;
    missing: number;
    excluded: number;
    experimentalUnits: number;
    levelsA: string[];
    levelsB: string[];
    balanced: boolean;
    completeRepeated: boolean;
    recommended: string;
    warnings: string[];
  };
};
export type StatisticalResults = {
  engine: string;
  calculation: string;
  method: string;
  ssType: string;
  terms: {
    source: string;
    ss?: number;
    df: number;
    ms?: number;
    f?: number;
    p?: number;
    etaSquared?: number;
    partialEtaSquared?: number;
    omegaSquared?: number;
  }[];
  groups: {
    factorA: string;
    factorB?: string;
    n: number;
    values: number[];
    mean: number;
    sd?: number;
    sem?: number;
    ciLower?: number;
    ciUpper?: number;
  }[];
  comparisons: {
    id: string;
    leftA: string;
    leftB?: string;
    rightA: string;
    rightB?: string;
    contrast: string;
    context?: string;
    difference: number;
    lower?: number;
    upper?: number;
    adjustedP: number;
    correction: string;
  }[];
  diagnostics: {
    code: string;
    statistic?: number;
    p?: number;
    details?: string;
  }[];
  residuals: number[];
  qqTheoretical: number[];
  qqObserved: number[];
  warnings: string[];
  corrections?: Record<string, number>;
};
export type StatisticalAnalysis = {
  schema: number;
  id: string;
  previousId?: string;
  created: string;
  snapshot: AnalysisSnapshot;
  results: StatisticalResults;
  appVersion: string;
  sourceChanged: boolean;
  resultOrigin: string;
};

// Each calculation has its own ephemeral worker. Profile changes, cancellation
// and completion destroy its filesystem and R objects; no browser persistence.
export async function calculateStatistics(
  snapshot: AnalysisSnapshot,
  signal?: AbortSignal,
): Promise<StatisticalResults> {
  const r = new WebR({
    baseUrl: "/statistics-engine/",
    repoUrl: "/statistics-engine/no-packages/",
    interactive: false,
    channelType: ChannelType.PostMessage,
  });
  let closed = false;
  const close = () => {
    if (!closed) {
      closed = true;
      r.close();
    }
  };
  let rejectCanceled!: (e: Error) => void;
  const canceled = new Promise<never>((_, reject) => {
    rejectCanceled = reject;
  });
  const abort = () => {
    rejectCanceled(new Error("statistics.canceled"));
    close();
  };
  const timer = setTimeout(() => {
    rejectCanceled(new Error("statistics.engine_timeout"));
    close();
  }, 180000);
  signal?.addEventListener("abort", abort, { once: true });
  try {
    if (signal?.aborted) throw new Error("statistics.canceled");
    const computation = (async () => {
      await r.init();
      const rows = snapshot.observations.filter(
        (o) => !o.missing && !o.excludeReason && o.quantity,
      );
      const d = snapshot.definition;
      const input = await new r.RList({
        values: rows.map((o) => o.quantity!.value),
        factorA: rows.map((o) => o.factorA),
        factorB: rows.map((o) => o.factorB || ""),
        unitId: rows.map((o) => o.unitId || ""),
        alpha: d.alpha,
        method: d.method,
        postHoc: d.postHoc,
        structure: d.structure,
        correction: d.sphericityCorrection || "GG",
      });
      await r.objs.globalEnv.bind("mne_input", input);
      await r.evalRVoid(code);
      const result = await r.evalRString("mne_json(mne_engine(mne_input))");
      if (signal?.aborted || closed) throw new Error("statistics.canceled");
      return JSON.parse(result) as StatisticalResults;
    })();
    return await Promise.race([computation, canceled]);
  } catch (e: any) {
    const identifier = String(e?.message || e).match(
      /statistics\.[a-z_]+/,
    )?.[0];
    const error = new Error(
      identifier ||
        (signal?.aborted ? "statistics.canceled" : "statistics.engine_failed"),
    );
    Object.assign(error, { code: error.message });
    throw error;
  } finally {
    clearTimeout(timer);
    signal?.removeEventListener("abort", abort);
    close();
  }
}
