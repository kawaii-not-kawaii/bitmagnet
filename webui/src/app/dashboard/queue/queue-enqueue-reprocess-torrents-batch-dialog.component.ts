import { Component, Inject, inject } from "@angular/core";
import { MAT_DIALOG_DATA, MatDialogRef } from "@angular/material/dialog";
import { Apollo } from "apollo-angular";
import { catchError, EMPTY } from "rxjs";
import { AppModule } from "../../app.module";
import { ErrorsService } from "../../errors/errors.service";
import * as generated from "../../graphql/generated";
import { contentTypeList } from "../../torrents/content-types";

type ReprocessContentType = generated.ContentType | "null" | "all";

@Component({
  selector: "app-queue-enqueue-reprocess-torrents-batch-dialog",
  standalone: true,
  imports: [AppModule],
  templateUrl: "./queue-enqueue-reprocess-torrents-batch-dialog.component.html",
  styleUrl: "./queue-enqueue-reprocess-torrents-batch-dialog.component.scss",
})
export class QueueEnqueueReprocessTorrentsBatchDialogComponent {
  private apollo = inject(Apollo);
  private errorsService = inject(ErrorsService);
  protected readonly dialogRef = inject(
    MatDialogRef<QueueEnqueueReprocessTorrentsBatchDialogComponent>,
  );

  protected readonly allContentTypes = contentTypeList;
  protected stage: "PENDING" | "REQUESTING" | "DONE" = "PENDING";

  protected purge = true;
  protected apisDisabled = true;
  protected localSearchDisabled = true;
  protected classifierRematch = false;
  protected contentTypes: ReprocessContentType[] = ["all"];
  protected orphans = false;

  @Inject(MAT_DIALOG_DATA) public data: { onEnqueued?: () => void };

  protected selectAllContentTypes() {
    this.contentTypes = ["all"];
    this.orphans = false;
  }

  protected toggleContentType(
    contentType: Exclude<ReprocessContentType, "all">,
    checked: boolean,
  ) {
    const current = this.contentTypes.includes("all") ? [] : this.contentTypes;
    const next = checked
      ? [...current, contentType]
      : current.filter((value) => value !== contentType);
    this.contentTypes =
      next.length === this.allContentTypes.length || !next.length
        ? ["all"]
        : next;
    this.orphans = false;
  }

  protected toggleOrphans(checked: boolean) {
    this.orphans = checked;
    if (checked) {
      this.contentTypes = ["all"];
    }
  }

  protected handleEnqueue() {
    if (this.stage !== "PENDING") {
      return;
    }
    this.stage = "REQUESTING";
    this.apollo
      .mutate<
        generated.QueueEnqueueReprocessTorrentsBatchMutation,
        generated.QueueEnqueueReprocessTorrentsBatchMutationVariables
      >({
        mutation: generated.QueueEnqueueReprocessTorrentsBatchDocument,
        variables: {
          input: {
            purge: this.purge,
            apisDisabled: this.apisDisabled,
            localSearchDisabled: this.localSearchDisabled,
            classifierRematch: this.classifierRematch,
            contentTypes: this.contentTypes.includes("all")
              ? undefined
              : this.contentTypes.map((contentType) =>
                  contentType === "null"
                    ? null
                    : (contentType as generated.ContentType),
                ),
            orphans: this.orphans || undefined,
          },
        },
      })
      .pipe(
        catchError((error: Error) => {
          this.errorsService.addError(error.message);
          this.dialogRef.close();
          return EMPTY;
        }),
      )
      .subscribe(() => {
        this.stage = "DONE";
        this.data?.onEnqueued?.();
      });
  }
}
