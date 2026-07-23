import { Component } from "@angular/core";
import { MatCardModule } from "@angular/material/card";
import { TranslocoDirective } from "@jsverse/transloco";
import { DocumentTitleComponent } from "../layout/document-title.component";

@Component({
  selector: "app-not-found",
  imports: [MatCardModule, TranslocoDirective, DocumentTitleComponent],
  templateUrl: "./not-found.component.html",
  styleUrl: "./not-found.component.scss",
})
export class NotFoundComponent {}
