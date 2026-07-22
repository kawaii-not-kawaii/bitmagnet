import { DecimalPipe } from "@angular/common";
import { Component, Input } from "@angular/core";

export type BarChartSeries = {
  label: string;
  tone: "primary" | "secondary";
};

export type BarChartBucket = {
  label: string;
  values: readonly number[];
};

type RenderBucket = {
  label: string;
  bars: { value: number; height: number }[];
};

@Component({
  selector: "app-bar-chart",
  standalone: true,
  imports: [DecimalPipe],
  templateUrl: "./bar-chart.component.html",
  styleUrl: "./bar-chart.component.scss",
})
export class BarChartComponent {
  @Input() series: readonly BarChartSeries[] = [];
  @Input() unit = "";
  @Input() ariaLabel = "Bar chart";

  protected renderBuckets: RenderBucket[] = [];

  @Input() set buckets(buckets: readonly BarChartBucket[]) {
    const values = buckets.flatMap((bucket) => bucket.values.slice(0, 2));
    const max = Math.max(...values, 0);
    this.renderBuckets = buckets.map((bucket) => ({
      label: bucket.label,
      bars: bucket.values.slice(0, 2).map((value) => ({
        value,
        height: max ? (value / max) * 100 : 0,
      })),
    }));
  }
}
