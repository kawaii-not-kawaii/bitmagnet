import * as generated from "../../graphql/generated";
import {
  LLM_EVENT_LIMIT,
  buildClassifierConfigValue,
  filterLlmEvents,
  mapDashboardLlmData,
} from "./dashboard-llm.service";

describe("DashboardLlmService mapping", () => {
  it("maps the observability contract into one display model", () => {
    const data = dashboardData();
    const view = mapDashboardLlmData(data, 123456);

    expect(view.lastPolledAt).toBe(123456);
    expect(view.config.concurrency).toBe(10);
    expect(view.events.map((event) => event.torrentName)).toEqual([
      "Matched movie",
      "Unknown release",
      "Broken release",
    ]);
    expect(view.counts).toEqual({
      ALL: 3,
      MATCHED: 1,
      UNMATCHED: 1,
      ERROR: 1,
    });
    expect(view.windowSuccessRate).toBeCloseTo(1 / 3);
    expect(view.latencyP50).toBe("1.50s");
    expect(view.latencyP95).toBe("2.50s");
    expect(view.providers[0]).toEqual(
      jasmine.objectContaining({
        label: "openai · gpt-4o-mini",
        rate: 0.5,
        latencyP50: "500ms",
      }),
    );
    expect(view.distribution).toEqual([{ contentType: "movie", count: 1 }]);
    expect(view.slots).toEqual([true, true, true, true]);
    expect(view.capacityStatus).toBe("saturated, backlog growing");
    expect(view.drainRatePerHour).toBe(120);
    expect(view.etaHours).toBe(2);
    expect(
      filterLlmEvents(view.events, "ERROR").map((event) => event.error),
    ).toEqual(["malformed response"]);
  });

  it("preserves unrelated config fields while serializing edited LLM values", () => {
    const view = mapDashboardLlmData(dashboardData(), 123456);
    const value = buildClassifierConfigValue(view.config, {
      enabled: false,
      concurrency: 6,
      providerName: " local ",
      baseUrl: " http://localhost:8080 ",
      model: " gemma-4 ",
      apiKey: "replacement-key",
      batchSize: 8,
      maxContext: 32000,
      maxTokens: 512,
      intervalSeconds: 6,
      timeoutSeconds: 45,
    });

    expect(value["Concurrency"]).toBe(6);
    expect(value["Llm"]).toEqual(
      jasmine.objectContaining({
        Enabled: false,
        ProviderName: "local",
        ProviderBaseURL: "http://localhost:8080",
        ProviderModel: "gemma-4",
        ProviderAPIKey: "replacement-key",
        BatchSize: 8,
        MaxContext: 32000,
        MaxTokens: 512,
        Interval: "6s",
        Timeout: "45s",
      }),
    );
  });
});

function dashboardData(): generated.DashboardDataQuery {
  const event = (
    timestamp: string,
    outcome: generated.LlmClassificationOutcome,
    torrentName: string,
    durationMs: number,
    contentType: string,
    error: string,
  ): generated.DashboardDataQuery["llm"]["events"][number] => ({
    timestamp,
    infoHash: torrentName.padEnd(40, "0").slice(0, 40),
    torrentName,
    provider: outcome === "ERROR" ? "ollama" : "openai",
    durationMs,
    outcome,
    promptTokens: 20,
    completionTokens: 8,
    contentType,
    title: outcome === "MATCHED" ? "Matched" : "",
    year: outcome === "MATCHED" ? 2024 : 0,
    season: 0,
    episode: 0,
    languages: outcome === "MATCHED" ? ["en"] : [],
    error,
  });

  return {
    dashboard: {
      summary: {
        totalTorrents: 1000,
        torrentsToday: 10,
        indexedLastHour: 5,
        indexedPreviousHour: 4,
        classifiedPercent: 87.4,
        queueProcessed: 900,
        queuePending: 240,
        queueFailed: 2,
      },
    },
    llm: {
      events: [
        event(
          "2026-07-22T12:13:00Z",
          "UNMATCHED",
          "Unknown release",
          500,
          "",
          "",
        ),
        event(
          "2026-07-22T12:12:00Z",
          "ERROR",
          "Broken release",
          2500,
          "",
          "malformed response",
        ),
        event(
          "2026-07-22T12:14:00Z",
          "MATCHED",
          "Matched movie",
          1500,
          "movie",
          "",
        ),
      ].slice(0, LLM_EVENT_LIMIT),
      stats: {
        attempted: 30,
        matched: 20,
        unmatched: 7,
        errored: 3,
        skipped: 1,
        promptTokens: 600,
        completionTokens: 240,
        successRate: 2 / 3,
        perProvider: [
          {
            provider: "openai",
            attempted: 2,
            matched: 1,
            unmatched: 1,
            errored: 0,
          },
          {
            provider: "ollama",
            attempted: 1,
            matched: 0,
            unmatched: 0,
            errored: 1,
          },
        ],
        errorCategories: [
          { category: "rate-limited", count: 2 },
          { category: "invalid-json", count: 1 },
        ],
        inFlight: 4,
        concurrency: 4,
        windowStart: "2026-07-22T12:00:00Z",
        oldestBuffered: null,
        windowAttempted: 3,
        latencyP50Ms: 1500,
        latencyP95Ms: 2500,
        throughputPerMinute: 2,
        queuePending: 240,
      },
    },
    config: {
      sections: [
        {
          key: "classifier",
          runtimeChangeable: "LIVE_APPLY_AVAILABLE",
          value: {
            Concurrency: 10,
            Llm: {
              Enabled: true,
              ProviderName: "openai",
              ProviderBaseURL: "https://api.openai.com/v1",
              ProviderModel: "gpt-4o-mini",
              ProviderAPIKey: "***REDACTED***",
              BatchSize: 5,
              MaxContext: 16000,
              MaxTokens: 256,
              Interval: 5_000_000_000,
              Timeout: 30_000_000_000,
            },
          },
        },
      ],
    },
  };
}
