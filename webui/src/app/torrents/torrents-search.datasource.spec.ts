import { Observable, Subject } from "rxjs";
import { Apollo } from "apollo-angular";
import { ErrorsService } from "../errors/errors.service";
import * as generated from "../graphql/generated";
import {
  emptyResult,
  TorrentsSearchDatasource,
} from "./torrents-search.datasource";

describe("TorrentsSearchDatasource request lifecycle", () => {
  it("settles failed refreshes, retains results and allows a successful retry", () => {
    const params = new Subject<generated.TorrentContentSearchQueryVariables>();
    const requests: Subject<unknown>[] = [];
    const errors = jasmine.createSpyObj<ErrorsService>("errors", ["addError"]);
    const apollo = {
      query: () => {
        const request = new Subject<unknown>();
        requests.push(request);
        return request;
      },
    };
    const source = new TorrentsSearchDatasource(
      apollo as unknown as Apollo,
      errors,
      params,
    );
    let loading = false;
    const subscription = source.loading$.subscribe(
      (value) => (loading = value),
    );
    const first = { ...emptyResult, totalCount: 7 };
    const next = { ...emptyResult, totalCount: 8 };
    params.next({ input: { limit: 10, page: 2 } });
    requests[0].next({ data: { torrentContent: { search: first } } });
    requests[0].complete();

    source.refresh();
    expect(loading).toBeTrue();
    requests[1].error(new Error("offline"));
    expect(loading).toBeFalse();
    expect(source.result).toEqual(first);
    expect(errors.addError.calls.mostRecent().args).toEqual([
      "Error loading item results: offline",
    ]);

    source.refresh();
    requests[2].next({ data: { torrentContent: { search: next } } });
    requests[2].complete();
    expect(source.result).toEqual(next);
    expect(loading).toBeFalse();
    source.disconnect();
    subscription.unsubscribe();
  });

  it("cancels replaced requests and stops all work after disconnect", () => {
    const params = new Subject<generated.TorrentContentSearchQueryVariables>();
    const requests: Subject<unknown>[] = [];
    let cancelled = 0;
    const apollo = {
      query: () => {
        const request = new Subject<unknown>();
        requests.push(request);
        return new Observable((subscriber) => {
          const subscription = request.subscribe(subscriber);
          return () => {
            cancelled++;
            subscription.unsubscribe();
          };
        });
      },
    };
    const errors = jasmine.createSpyObj<ErrorsService>("errors", ["addError"]);
    const source = new TorrentsSearchDatasource(
      apollo as unknown as Apollo,
      errors,
      params,
    );
    let loading = false;
    source.loading$.subscribe((value) => (loading = value));
    params.next({ input: { page: 1 } });
    params.next({ input: { page: 2 } });
    expect(cancelled).toBe(1);
    expect(loading).toBeTrue();
    const current = { ...emptyResult, totalCount: 20 };
    const obsolete = { ...emptyResult, totalCount: 10 };
    requests[1].next({ data: { torrentContent: { search: current } } });
    requests[0].next({ data: { torrentContent: { search: obsolete } } });
    expect(source.result).toEqual(current);

    source.refresh();
    expect(loading).toBeTrue();
    source.disconnect();
    expect(cancelled).toBe(3);
    expect(loading).toBeFalse();
    requests[2].next({ data: { torrentContent: { search: obsolete } } });
    params.next({ input: { page: 3 } });
    source.refresh();
    source.disconnect();
    expect(requests.length).toBe(3);
    expect(source.result).toEqual(current);
  });

  it("disposes a request from a synchronous result observer", () => {
    const params = new Subject<generated.TorrentContentSearchQueryVariables>();
    let cancelled = false;
    const first = { ...emptyResult, totalCount: 7 };
    const apollo = {
      query: () =>
        new Observable((subscriber) => {
          subscriber.next({ data: { torrentContent: { search: first } } });
          subscriber.next({
            data: {
              torrentContent: { search: { ...emptyResult, totalCount: 8 } },
            },
          });
          return () => {
            cancelled = true;
          };
        }),
    };
    const errors = jasmine.createSpyObj<ErrorsService>("errors", ["addError"]);
    const source = new TorrentsSearchDatasource(
      apollo as unknown as Apollo,
      errors,
      params,
    );
    source.result$.subscribe((result) => {
      if (result.totalCount) source.disconnect();
    });
    params.next({ input: { page: 1 } });
    expect(cancelled).toBeTrue();
    expect(source.result).toEqual(first);
  });

  it("does not start a request when a loading observer leaves search", () => {
    const params = new Subject<generated.TorrentContentSearchQueryVariables>();
    let requests = 0;
    const apollo = {
      query: () => {
        requests++;
        return new Subject();
      },
    };
    const errors = jasmine.createSpyObj<ErrorsService>("errors", ["addError"]);
    const source = new TorrentsSearchDatasource(
      apollo as unknown as Apollo,
      errors,
      params,
    );
    source.loading$.subscribe((loading) => {
      if (loading) source.disconnect();
    });
    params.next({ input: { page: 1 } });
    expect(requests).toBe(0);
  });
});
