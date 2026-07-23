import { Component, inject } from "@angular/core";
import { FormsModule } from "@angular/forms";
import { Apollo } from "apollo-angular";
import { catchError, EMPTY } from "rxjs";
import { AppModule } from "../../app.module";
import { ErrorsService } from "../../errors/errors.service";
import * as generated from "../../graphql/generated";
import { DocumentTitleComponent } from "../../layout/document-title.component";
import { availableQueueNames } from "./queue.constants";

@Component({
  selector: "app-queue-admin",
  imports: [AppModule, DocumentTitleComponent, FormsModule],
  templateUrl: "./queue-admin.component.html",
  styleUrl: "./queue-admin.component.scss",
})
export class QueueAdminComponent {
  private apollo = inject(Apollo);
  private errorsService = inject(ErrorsService);

  protected readonly availableQueueNames = availableQueueNames;
  protected orphans = false;
  protected purgeQueue = availableQueueNames[0];
  protected enqueueStage: "PENDING" | "REQUESTING" | "DONE" = "PENDING";
  protected purgeStage: "PENDING" | "CONFIRMING" | "REQUESTING" | "DONE" =
    "PENDING";

  protected handleEnqueue() {
    if (this.enqueueStage !== "PENDING") {
      return;
    }
    this.enqueueStage = "REQUESTING";
    this.apollo
      .mutate<
        generated.QueueEnqueueReprocessTorrentsBatchMutation,
        generated.QueueEnqueueReprocessTorrentsBatchMutationVariables
      >({
        mutation: generated.QueueEnqueueReprocessTorrentsBatchDocument,
        variables: {
          input: {
            purge: true,
            apisDisabled: true,
            localSearchDisabled: true,
            classifierRematch: false,
            orphans: this.orphans || undefined,
          },
        },
      })
      .pipe(
        catchError((error: Error) => {
          this.enqueueStage = "PENDING";
          this.errorsService.addError(error.message);
          return EMPTY;
        }),
      )
      .subscribe(() => (this.enqueueStage = "DONE"));
  }

  protected handlePurge() {
    if (this.purgeStage !== "CONFIRMING") {
      return;
    }
    this.purgeStage = "REQUESTING";
    this.apollo
      .mutate<
        generated.QueuePurgeJobsMutation,
        generated.QueuePurgeJobsMutationVariables
      >({
        mutation: generated.QueuePurgeJobsDocument,
        variables: {
          input: {
            queues: this.purgeQueue ? [this.purgeQueue] : undefined,
            statuses: ["processed", "failed"],
          },
        },
      })
      .pipe(
        catchError((error: Error) => {
          this.purgeStage = "PENDING";
          this.errorsService.addError(error.message);
          return EMPTY;
        }),
      )
      .subscribe(() => (this.purgeStage = "DONE"));
  }
}
