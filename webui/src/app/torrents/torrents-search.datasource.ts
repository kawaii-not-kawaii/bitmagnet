import { CollectionViewer, DataSource } from "@angular/cdk/collections";
import {
  BehaviorSubject,
  Observable,
  scan,
  Subscriber,
  Subscription,
} from "rxjs";
import { map } from "rxjs/operators";
import { Apollo } from "apollo-angular";
import * as generated from "../graphql/generated";
import { ErrorsService } from "../errors/errors.service";

export const emptyResult: generated.TorrentContentSearchResult = {
  items: [],
  totalCount: 0,
  totalCountIsEstimate: false,
  aggregations: {},
};

type BudgetedCount = {
  count: number;
  isEstimate: boolean;
};

export class TorrentsSearchDatasource implements DataSource<generated.TorrentContent> {
  private input?: generated.TorrentContentSearchQueryInput;

  private currentRequest = 0;
  private subscriptions = new Subscription();
  private currentSubscription?: Subscription;

  private loadingSubject = new BehaviorSubject(false);
  public loading$ = this.loadingSubject.asObservable();

  public result: generated.TorrentContentSearchResult = emptyResult;
  private resultSubject =
    new BehaviorSubject<generated.TorrentContentSearchResult>(this.result);
  public result$ = this.resultSubject.asObservable();

  public items$ = this.resultSubject.pipe(map((result) => result.items));

  public overallTotalCount$ = this.resultSubject.pipe(
    map((result) => {
      let overallTotalCount = 0;
      let overallIsEstimate = false;
      for (const ct of result.aggregations.contentType ?? []) {
        overallTotalCount += ct.count;
        overallIsEstimate = overallIsEstimate || ct.isEstimate;
      }
      return {
        count: overallTotalCount,
        isEstimate: overallIsEstimate,
      };
    }),
  );
  public availableContentTypes$: Observable<generated.ContentType[]> =
    this.resultSubject.pipe(
      scan(
        (acc: generated.ContentType[], next) =>
          Array.from(
            new Set([
              ...acc,
              ...(next.aggregations.contentType ?? []).flatMap((agg) =>
                agg.value ? [agg.value] : [],
              ),
            ]),
          ),
        [],
      ),
    );

  public contentTypeCounts$: Observable<Record<string, BudgetedCount>> =
    this.resultSubject.pipe(
      map((result) =>
        Object.fromEntries<BudgetedCount>(
          (result.aggregations.contentType ?? []).map((ct) => [
            ct.value as string,
            {
              count: ct.count,
              isEstimate: ct.isEstimate,
            },
          ]),
        ),
      ),
    );

  constructor(
    private apollo: Apollo,
    private errorsService: ErrorsService,
    searchQueryVariables: Observable<generated.TorrentContentSearchQueryVariables>,
  ) {
    this.subscriptions.add(
      searchQueryVariables.subscribe((variables) => {
        this.input = variables.input;
        this.refresh();
      }),
    );
  }

  connect({}: CollectionViewer): Observable<generated.TorrentContent[]> {
    return this.items$;
  }

  disconnect(): void {
    if (this.subscriptions.closed) {
      return;
    }
    ++this.currentRequest;
    this.subscriptions.unsubscribe();
    this.currentSubscription?.unsubscribe();
    this.loadingSubject.next(false);
    this.loadingSubject.complete();
    this.resultSubject.complete();
  }

  refresh() {
    if (this.input) {
      this.loadResult({ input: this.input });
    }
  }

  private loadResult(
    variables: generated.TorrentContentSearchQueryVariables,
  ): void {
    if (this.subscriptions.closed) {
      return;
    }
    const currentRequest = ++this.currentRequest;
    this.currentSubscription?.unsubscribe();
    this.loadingSubject.next(true);
    if (this.subscriptions.closed || currentRequest !== this.currentRequest) {
      return;
    }
    const result = this.apollo
      .query<
        generated.TorrentContentSearchQuery,
        generated.TorrentContentSearchQueryVariables
      >({
        query: generated.TorrentContentSearchDocument,
        // Omitted cached bypasses both backend cache reads and writes.
        variables: { input: { ...variables.input, cached: undefined } },
        fetchPolicy: "no-cache",
      })
      .pipe(map((r) => r.data.torrentContent.search));
    const subscriber = new Subscriber<generated.TorrentContentSearchResult>({
      next: (r: generated.TorrentContentSearchResult) => {
        if (currentRequest !== this.currentRequest) {
          return;
        }
        this.loadingSubject.next(false);
        if (currentRequest === this.currentRequest) {
          this.result = r;
          this.resultSubject.next(r);
        }
      },
      error: (err: Error) => {
        this.errorsService.addError(
          `Error loading item results: ${err.message}`,
        );
        if (currentRequest === this.currentRequest) {
          this.loadingSubject.next(false);
        }
      },
      complete: () => {
        if (currentRequest === this.currentRequest) {
          this.loadingSubject.next(false);
        }
      },
    });
    this.currentSubscription = subscriber;
    result.subscribe(subscriber);
  }
}
