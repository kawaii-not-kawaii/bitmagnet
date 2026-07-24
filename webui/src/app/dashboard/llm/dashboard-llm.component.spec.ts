import { ComponentFixture, TestBed } from "@angular/core/testing";
import { Apollo } from "apollo-angular";
import { of } from "rxjs";
import { ErrorsService } from "../../errors/errors.service";
import type * as generated from "../../graphql/generated";
import { DashboardLlmComponent } from "./dashboard-llm.component";
import { DashboardLlmService } from "./dashboard-llm.service";
import type { LlmDashboardView } from "./dashboard-llm.service";

type ConnectionResult =
  generated.DashboardLlmTestConnectionMutation["dashboard"]["testLlmConnection"];

describe("DashboardLlmComponent capacity result", () => {
  let connection: ConnectionResult;
  let element: HTMLElement;
  let fixture: ComponentFixture<DashboardLlmComponent>;

  beforeEach(async () => {
    const apollo = {
      mutate: () =>
        of({ data: { dashboard: { testLlmConnection: connection } } }),
    };
    const data = {
      data$: of(dashboardView()),
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

  function testConnection(): void {
    const button = element.querySelector<HTMLButtonElement>(
      ".form-actions button",
    )!;
    button.click();
    fixture.detectChanges();
  }
});

function dashboardView(): LlmDashboardView {
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
    events: [],
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
    counts: { ALL: 0, MATCHED: 0, UNMATCHED: 0, ERROR: 0 },
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
