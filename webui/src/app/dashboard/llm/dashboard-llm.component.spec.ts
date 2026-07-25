import { signal } from "@angular/core";
import { ComponentFixture, TestBed } from "@angular/core/testing";
import { Apollo } from "apollo-angular";
import { of } from "rxjs";
import { ErrorsService } from "../../errors/errors.service";
import type * as generated from "../../graphql/generated";
import { UiPreferences } from "../../layout/ui-preferences.service";
import { DashboardLlmComponent } from "./dashboard-llm.component";
import { DashboardLlmService } from "./dashboard-llm.service";
import type { LlmDashboardView, LlmEvent } from "./dashboard-llm.service";

type ConnectionResult =
  generated.DashboardLlmTestConnectionMutation["dashboard"]["testLlmConnection"];

describe("DashboardLlmComponent", () => {
  let connection: ConnectionResult;
  let element: HTMLElement;
  let fixture: ComponentFixture<DashboardLlmComponent>;
  const density = signal<"comfortable" | "compact">("comfortable");

  beforeEach(async () => {
    density.set("comfortable");
    const apollo = {
      mutate: () =>
        of({ data: { dashboard: { testLlmConnection: connection } } }),
    };
    const data = {
      data$: of(dashboardView(llmEvents())),
      refetch: () => Promise.resolve(),
    };

    TestBed.overrideComponent(DashboardLlmComponent, {
      set: {
        providers: [{ provide: DashboardLlmService, useValue: data }],
      },
    });
    await TestBed.configureTestingModule({
      imports: [DashboardLlmComponent],
      providers: [
        { provide: Apollo, useValue: apollo },
        { provide: ErrorsService, useValue: { addError: () => undefined } },
        { provide: UiPreferences, useValue: { density } },
      ],
    }).compileComponents();

    fixture = TestBed.createComponent(DashboardLlmComponent);
    element = fixture.nativeElement as HTMLElement;
    fixture.detectChanges();
  });

  it("renders slot capacity after a successful connection", () => {
    connection = {
      ok: true,
      connected: true,
      latencySeconds: 0.12,
      capacity: {
        source: "slots",
        contextPerRequest: 8192,
        maxCompletionTokens: 512,
        slots: 16,
        fits: true,
        message: "16 slots × 8192 ctx · fits",
      },
    };

    testConnection();

    const capacity = element.querySelector(".capacity-result")!;
    expect(capacity.textContent).toContain("16 slots × 8192 ctx · fits");
    expect(capacity.classList.contains("warning")).toBeFalse();
  });

  it("renders hosted capacity on failure and warns when it does not fit", () => {
    connection = {
      ok: false,
      error: "configured context exceeds provider capacity",
      connected: false,
      latencySeconds: 0,
      capacity: {
        source: "models",
        contextPerRequest: 128000,
        maxCompletionTokens: 4096,
        slots: null,
        fits: false,
        message: "context 128000 · concurrency is your quota/cost throttle",
      },
    };

    testConnection();

    const capacity = element.querySelector(".capacity-result")!;
    expect(capacity.textContent).toContain(
      "context 128000 · concurrency is your quota/cost throttle",
    );
    expect(capacity.textContent).toContain("⚠");
    expect(capacity.classList.contains("warning")).toBeTrue();
  });

  it("bounds the feed using the density preference", () => {
    expect(feedRows().length).toBe(14);
    expect(feedFooter().textContent).toContain("showing 14 of 37 events");
    expect(loadOlderButton()).not.toBeNull();

    density.set("compact");
    fixture.detectChanges();

    expect(feedRows().length).toBe(25);
    expect(feedFooter().textContent).toContain("showing 25 of 37 events");
  });

  it("reveals ten older events and hides the control when exhausted", () => {
    loadOlderButton()!.click();
    fixture.detectChanges();

    expect(feedRows().length).toBe(24);
    expect(feedFooter().textContent).toContain("showing 24 of 37 events");

    loadOlderButton()!.click();
    fixture.detectChanges();
    loadOlderButton()!.click();
    fixture.detectChanges();

    expect(feedRows().length).toBe(37);
    expect(feedFooter().textContent).toContain("showing 37 of 37 events");
    expect(loadOlderButton()).toBeNull();
  });

  it("resets the visible window when switching filters", () => {
    loadOlderButton()!.click();
    fixture.detectChanges();
    expect(feedRows().length).toBe(24);

    const matchedFilter = Array.from(
      element.querySelectorAll<HTMLButtonElement>(".feed-filters button"),
    ).find((button) => button.textContent?.trim().startsWith("Matched"))!;
    matchedFilter.click();
    fixture.detectChanges();

    expect(fixture.componentInstance.feedLimit()).toBeNull();
    expect(feedRows().length).toBe(14);
    expect(feedFooter().textContent).toContain("showing 14 of 19 events");
  });

  it("aligns mixed-outcome names behind fixed-width badges", () => {
    const rows = feedRows();
    const badges = rows.map(
      (row) => row.querySelector<HTMLElement>(".outcome")!,
    );

    expect(badges.map((badge) => badge.textContent?.trim())).toEqual(
      jasmine.arrayContaining(["MATCHED", "UNMATCHED", "ERROR"]),
    );
    expect(
      badges.every((badge) => getComputedStyle(badge).width === "78px"),
    ).toBeTrue();
    expect(
      badges.every((badge) => getComputedStyle(badge).textAlign === "center"),
    ).toBeTrue();

    const nameStarts = rows.map((row) =>
      Math.round(
        row.querySelector<HTMLElement>(".event-title")!.getBoundingClientRect()
          .left,
      ),
    );
    expect(nameStarts.every((left) => left === nameStarts[0])).toBeTrue();
  });

  function testConnection(): void {
    const button = element.querySelector<HTMLButtonElement>(
      ".form-actions button",
    )!;
    button.click();
    fixture.detectChanges();
  }

  function feedRows(): HTMLElement[] {
    return Array.from(element.querySelectorAll<HTMLElement>(".feed-row"));
  }

  function feedFooter(): HTMLElement {
    return element.querySelector<HTMLElement>(".feed-footer")!;
  }

  function loadOlderButton(): HTMLButtonElement | null {
    return element.querySelector<HTMLButtonElement>(".feed-footer button");
  }
});

