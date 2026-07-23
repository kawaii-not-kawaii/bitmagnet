import { AsyncPipe, DatePipe, DecimalPipe, PercentPipe } from "@angular/common";
import { Component, OnDestroy, inject } from "@angular/core";
import {
  FormBuilder,
  ReactiveFormsModule,
  Validators,
} from "@angular/forms";
import { Apollo } from "apollo-angular";
import { finalize, tap } from "rxjs";
import { ErrorsService } from "../../errors/errors.service";
import * as generated from "../../graphql/generated";
import {
  DashboardLlmService,
  FeedFilter,
  LlmDashboardView,
  LlmEvent,
  REDACTED_VALUE,
  buildClassifierConfigValue,
  filterLlmEvents,
  formatDuration,
  mapClassifierConfig,
} from "./dashboard-llm.service";

@Component({
  selector: "app-dashboard-llm",
  imports: [AsyncPipe, DatePipe, DecimalPipe, PercentPipe, ReactiveFormsModule],
  providers: [DashboardLlmService],
  templateUrl: "./dashboard-llm.component.html",
  styleUrl: "./dashboard-llm.component.scss",
})
export class DashboardLlmComponent implements OnDestroy {
  private apollo = inject(Apollo);
  private data = inject(DashboardLlmService);
  private errors = inject(ErrorsService);
  private fb = inject(FormBuilder);
  private required = Validators.required.bind(Validators);
  private freshnessTimer?: number;
  private connectionTimer?: number;
  private formInitialized = false;
  private lastView?: LlmDashboardView;

  readonly filters: { key: FeedFilter; label: string }[] = [
    { key: "ALL", label: "All" },
    { key: "MATCHED", label: "Matched" },
    { key: "UNMATCHED", label: "Unmatched" },
    { key: "ERROR", label: "Error" },
  ];
  readonly view$ = this.data.data$.pipe(tap((view) => this.acceptView(view)));
  readonly formatDuration = formatDuration;

  form = this.fb.nonNullable.group({
    enabled: false,
    concurrency: [10, [this.required, Validators.min(1)]],
    providerName: ["default", this.required],
    baseUrl: ["", this.required],
    model: ["", this.required],
    apiKey: REDACTED_VALUE,
    batchSize: [5, [this.required, Validators.min(1)]],
    maxContext: [16000, [this.required, Validators.min(1)]],
    maxTokens: [256, [this.required, Validators.min(1)]],
    intervalSeconds: [5, [this.required, Validators.min(0)]],
    timeoutSeconds: [30, [this.required, Validators.min(1)]],
  });
  benchmarkSampleSize = this.fb.nonNullable.control(20, [
    this.required,
    Validators.min(1),
    Validators.max(500),
  ]);

  feedFilter: FeedFilter = "ALL";
  openEvent?: string;
  pollFresh = false;
  lastPollResponseAt?: Date;
  saving = false;
  benchmarking = false;
  saveMessage = "";
  testState: "idle" | "testing" | "ok" | "error" = "idle";
  connectionMessage = "";
  connectionCapacity?: generated.DashboardLlmTestConnectionMutation["dashboard"]["testLlmConnection"]["capacity"];
  benchmark?: generated.DashboardLlmRunBenchmarkMutation["dashboard"]["runLlmBenchmark"];

  ngOnDestroy() {
    clearTimeout(this.freshnessTimer);
    clearTimeout(this.connectionTimer);
  }

  filteredEvents(events: LlmEvent[]) {
    return filterLlmEvents(events, this.feedFilter);
  }

  selectFilter(filter: FeedFilter) {
    this.feedFilter = filter;
    this.openEvent = undefined;
  }

  toggleEvent(event: LlmEvent) {
    const key = this.eventKey(event);
    this.openEvent = this.openEvent === key ? undefined : key;
  }

  eventKey(event: LlmEvent) {
    return `${event.timestamp}:${event.infoHash}:${event.outcome}`;
  }

  episodeLabel(event: LlmEvent) {
    if (event.season === 0 && event.episode === 0) {
      return "—";
    }

    return `S${String(event.season).padStart(2, "0")}E${String(event.episode).padStart(2, "0")}`;
  }

  toggleEnabled() {
    if (!this.lastView) {
      return;
    }

    this.save();
  }

