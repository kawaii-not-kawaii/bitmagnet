import { Component, inject } from "@angular/core";
import { FormControl } from "@angular/forms";
import { MatDialogRef } from "@angular/material/dialog";
import { AppModule } from "../app.module";
import { AuthService } from "./auth.service";

/**
 * ApiKeyDialogComponent prompts the operator for the GraphQL API key. It is
 * opened by the shell when the server rejects a request with a 401 (missing or
 * wrong key). On save it stores the key and reloads, so every active query
 * re-runs with the new credential rather than leaving stale 401 errors on
 * screen.
 */
@Component({
  selector: "app-api-key-dialog",
  standalone: true,
  imports: [AppModule],
  templateUrl: "./api-key-dialog.component.html",
  styleUrl: "./api-key-dialog.component.scss",
})
export class ApiKeyDialogComponent {
  private readonly auth = inject(AuthService);
  private readonly dialogRef =
    inject<MatDialogRef<ApiKeyDialogComponent>>(MatDialogRef);

  protected readonly apiKey = new FormControl("", { nonNullable: true });

  protected save(): void {
    if (this.apiKey.value.trim().length === 0) {
      return;
    }
    this.auth.setApiKey(this.apiKey.value);
    this.dialogRef.close(true);
    // Reload so all queries re-run with the new key. A blunt but reliable way
    // to recover the whole UI from an auth failure.
    window.location.reload();
  }
}
