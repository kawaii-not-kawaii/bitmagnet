import { Component, inject, Input, OnInit } from "@angular/core";
import { BaseChartDirective } from "ng2-charts";
import {
  ChartConfiguration,
  ChartType,
  ChartEvent,
  LegendItem,
  LegendElement,
} from "chart.js";
import { Observable } from "rxjs";
import { TranslocoService } from "@jsverse/transloco";
import { ThemeInfoService } from "../themes/theme-info.service";
import { ChartAdapter } from "./types";

@Component({
  selector: "app-chart",
  standalone: true,
  imports: [BaseChartDirective],
  templateUrl: "./chart.component.html",
  styleUrl: "./chart.component.scss",
})
export class ChartComponent<Data = unknown, Type extends ChartType = ChartType>
  implements OnInit
{
  private themeInfo = inject(ThemeInfoService);
  private transloco = inject(TranslocoService);
  private hiddenDatasets = new Map<string, boolean>();

  @Input() title: string;
  @Input() $data: Observable<Data> = new Observable();
  @Input() adapter: ChartAdapter<Data, Type>;
  @Input() width = 500;
  @Input() height = 500;

  chartConfig: ChartConfiguration;

  private data: Data;

  @Input() legend = true;

  ngOnInit() {
    this.updateChart();
    this.$data.subscribe((data) => {
      this.data = data;
      this.updateChart();
    });
    this.themeInfo.info$.subscribe(() => {
      this.updateChart();
    });
    this.transloco.langChanges$.subscribe(() => {
      this.updateChart();
    });
  }

  protected toggleLegend() {
    this.legend = !this.legend;
    this.updateChart();
  }

  private legendOnClick(
    e: ChartEvent,
    item: LegendItem,
    ctx: LegendElement<"line">,
  ) {
    const meta = ctx.chart.getDatasetMeta(item.datasetIndex!);
    meta.hidden = !meta.hidden;
    this.hiddenDatasets.set(meta.label, meta.hidden);
    ctx.chart.update();
  }

  private updateChart() {
    this.chartConfig = this.adapter.create(this.data, {
      legend: this.legend,
      hiddenDatasets: this.hiddenDatasets,
      legendOnClick: this.legendOnClick.bind(this),
    }) as ChartConfiguration;
  }
}
