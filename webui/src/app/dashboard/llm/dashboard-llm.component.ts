import { Component, inject } from "@angular/core";
import { FormBuilder, Validators } from "@angular/forms";
import { Apollo } from "apollo-angular";
import { finalize, map, take } from "rxjs";
import { AppModule } from "../../app.module";
import { ErrorsService } from "../../errors/errors.service";
import * as generated from "../../graphql/generated";
import { GraphQLModule } from "../../graphql/graphql.module";

@Component({
  selector: "app-dashboard-llm",
  standalone: true,
  imports: [AppModule, GraphQLModule],
  templateUrl: "./dashboard-llm.component.html",
  styleUrl: "./dashboard-llm.component.scss",
})
export class DashboardLlmComponent {
  private apollo = inject(Apollo);
  private errors = inject(ErrorsService);
  private fb = inject(FormBuilder);
  private required = Validators.required.bind(Validators);
  private query = this.apollo.watchQuery<generated.DashboardDataQuery>({
    query: generated.DashboardDataDocument,
    fetchPolicy: "network-only",
    pollInterval: 30000,
  });

  llm$ = this.query.valueChanges.pipe(
    map((result) => result.data.dashboard.llm),
  );
  form = this.fb.nonNullable.group({
    enabled: false,
    providerName: ["default", this.required],
    baseUrl: ["", this.required],
    model: ["", this.required],
    apiKey: "",
    batchSize: [5, [this.required, Validators.min(1)]],
    maxContext: [16000, [this.required, Validators.min(1)]],
    maxTokens: [256, [this.required, Validators.min(1)]],
    intervalSeconds: [5, [this.required, Validators.min(0)]],
    timeoutSeconds: [30, [this.required, Validators.min(1)]],
  });

  saving = false;
  testing = false;
  benchmarking = false;
  saveMessage = "";
  connectionResult?: generated.DashboardLlmTestConnectionMutation["dashboard"]["testLlmConnection"];
  benchmark?: generated.DashboardLlmRunBenchmarkMutation["dashboard"]["runLlmBenchmark"];

  constructor() {
    this.llm$.pipe(take(1)).subscribe(({ state }) => {
      this.form.patchValue({
        enabled: state.enabled,
        providerName: state.providerName,
        baseUrl: state.baseUrl,
        model: state.model,
        batchSize: state.batchSize,
        maxContext: state.maxContext,
        maxTokens: state.maxTokens,
        intervalSeconds: state.intervalSeconds,
        timeoutSeconds: state.timeoutSeconds,
      });
    });
  }

  save() {
    if (this.form.invalid) {
      this.form.markAllAsTouched();
      return;
    }
    const { apiKey, ...input } = this.form.getRawValue();
    this.saving = true;
    this.saveMessage = "";
    this.apollo
      .mutate<generated.DashboardLlmUpdateMutation>({
        mutation: generated.DashboardLlmUpdateDocument,
        variables: { input: { ...input, apiKey: apiKey || null } },
      })
      .pipe(finalize(() => (this.saving = false)))
      .subscribe({
        next: () => {
          this.saveMessage = "Configuration saved";
          this.form.controls.apiKey.setValue("");
          void this.query.refetch();
        },
        error: (error: Error) => this.errors.addError(error.message),
      });
  }

  testConnection() {
    this.testing = true;
    this.connectionResult = undefined;
    this.apollo
      .mutate<generated.DashboardLlmTestConnectionMutation>({
        mutation: generated.DashboardLlmTestConnectionDocument,
      })
      .pipe(finalize(() => (this.testing = false)))
      .subscribe({
        next: (result) => {
          this.connectionResult = result.data?.dashboard.testLlmConnection;
        },
        error: (error: Error) => this.errors.addError(error.message),
      });
  }

  runBenchmark() {
    this.benchmarking = true;
    this.benchmark = undefined;
    this.apollo
      .mutate<generated.DashboardLlmRunBenchmarkMutation>({
        mutation: generated.DashboardLlmRunBenchmarkDocument,
        variables: { sampleSize: 20 },
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
}
