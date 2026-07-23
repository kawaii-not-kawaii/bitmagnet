import { Component } from "@angular/core";
import { AppModule } from "../app.module";
import { DocumentTitleComponent } from "../layout/document-title.component";

@Component({
  selector: "app-not-found",
  imports: [AppModule, DocumentTitleComponent],
  templateUrl: "./not-found.component.html",
  styleUrl: "./not-found.component.scss",
})
export class NotFoundComponent {}