function dashboardView(events: LlmEvent[] = []): LlmDashboardView {
  return {
    lastPolledAt: 0,
    summary: {
      totalTorrents: 0,
      torrentsToday: 0,
      indexedLastHour: 0,
      indexedPreviousHour: 0,
      classifiedPercent: 0,
      queueProcessed: 0,
      queuePending: 0,
      queueFailed: 0,
    },
    events,
    stats: {
      attempted: 0,
      matched: 0,
      unmatched: 0,
      errored: 0,
      skipped: 0,
      promptTokens: 0,
      completionTokens: 0,
      successRate: 0,
      perProvider: [],
      errorCategories: [],
      inFlight: 0,
      concurrency: 4,
      effectiveConcurrency: 4,
      windowStart: "2026-07-23T00:00:00Z",
      oldestBuffered: null,
      windowAttempted: 0,
      latencyP50Ms: 0,
      latencyP95Ms: 0,
      throughputPerMinute: 0,
      queuePending: 0,
    },
    config: {
      raw: { Concurrency: 4 },
      llmRaw: {},
      runtimeChangeable: "LIVE_APPLY_AVAILABLE",
      enabled: true,
      concurrency: 4,
      providerName: "openai",
      baseUrl: "https://example.com/v1",
      model: "test-model",
      apiKey: "***REDACTED***",
      batchSize: 1,
      maxContext: 16000,
      maxTokens: 256,
      intervalSeconds: 5,
      timeoutSeconds: 30,
    },
    distribution: [],
    providers: [],
    counts: {
      ALL: events.length,
      MATCHED: events.filter((event) => event.outcome === "MATCHED").length,
      UNMATCHED: events.filter((event) => event.outcome === "UNMATCHED").length,
      ERROR: events.filter((event) => event.outcome === "ERROR").length,
    },
    latencyP50: "0ms",
    latencyP95: "0ms",
    windowSuccessRate: 0,
    windowTruncated: false,
    windowCoverageStart: "2026-07-23T00:00:00Z",
    slots: [false, false, false, false],
    utilization: 0,
    capacityStatus: "keeping up",
    drainRatePerHour: 0,
  };
}

function llmEvents(): LlmEvent[] {
  return Array.from({ length: 37 }, (_, index) => {
    const outcome =
      index % 4 < 2 ? "MATCHED" : index % 4 === 2 ? "UNMATCHED" : "ERROR";

    return {
      timestamp: new Date(Date.UTC(2026, 6, 23, 0, 0, index)).toISOString(),
      infoHash: String(index).padStart(40, "0"),
      torrentName: `Torrent ${index}`,
      provider: outcome === "ERROR" ? "ollama" : "openai",
      durationMs: 1000 + index,
      outcome,
      promptTokens: 20,
      completionTokens: 8,
      contentType: outcome === "MATCHED" ? "movie" : "",
      title: outcome === "MATCHED" ? `Title ${index}` : "",
      year: outcome === "MATCHED" ? 2026 : 0,
      season: 0,
      episode: 0,
      languages: outcome === "MATCHED" ? ["en"] : [],
      error: outcome === "ERROR" ? "provider failed" : "",
    };
  });
}
