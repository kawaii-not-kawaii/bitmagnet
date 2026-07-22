import { Component, Input, OnInit } from "@angular/core";
import { TimeAgoPipe } from "../../pipes/time-ago.pipe";
import * as generated from "../../graphql/generated";
import { AppModule } from "../../app.module";
import { QueueJobsDatasource } from "./queue-jobs.datasource";

@Component({
  selector: "app-queue-jobs-table",
  standalone: true,
  imports: [AppModule, TimeAgoPipe],
  templateUrl: "./queue-jobs-table.component.html",
  styleUrl: "./queue-jobs-table.component.scss",
})
export class QueueJobsTableComponent implements OnInit {
  @Input() dataSource: QueueJobsDatasource;

  protected expandedId: string | null = null;
  protected items = Array<generated.QueueJob>();

  ngOnInit() {
    this.dataSource.items$.subscribe((items) => {
      this.items = items;
      if (this.expandedId && !items.some(({ id }) => id === this.expandedId)) {
        this.expandedId = null;
      }
    });
  }

  protected toggleQueueJobId(id: string) {
    this.expandedId = this.expandedId === id ? null : id;
  }

  protected beautifyPayload(payload: string): string {
    try {
      return JSON.stringify(JSON.parse(payload), null, 2);
    } catch {
      return payload;
    }
  }
}
