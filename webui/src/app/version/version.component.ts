import { Component, inject, OnInit } from "@angular/core";
import { Apollo } from "apollo-angular";
import { map } from "rxjs";
import { TranslocoDirective } from "@jsverse/transloco";
import * as generated from "../graphql/generated";

const defaultVersionName = "v-unknown";

@Component({
  selector: "app-version",
  templateUrl: "./version.component.html",
  imports: [TranslocoDirective],
})
export class VersionComponent implements OnInit {
  private apollo = inject(Apollo);

  version: string = defaultVersionName;
  versionUnknown = true;

  ngOnInit(): void {
    this.apollo
      .query<generated.VersionQuery, generated.VersionQueryVariables>({
        query: generated.VersionDocument,
      })
      .pipe(map((r) => r.data.version))
      .subscribe({
        next: (version: string) => {
          if (version) {
            this.version = version;
            this.versionUnknown = false;
          } else {
            this.version = defaultVersionName;
            this.versionUnknown = true;
          }
        },
        error: () => {
          this.version = defaultVersionName;
        },
      });
  }
}
