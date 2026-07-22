import { Component, Inject, inject } from "@angular/core";
import { MAT_DIALOG_DATA, MatDialogRef } from "@angular/material/dialog";
import { Apollo } from "apollo-angular";
import { catchError, EMPTY } from "rxjs";
import { map } from "rxjs/operators";
import { AppModule } from "../../app.module";
import * as generated from "../../graphql/generated";
import { availableQueueNames, statusNames } from "./queue.constants";

@Component({
  selector: "app-queue-purge-jobs-dialog",
  standalone: true,
  imports: [AppModule],
  templateUrl: "./queue-purge-jobs-dialog.component.html",
  styleUrl: "./queue-purge-jobs-dialog.component.scss",
})
export class QueuePurgeJobsDialogComponent {
  private apollo = inject(Apollo);
  protected readonly dialogRef = inject(
    MatDialogRef<QueuePurgeJobsDialogComponent>,
  );

  protected queues?: string[];
  protected statuses?: generated.QueueJobStatus[];
  protected stage: "PENDING" | "REQUESTING" | "DONE" = "PENDING";
  protected error?: Error;

  protected readonly availableQueueNames = availableQueueNames;
  protected readonly statusNames = statusNames;

  @Inject(MAT_DIALOG_DATA) public data: { onPurged?: () => void };

  protected selectAllQueues() {
    this.queues = undefined;
  }

  protected formatQueueName(queue: string) {
    return queue.replaceAll("_", " ");
  }

  protected toggleQueue(queue: string, checked: boolean) {
    if (checked) {
      const queues = [...(this.queues ?? []), queue];
      this.queues =
        queues.length === this.availableQueueNames.length ? undefined : queues;
      return;
    }
    const queues = this.queues?.filter((value) => value !== queue);
    this.queues = queues?.length ? queues : undefined;
  }

  protected selectAllStatuses() {
    this.statuses = undefined;
  }

  protected toggleStatus(status: generated.QueueJobStatus, checked: boolean) {
    if (checked) {
      const statuses = [...(this.statuses ?? []), status];
      this.statuses =
        statuses.length === this.statusNames.length ? undefined : statuses;
      return;
    }
    const statuses = this.statuses?.filter((value) => value !== status);
    this.statuses = statuses?.length ? statuses : undefined;
  }

  protected handlePurgeJobs() {
    if (this.stage !== "PENDING") {
      return;
    }
    this.stage = "REQUESTING";
    this.apollo
      .mutate<
        generated.QueuePurgeJobsMutation,
        generated.QueuePurgeJobsMutationVariables
      >({
        mutation: generated.QueuePurgeJobsDocument,
        variables: {
          input: {
            queues: this.queues,
            statuses: this.statuses,
          },
        },
      })
      .pipe(
        catchError((err: Error) => {
          this.stage = "DONE";
          this.error = err;
          return EMPTY;
        }),
        map(() => {
          this.stage = "DONE";
          this.data?.onPurged?.();
        }),
      )
      .subscribe();
  }
}
