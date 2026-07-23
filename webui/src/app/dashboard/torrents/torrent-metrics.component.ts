import { Component, inject, OnDestroy } from "@angular/core";
import { format as formatDate } from "date-fns/format";
import { Apollo } from "apollo-angular";
import { map } from "rxjs/operators";
import { AppModule } from "../../app.module";
import {
  BarChartBucket,
  BarChartComponent,
  BarChartSeries,
} from "../../charting/bar-chart.component";
import { ErrorsService } from "../../errors/errors.service";
import {
  autoRefreshIntervalNames,
  defaultBucketParams,
  durationSeconds,
  eventNames,
  resolutionNames,
  timeframeNames,
} from "./torrent-metrics.constants";
import { TorrentMetricsController } from "./torrent-metrics.controller";
import { normalizeBucket } from "./torrent-metrics.utils";
import type { EventName, Result } from "./torrent-metrics.types";

type MetricsView = {
  result: Result;
  buckets: BarChartBucket[];
  series: BarChartSeries[];
  peakThroughput: number;
};
@Component({
  selector: "app-torrent-metrics",
  templateUrl: "./torrent-metrics.component.html",
  styleUrl: "./torrent-metrics.component.scss",
  imports: [AppModule, BarChartComponent],
})
export class TorrentMetricsComponent implements OnDestroy {
  private apollo = inject(Apollo);
  torrentMetricsController = new TorrentMetricsController(
    this.apollo,
    {
      buckets: defaultBucketParams,
      autoRefresh: "seconds_30",
    },
    inject(ErrorsService),
  );

  protected readonly resolutionNames = resolutionNames;
  protected readonly timeframeNames = timeframeNames;
  protected readonly autoRefreshIntervalNames = autoRefreshIntervalNames;
  protected readonly view$ = this.torrentMetricsController.result$.pipe(
    map((result) => (result ? this.createChartView(result) : undefined)),
  );

  ngOnDestroy() {
    this.torrentMetricsController.setAutoRefreshInterval("off");
  }

  protected readonly eventNames = eventNames;

  eventTotal(result: Result, eventName?: EventName) {
    let total = 0;
    const names: readonly EventName[] = eventName ? [eventName] : eventNames;
    for (const source of result.sourceSummaries) {
      for (const name of names) {
        const entries = source.events?.eventBuckets[name]?.entries ?? {};
        for (const entry of Object.values(entries)) {
          total += entry?.count ?? 0;
        }
      }
    }
    return total;
  }

  optionLabel(value: string) {
    if (value === "off") return "Off";
    const [unit, amount] = value.split("_");
    return amount ? `${amount}${unit[0]}` : value;
  }

  handleMultiplierEvent(event: Event) {
    const value = (event.currentTarget as HTMLInputElement).value;
    this.torrentMetricsController.setBucketMultiplier(
      /^\d+$/.test(value) ? parseInt(value) : "AUTO",
    );
  }

  private createChartView(result: Result): MetricsView {
    const events = eventNames.filter(
      (event) => !result.params.event || result.params.event === event,
    );
    const latestBucket = Math.max(
      result.bucketSpan?.latestBucket ?? 0,
      normalizeBucket(new Date(), result.params.buckets).index,
    );
    const buckets = Array.from({ length: 12 }, (_, index) => {
      const bucket = latestBucket - 11 + index;
      const timestamp =
        1000 *
        durationSeconds[result.params.buckets.duration] *
        result.params.buckets.multiplier *
        bucket;

      return {
        label:
          index % 2 === 0
            ? formatDate(
                timestamp,
                result.params.buckets.duration === "day" ? "d LLL" : "HH:mm",
              )
            : "",
        values: events.map((event) =>
          result.sourceSummaries.reduce(
            (total, source) =>
              total +
              (source.events?.eventBuckets[event]?.entries[`${bucket}`]
                ?.count ?? 0),
            0,
          ),
        ),
      };
    });

    return {
      result,
      buckets,
      series: events.map((event) => ({
        label: event === "created" ? "Created" : "Updated",
        tone: event === "created" ? "primary" : "secondary",
      })),
      peakThroughput: Math.max(
        ...buckets.map((bucket) =>
          bucket.values.reduce((total, value) => total + value, 0),
        ),
        0,
      ),
    };
  }
}
