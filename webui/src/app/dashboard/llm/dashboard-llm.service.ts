import { Injectable, OnDestroy, inject } from "@angular/core";
import { Apollo } from "apollo-angular";
import { map, shareReplay } from "rxjs";
import * as generated from "../../graphql/generated";

export const LLM_EVENT_LIMIT = 500;
export const LLM_WINDOW_MINUTES = 15;
export const REDACTED_VALUE = "***REDACTED***";

export type FeedFilter = "ALL" | "MATCHED" | "UNMATCHED" | "ERROR";
export type LlmEvent = generated.DashboardDataQuery["llm"]["events"][number];
type LlmStats = generated.DashboardDataQuery["llm"]["stats"];
type ConfigSection = generated.DashboardDataQuery["config"]["sections"][number];

export interface ContentTypeCount {
  contentType: string;
  count: number;
}

export interface ProviderView {
  provider: string;
  label: string;
  matched: number;
  rate: number;
  latencyP50Ms: number;
  latencyP50: string;
}

export interface ClassifierConfigView {
  raw: Record<string, unknown>;
  llmRaw: Record<string, unknown>;
  runtimeChangeable?: generated.ConfigRuntimeChangeability;
  enabled: boolean;
  providerName: string;
  baseUrl: string;
  model: string;
  apiKey: string;
  batchSize: number;
  maxContext: number;
  maxTokens: number;
  intervalSeconds: number;
  timeoutSeconds: number;
}

export interface LlmDashboardView {
  lastPolledAt: number;
  summary: generated.DashboardDataQuery["dashboard"]["summary"];
  events: LlmEvent[];
  stats: LlmStats;
  config: ClassifierConfigView;
  distribution: ContentTypeCount[];
  providers: ProviderView[];
  counts: Record<FeedFilter, number>;
  latencyP50: string;
  latencyP95: string;
  windowSuccessRate: number;
  windowTruncated: boolean;
  windowCoverageStart: string;
  slots: boolean[];
  utilization: number;
  capacityStatus: string;
  drainRatePerHour: number;
  etaHours?: number;
}

export interface LlmConfigFormValue {
  enabled: boolean;
  providerName: string;
  baseUrl: string;
  model: string;
  apiKey: string;
  batchSize: number;
  maxContext: number;
  maxTokens: number;
  intervalSeconds: number;
  timeoutSeconds: number;
}

@Injectable()
export class DashboardLlmService implements OnDestroy {
  private query = inject(Apollo).watchQuery<
    generated.DashboardDataQuery,
    generated.DashboardDataQueryVariables
  >({
    query: generated.DashboardDataDocument,
    variables: {
      eventLimit: LLM_EVENT_LIMIT,
      windowMinutes: LLM_WINDOW_MINUTES,
    },
    fetchPolicy: "network-only",
    pollInterval: 3000,
  });

  readonly data$ = this.query.valueChanges.pipe(
    map(({ data }) => mapDashboardLlmData(data, Date.now())),
    shareReplay({ bufferSize: 1, refCount: true }),
  );

  refetch() {
    return this.query.refetch();
  }

  ngOnDestroy() {
    this.query.stopPolling();
  }
}

export function mapDashboardLlmData(
  data: generated.DashboardDataQuery,
  lastPolledAt = Date.now(),
): LlmDashboardView {
  const events = [...data.llm.events]
    .slice(0, LLM_EVENT_LIMIT)
    .sort((a, b) => Date.parse(b.timestamp) - Date.parse(a.timestamp));
  const stats = data.llm.stats;
  const configSection = data.config.sections.find(
    (section) => section.key === "classifier",
  );
  const config = mapClassifierConfig(configSection);
  const distribution = contentTypeDistribution(events);
  const windowStart = Date.parse(stats.windowStart);
  const windowEvents = events.filter(
    (event) => Date.parse(event.timestamp) >= windowStart,
  );
  const windowMatched = windowEvents.filter(
    (event) => event.outcome === "MATCHED",
  ).length;
  const concurrency = Math.max(0, stats.concurrency);
  const utilization = concurrency > 0 ? stats.inFlight / concurrency : 0;
  const drainRatePerHour = stats.throughputPerMinute * 60;

  return {
    lastPolledAt,
    summary: data.dashboard.summary,
    events,
    stats,
    config,
    distribution,
    providers: providerViews(stats, events, config),
    counts: countEvents(events),
    latencyP50: formatDuration(stats.latencyP50Ms),
    latencyP95: formatDuration(stats.latencyP95Ms),
    windowSuccessRate:
      windowEvents.length > 0 ? windowMatched / windowEvents.length : 0,
    windowTruncated: Boolean(stats.oldestBuffered),
    windowCoverageStart: stats.oldestBuffered ?? stats.windowStart,
    slots: Array.from(
      { length: concurrency },
      (_, index) => index < stats.inFlight,
    ),
    utilization,
    capacityStatus:
      utilization >= 1
        ? "saturated, backlog growing"
        : utilization >= 0.8
          ? "near capacity"
          : "keeping up",
    drainRatePerHour,
    etaHours:
      drainRatePerHour > 0 ? stats.queuePending / drainRatePerHour : undefined,
  };
}

