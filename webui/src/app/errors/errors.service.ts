import { Injectable, inject } from "@angular/core";
import { MatSnackBar } from "@angular/material/snack-bar";

@Injectable({ providedIn: "root" })
export class ErrorsService {
  private snackBar = inject(MatSnackBar);

  public readonly expiry = 1000 * 10;

  addError(message: string, expiry = this.expiry) {
    this.snackBar.open(message, "Dismiss", {
      duration: expiry,
      panelClass: ["snack-bar-error"],
    });
  }
}
