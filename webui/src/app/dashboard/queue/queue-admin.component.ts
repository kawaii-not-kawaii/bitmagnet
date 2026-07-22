import { Component, inject } from "@angular/core";
import { MatDialog } from "@angular/material/dialog";
import { AppModule } from "../../app.module";
import { DocumentTitleComponent } from "../../layout/document-title.component";
import { QueueEnqueueReprocessTorrentsBatchDialogComponent } from "./queue-enqueue-reprocess-torrents-batch-dialog.component";
import { QueuePurgeJobsDialogComponent } from "./queue-purge-jobs-dialog.component";

@Component({
  selector: "app-queue-admin",
  standalone: true,
  imports: [AppModule, DocumentTitleComponent],
  templateUrl: "./queue-admin.component.html",
  styleUrl: "./queue-admin.component.scss",
})
export class QueueAdminComponent {
  private dialog = inject(MatDialog);

  protected openPurgeDialog() {
    this.dialog.open(QueuePurgeJobsDialogComponent, {
      panelClass: "bm-dialog",
      backdropClass: "bm-dialog-backdrop",
      width: "480px",
      maxWidth: "calc(100vw - 24px)",
    });
  }

  protected openEnqueueReprocessTorrentsBatch() {
    this.dialog.open(QueueEnqueueReprocessTorrentsBatchDialogComponent, {
      panelClass: "bm-dialog",
      backdropClass: "bm-dialog-backdrop",
      width: "560px",
      maxWidth: "calc(100vw - 24px)",
    });
  }
}