export function mapClassifierConfig(
  section?: Pick<ConfigSection, "value" | "runtimeChangeable">,
): ClassifierConfigView {
  const raw = asRecord(section?.value);
  const llmRaw = asRecord(raw["Llm"]);

  return {
    raw,
    llmRaw,
    runtimeChangeable: section?.runtimeChangeable,
    enabled: booleanValue(llmRaw["Enabled"], false),
    providerName: stringValue(llmRaw["ProviderName"], "default"),
    baseUrl: stringValue(llmRaw["ProviderBaseURL"]),
    model: stringValue(llmRaw["ProviderModel"]),
    apiKey: stringValue(llmRaw["ProviderAPIKey"], REDACTED_VALUE),
    batchSize: numberValue(llmRaw["BatchSize"], 5),
    maxContext: numberValue(llmRaw["MaxContext"], 16000),
    maxTokens: numberValue(llmRaw["MaxTokens"], 256),
    intervalSeconds: durationSeconds(llmRaw["Interval"], 5),
    timeoutSeconds: durationSeconds(llmRaw["Timeout"], 30),
  };
}

export function buildClassifierConfigValue(
  config: ClassifierConfigView,
  value: LlmConfigFormValue,
): Record<string, unknown> {
  return {
    ...config.raw,
    Llm: {
      ...config.llmRaw,
      Enabled: value.enabled,
      ProviderName: value.providerName.trim(),
      ProviderBaseURL: value.baseUrl.trim(),
      ProviderModel: value.model.trim(),
      ProviderAPIKey: value.apiKey,
      BatchSize: value.batchSize,
      MaxContext: value.maxContext,
      MaxTokens: value.maxTokens,
      Interval: `${value.intervalSeconds}s`,
      Timeout: `${value.timeoutSeconds}s`,
    },
  };
}

export function filterLlmEvents(
  events: LlmEvent[],
  filter: FeedFilter,
): LlmEvent[] {
  return filter === "ALL"
    ? events
    : events.filter((event) => event.outcome === filter);
}

export function formatDuration(milliseconds: number): string {
  if (milliseconds < 1000) {
    return `${milliseconds}ms`;
  }

  return `${(milliseconds / 1000).toFixed(milliseconds < 10000 ? 2 : 1)}s`;
}

function contentTypeDistribution(events: LlmEvent[]): ContentTypeCount[] {
  const counts = new Map<string, number>();

  for (const event of events.slice(0, LLM_EVENT_LIMIT)) {
    if (event.contentType) {
      counts.set(event.contentType, (counts.get(event.contentType) ?? 0) + 1);
    }
  }

  return [...counts]
    .map(([contentType, count]) => ({ contentType, count }))
    .sort(
      (a, b) => b.count - a.count || a.contentType.localeCompare(b.contentType),
    )
    .slice(0, 6);
}

function providerViews(
  stats: LlmStats,
  events: LlmEvent[],
  config: ClassifierConfigView,
): ProviderView[] {
  return stats.perProvider.map((provider) => {
    const durations = events
      .filter(
        (event) =>
          event.provider === provider.provider && event.durationMs >= 0,
      )
      .map((event) => event.durationMs)
      .sort((a, b) => a - b);
    const latencyP50Ms = percentile(durations, 50);

    return {
      provider: provider.provider,
      label:
        provider.provider === config.providerName && config.model
          ? `${provider.provider} · ${config.model}`
          : provider.provider,
      matched: provider.matched,
      rate: provider.attempted > 0 ? provider.matched / provider.attempted : 0,
      latencyP50Ms,
      latencyP50: formatDuration(latencyP50Ms),
    };
  });
}

function countEvents(events: LlmEvent[]): Record<FeedFilter, number> {
  return {
    ALL: events.length,
    MATCHED: events.filter((event) => event.outcome === "MATCHED").length,
    UNMATCHED: events.filter((event) => event.outcome === "UNMATCHED").length,
    ERROR: events.filter((event) => event.outcome === "ERROR").length,
  };
}

function percentile(sorted: number[], percentage: number): number {
  if (sorted.length === 0) {
    return 0;
  }

  return sorted[Math.ceil((sorted.length * percentage) / 100) - 1];
}

function asRecord(value: unknown): Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

function stringValue(value: unknown, fallback = ""): string {
  return typeof value === "string" ? value : fallback;
}

function numberValue(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function booleanValue(value: unknown, fallback: boolean): boolean {
  return typeof value === "boolean" ? value : fallback;
}

function durationSeconds(value: unknown, fallback: number): number {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value / 1_000_000_000;
  }

  if (typeof value === "string") {
    const match = value.match(/^([\d.]+)(ms|s|m)$/);
    if (match) {
      const amount = Number(match[1]);
      return match[2] === "ms"
        ? amount / 1000
        : match[2] === "m"
          ? amount * 60
          : amount;
    }
  }

  return fallback;
}
