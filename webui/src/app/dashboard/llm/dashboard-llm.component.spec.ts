import { signal } from "@angular/core";
import { ComponentFixture, TestBed } from "@angular/core/testing";
import { Apollo } from "apollo-angular";
import { of } from "rxjs";
import { ErrorsService } from "../../errors/errors.service";
import * as generated from "../../graphql/generated";
import { UiPreferences } from "../../layout/ui-preferences.service";
import { DashboardLlmComponent } from "./dashboard-llm.component";
import { DashboardLlmService, REDACTED_VALUE } from "./dashboard-llm.service";
import type { LlmDashboardView, LlmEvent } from "./dashboard-llm.service";
import { LLM_PRESETS_KEY } from "./dashboard-llm.presets";

type ConnectionResult =
  generated.DashboardLlmTestConnectionMutation["dashboard"]["testLlmConnection"];

describe("DashboardLlmComponent", () => {
  let connection: ConnectionResult;
  let element: HTMLElement;
  let fixture: ComponentFixture<DashboardLlmComponent>;
  const density = signal<"comfortable" | "compact">("comfortable");
  let setSectionMutationCalls: number;
  let view: LlmDashboardView;

  beforeEach(async () => {
    localStorage.removeItem(LLM_PRESETS_KEY);
    density.set("comfortable");
    setSectionMutationCalls = 0;
    view = dashboardView(llmEvents());
    const apollo = {
      mutate: jasmine
        .createSpy("mutate")
        .and.callFake((options: { mutation: unknown }) => {
          if (options.mutation === generated.DashboardLlmSetConfigDocument) {
            setSectionMutationCalls += 1;
          }

          return of({
            data: { dashboard: { testLlmConnection: connection } },
          });
        }),
    };
    const data = {
      data$: of(view),
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

  afterEach(() => {
    localStorage.removeItem(LLM_PRESETS_KEY);
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

  it("applies exactly five recommended controls without saving", () => {
    const component = fixture.componentInstance;
    component.form.patchValue({
      autoScale: true,
      providerName: "custom-provider",
      apiKey: "live-secret",
      intervalSeconds: 17,
      batchSize: 3,
      maxTokens: 128,
      maxContext: 4096,
      timeoutSeconds: 20,
      concurrency: 6,
    });
    component.form.markAsPristine();
    const before = component.form.getRawValue();
    connection = recommendedConnection();

    testConnection();
    buttonWithText("Apply recommendation")!.click();
    fixture.detectChanges();

    expect(component.form.getRawValue()).toEqual({
      ...before,
      batchSize: 1,
      maxTokens: 384,
      maxContext: 8192,
      timeoutSeconds: 45,
      concurrency: 12,
    });

    for (const name of [
      "batchSize",
      "maxTokens",
      "maxContext",
      "timeoutSeconds",
      "concurrency",
    ] as const) {
      expect(component.form.controls[name].dirty)
        .withContext(`${name} should be dirty`)
        .toBeTrue();
    }

    for (const name of [
      "enabled",
      "autoScale",
      "providerName",
      "baseUrl",
      "model",
      "apiKey",
      "intervalSeconds",
    ] as const) {
      expect(component.form.controls[name].pristine)
        .withContext(`${name} should remain pristine`)
        .toBeTrue();
    }

    expect(setSectionMutationCalls).toBe(0);
  });

  it("leaves ignored recommendations pristine and hides apply after failure", () => {
    connection = recommendedConnection();

    testConnection();

    expect(fixture.componentInstance.form.pristine).toBeTrue();
    expect(buttonWithText("Apply recommendation")).not.toBeNull();

    connection = {
      ok: false,
      error: "unauthorized",
      connected: false,
      latencySeconds: 0,
      capacity: {
        source: "unknown",
        contextPerRequest: null,
        maxCompletionTokens: null,
        slots: null,
        fits: null,
        message: "capacity unknown",
      },
    };
    testConnection();

    expect(fixture.componentInstance.form.pristine).toBeTrue();
    expect(buttonWithText("Apply recommendation")).toBeNull();
    expect(setSectionMutationCalls).toBe(0);
  });

  it("redacts secrets saved through preset UI and rejects blank names", () => {
    const component = fixture.componentInstance;
    component.form.controls.apiKey.setValue("sk-ui-secret");
    component.presetName.setValue("   ");
    fixture.detectChanges();

    expect(buttonWithText("Save preset")!.disabled).toBeTrue();
    component.saveCurrentPreset();
    expect(localStorage.getItem(LLM_PRESETS_KEY)).toBeNull();

    component.presetName.setValue("Hosted");
    fixture.detectChanges();
    buttonWithText("Save preset")!.click();
    fixture.detectChanges();

    const stored = localStorage.getItem(LLM_PRESETS_KEY)!;
    expect(stored).not.toContain("sk-ui-secret");
    expect(stored).toContain(REDACTED_VALUE);
    expect(presetRow("Hosted")).not.toBeNull();

    presetRow("Hosted")!
      .querySelector<HTMLButtonElement>(".delete-preset")!
      .click();
    fixture.detectChanges();

    expect(component.presets).toEqual([]);
    expect(presetRow("Hosted")).toBeNull();
    expect(setSectionMutationCalls).toBe(0);
  });

  it("preserves same-provider credentials and clears them when switching", () => {
    const component = fixture.componentInstance;
    component.form.patchValue({
      providerName: "openai",
      baseUrl: "https://example.com/v1/",
      apiKey: "same-secret",
    });
    component.presetName.setValue("Same provider");
    component.saveCurrentPreset();

    component.form.patchValue({
      providerName: "other",
      baseUrl: "https://other.example/v1",
      apiKey: "other-secret",
    });
    component.presetName.setValue("Other provider");
    component.saveCurrentPreset();
    fixture.detectChanges();

    component.form.patchValue({
      providerName: "openai",
      baseUrl: "https://example.com/v1",
      apiKey: "current-secret",
    });
    component.form.markAsPristine();
    presetRow("Same provider")!
      .querySelector<HTMLButtonElement>(".load-preset")!
      .click();
    fixture.detectChanges();

    expect(component.form.controls.apiKey.value).toBe("current-secret");
    expect(component.form.dirty).toBeTrue();

    component.form.markAsPristine();
    presetRow("Other provider")!
      .querySelector<HTMLButtonElement>(".load-preset")!
      .click();
    fixture.detectChanges();

    expect(component.form.controls.providerName.value).toBe("other");
    expect(component.form.controls.apiKey.value).toBe("");
    expect(component.form.dirty).toBeTrue();
    expect(setSectionMutationCalls).toBe(0);
  });

  it("renders effective concurrency, a nonredundant ceiling, and zero state", () => {
    view.stats.inFlight = 3;
    view.stats.concurrency = 8;
    view.stats.effectiveConcurrency = 4;
    view.effectiveConcurrency = 4;
    view.concurrencyCeiling = 8;
    view.slots = [true, true, true, false];
    view.utilization = 0.75;
    fixture.detectChanges();

    expect(element.querySelector(".slot-heading")!.textContent).toContain(
      "3 / 4",
    );
    expect(element.querySelector(".slot-ceiling")!.textContent).toContain(
      "ceiling 8",
    );

    view.stats.inFlight = 2;
    view.stats.concurrency = 8;
    view.stats.effectiveConcurrency = 8;
    view.effectiveConcurrency = 8;
    view.concurrencyCeiling = 8;
    view.slots = [true, true, false, false, false, false, false, false];
    fixture.detectChanges();

    expect(element.querySelector(".slot-heading")!.textContent).toContain(
      "2 / 8",
    );
    expect(element.querySelector(".slot-ceiling")).toBeNull();

    view.stats.inFlight = 0;
    view.stats.concurrency = 0;
    view.stats.effectiveConcurrency = 0;
    view.effectiveConcurrency = 0;
    view.concurrencyCeiling = 0;
    view.slots = [];
    view.utilization = 0;
    fixture.detectChanges();

    expect(element.querySelector(".slot-gauge")!.textContent).toContain(
      "No concurrency configured",
    );
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

  function buttonWithText(label: string): HTMLButtonElement | null {
    return (
      Array.from(element.querySelectorAll<HTMLButtonElement>("button")).find(
        (button) => button.textContent?.trim() === label,
      ) ?? null
    );
  }

  function presetRow(name: string): HTMLElement | null {
    return (
      Array.from(element.querySelectorAll<HTMLElement>(".preset-row")).find(
        (row) => row.querySelector("strong")?.textContent?.trim() === name,
      ) ?? null
    );
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

function recommendedConnection(): ConnectionResult {
  return {
    ok: true,
    connected: true,
    latencySeconds: 0.25,
    capacity: {
      source: "slots",
      contextPerRequest: 8192,
      maxCompletionTokens: 384,
      slots: 12,
      fits: true,
      recommendedConcurrency: 12,
      message: "12 slots × 8192 ctx · fits",
      recommendedConfig: {
        batchSize: 1,
        maxTokens: 384,
        maxContext: 8192,
        timeoutSeconds: 45,
        concurrency: 12,
      },
    },
  };
}

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
      autoScale: false,
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
    effectiveConcurrency: 4,
    concurrencyCeiling: 4,
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
