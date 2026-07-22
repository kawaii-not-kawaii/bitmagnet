import { Component, effect, inject } from "@angular/core";
import { RouterOutlet } from "@angular/router";
import { MatIconRegistry } from "@angular/material/icon";
import { MatDialog, MatDialogRef } from "@angular/material/dialog";
import { DomSanitizer } from "@angular/platform-browser";
import { LayoutComponent } from "./layout/layout.component";
import { initializeIcons } from "./app.icons";
import { AuthService } from "./auth/auth.service";
import { ApiKeyDialogComponent } from "./auth/api-key-dialog.component";

@Component({
  selector: "app-root",
  standalone: true,
  imports: [RouterOutlet, LayoutComponent],
  templateUrl: "./app.component.html",
  styleUrl: "./app.component.scss",
})
export class AppComponent {
  title = "bitmagnet";

  private readonly auth = inject(AuthService);
  private readonly dialog = inject(MatDialog);
  private authDialogRef?: MatDialogRef<ApiKeyDialogComponent>;

  constructor(iconRegistry: MatIconRegistry, domSanitizer: DomSanitizer) {
    initializeIcons(iconRegistry, domSanitizer);

    // Open the API-key prompt whenever the server signals authentication is
    // required, and only one at a time. When auth is disabled server-side no
    // 401 ever arrives, so this never fires and the UI is unaffected.
    effect(() => {
      if (this.auth.authRequired() && !this.authDialogRef) {
        this.authDialogRef = this.dialog.open(ApiKeyDialogComponent, {
          disableClose: true,
          width: "32rem",
        });
        this.authDialogRef.afterClosed().subscribe(() => {
          this.authDialogRef = undefined;
        });
      }
    });
  }
}