  save() {
    if (!this.lastView || this.form.invalid) {
      this.form.markAllAsTouched();
      return;
    }

    // An untouched redacted API key is passed through as-is: the backend
    // recognizes the placeholder and keeps the configured value.
    const value = buildClassifierConfigValue(
      this.lastView.config,
      this.form.getRawValue(),
    );
    this.saving = true;
    this.saveMessage = "";

    this.apollo
      .mutate<
        generated.DashboardLlmSetConfigMutation,
        generated.DashboardLlmSetConfigMutationVariables
      >({
        mutation: generated.DashboardLlmSetConfigDocument,
        variables: { value },
      })
      .pipe(finalize(() => (this.saving = false)))
      .subscribe({
        next: (result) => {
          const applied = result.data?.config.setSection;
          if (!applied) {
            return;
          }

          const config = mapClassifierConfig(applied.section);
          this.lastView = { ...this.lastView!, config };
          this.patchForm(config);
          this.saveMessage =
            applied.applied === "LIVE_APPLY_AVAILABLE"
              ? "Configuration applied live"
              : "Configuration saved · restart required";
          void this.data.refetch();
        },
        error: (error: Error) => this.errors.addError(error.message),
      });
  }

  testConnection() {
    this.testState = "testing";
    this.connectionMessage = "";
    this.connectionCapacity = undefined;
    clearTimeout(this.connectionTimer);

    this.apollo
      .mutate<generated.DashboardLlmTestConnectionMutation>({
        mutation: generated.DashboardLlmTestConnectionDocument,
      })
      .pipe(
        finalize(() => {
          if (this.testState === "testing") {
            this.testState = "idle";
          }
        }),
      )
      .subscribe({
        next: (result) => {
          const connection = result.data?.dashboard.testLlmConnection;
          this.connectionCapacity = connection?.capacity ?? undefined;
          if (connection?.ok && connection.connected) {
            this.testState = "ok";
            this.connectionMessage = `✓ Connected · ${connection.latencySeconds.toFixed(2)}s`;
            this.connectionTimer = setTimeout(() => {
              this.testState = "idle";
              this.connectionMessage = "";
              this.connectionCapacity = undefined;
            }, 3200);
            return;
          }

          this.testState = "error";
          this.connectionMessage = connection?.error ?? "Connection failed";
        },
        error: (error: Error) => {
          this.testState = "error";
          this.connectionMessage = error.message;
          this.errors.addError(error.message);
        },
      });
  }

  useRecommendedConcurrency(slots: number) {
    this.form.controls.concurrency.setValue(slots);
    this.form.controls.concurrency.markAsDirty();
  }

  runBenchmark() {
    if (this.benchmarkSampleSize.invalid) {
      this.benchmarkSampleSize.markAsTouched();
      return;
    }

    this.benchmarking = true;
    this.benchmark = undefined;
    this.apollo
      .mutate<
        generated.DashboardLlmRunBenchmarkMutation,
        generated.DashboardLlmRunBenchmarkMutationVariables
      >({
        mutation: generated.DashboardLlmRunBenchmarkDocument,
        variables: { sampleSize: this.benchmarkSampleSize.value },
      })
      .pipe(finalize(() => (this.benchmarking = false)))
      .subscribe({
        next: (result) => {
          this.benchmark = result.data?.dashboard.runLlmBenchmark;
        },
        error: (error: Error) => this.errors.addError(error.message),
      });
  }

  distributionWidth(count: number, distribution: { count: number }[]) {
    const maximum = Math.max(1, ...distribution.map((entry) => entry.count));
    return `${(count / maximum) * 100}%`;
  }

  private acceptView(view: LlmDashboardView) {
    this.lastView = view;
    this.lastPollResponseAt = new Date(view.lastPolledAt);
    this.pollFresh = true;
    clearTimeout(this.freshnessTimer);
    this.freshnessTimer = setTimeout(() => (this.pollFresh = false), 6500);

    if (!this.formInitialized) {
      this.patchForm(view.config);
      this.formInitialized = true;
    }
  }

  private patchForm(config: LlmDashboardView["config"]) {
    this.form.patchValue({
      enabled: config.enabled,
      concurrency: config.concurrency,
      providerName: config.providerName,
      baseUrl: config.baseUrl,
      model: config.model,
      apiKey: config.apiKey,
      batchSize: config.batchSize,
      maxContext: config.maxContext,
      maxTokens: config.maxTokens,
      intervalSeconds: config.intervalSeconds,
      timeoutSeconds: config.timeoutSeconds,
    });
  }
}
