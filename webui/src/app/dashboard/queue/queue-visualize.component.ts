import { Component, inject, OnDestroy } from "@angular/core";
import { format as formatDate } from "date-fns/format";
import { Apollo } from "apollo-angular";
import { Observable } from "rxjs";
import { map } from "rxjs/operators";
import { ErrorsService } from "../../errors/errors.service";
import { AppModule } from "../../app.module";
import { DocumentTitleComponent } from "../../layout/document-title.component";
import { QueueJobStatus } from "../../graphql/generated";
import {
  availableQueueNames,
  durationSeconds,
  eventNames,
} from "./queue.constants";
import {
  normalizeBucket,
  QueueMetricsController,
} from "./queue-metrics.controller";
import { EventName, Result, TimeframeName } from "./queue-metrics.types";

type StatusTotal = {
  status: QueueJobStatus;
  count: number;
  percent: number;
};

type ThroughputBar = {
  count: number;
  height: number;
  label: string;
};

type VisualizeView = {
  queueLabel: string;
  statusTotals: StatusTotal[];
  bars: ThroughputBar[];
};

const statusOrder: QueueJobStatus[] = [
  "processed",
  "pending",
  "retry",
  "failed",
];

@Component({
  selector: "app-queue-visualize",
  standalone: true,
  templateUrl: "./queue-visualize.component.html",
  styleUrl: "./queue-visualize.component.scss",
  imports: [AppModule, DocumentTitleComponent],
})
export class QueueVisualizeComponent implements OnDestroy {
  private apollo = inject(Apollo);
  protected readonly queueMetricsController = new QueueMetricsController(
    this.apollo,
    {
      buckets: {
        duration: "AUTO",
        multiplier: "AUTO",
        timeframe: "hours_1",
      },
      autoRefresh: "off",
    },
    inject(ErrorsService),
  );

  protected readonly timeframes: {
    value: TimeframeName;
    label: string;
  }[] = [
    { value: "minutes_15", label: "15m" },
    { value: "minutes_30", label: "30m" },
    { value: "hours_1", label: "1h" },
    { value: "hours_6", label: "6h" },
    { value: "hours_12", label: "12h" },
    { value: "days_1", label: "1d" },
    { value: "weeks_1", label: "1w" },
  ];
  protected readonly availableQueueNames = availableQueueNames;
  protected readonly eventNames = eventNames;
  protected readonly view$: Observable<VisualizeView> =
    this.queueMetricsController.result$.pipe(
      map((result) => this.createView(result)),
    );

  ngOnDestroy() {
    this.queueMetricsController.setAutoRefreshInterval("off");
  }

  refresh() {
    this.queueMetricsController.refresh();
  }

  private createView(result: Result | undefined): VisualizeView {
    const statusCounts = statusOrder.map((status) => ({
      status,
      count:
        result?.queues.reduce(
          (total, queue) => total + queue.statusCounts[status],
          0,
        ) ?? 0,
    }));
    const maxStatus = Math.max(...statusCounts.map(({ count }) => count));

    return {
      queueLabel:
        result?.params.queue ??
        this.queueMetricsController.params.queue ??
        "all queues",
      statusTotals: statusCounts.map(({ status, count }) => ({
        status,
        count,
        percent: count ? Math.max(3, Math.round((count / maxStatus) * 100)) : 0,
      })),
      bars: result ? this.createBars(result) : [],
    };
  }

  private createBars(result: Result): ThroughputBar[] {
    const counts = new Map<number, number>();
    const events: readonly EventName[] = result.params.event
      ? [result.params.event]
      : eventNames;

    for (const queue of result.queues) {
      for (const event of events) {
        for (const [bucket, entry] of Object.entries(
          queue.events?.eventBuckets[event]?.entries ?? {},
        )) {
          if (!entry) {
            continue;
          }
          counts.set(
            parseInt(bucket),
            (counts.get(parseInt(bucket)) ?? 0) + entry.count,
          );
        }
      }
    }

    const latestBucket = Math.max(
      result.bucketSpan?.latestBucket ?? 0,
      normalizeBucket(new Date(), result.params.buckets).index,
    );
    const values = Array.from(
      { length: 12 },
      (_, index) => counts.get(latestBucket - 11 + index) ?? 0,
    );
    const maxCount = Math.max(...values);

    return values.map((count, index) => {
      const bucket = latestBucket - 11 + index;
      const timestamp =
        1000 *
        durationSeconds[result.params.buckets.duration] *
        result.params.buckets.multiplier *
        bucket;
      const showLabel = index % 2 === 0 || index === values.length - 1;

      return {
        count,
        height: count ? Math.max(3, Math.round((count / maxCount) * 100)) : 0,
        label: showLabel
          ? formatDate(
              timestamp,
              result.params.buckets.duration === "day" ? "d LLL" : "HH:mm",
            )
          : "",
      };
    });
  }
}
