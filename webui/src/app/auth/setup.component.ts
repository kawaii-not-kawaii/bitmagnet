import { HttpErrorResponse } from "@angular/common/http";
import { Component, inject } from "@angular/core";
import { FormBuilder, ReactiveFormsModule, Validators } from "@angular/forms";
import { Router } from "@angular/router";
import { firstValueFrom } from "rxjs";
import { AuthService } from "./auth.service";
import { AuthShellComponent } from "./auth-shell.component";

@Component({
  selector: "app-setup",
  imports: [AuthShellComponent, ReactiveFormsModule],
  templateUrl: "./setup.component.html",
  styleUrl: "./setup.component.scss",
})
export class SetupComponent {
  private readonly auth = inject(AuthService);
  private readonly formBuilder = inject(FormBuilder);
  private readonly router = inject(Router);
  private readonly required = Validators.required.bind(Validators);

  protected readonly form = this.formBuilder.nonNullable.group({
    username: ["", this.required],
    password: ["", [this.required, Validators.minLength(8)]],
    confirm: ["", this.required],
  });
  protected busy = false;
  protected error = "";

  protected get passwordsMatch(): boolean {
    const { password, confirm } = this.form.getRawValue();
    return confirm.length > 0 && password === confirm;
  }

  protected get canSubmit(): boolean {
    return (
      !this.busy &&
      this.form.valid &&
      this.form.controls.username.value.trim().length > 0 &&
      this.passwordsMatch
    );
  }

  protected get strength(): { level: number; label: string } {
    const password = this.form.controls.password.value;
    if (password.length === 0) {
      return { level: 0, label: "" };
    }
    const variety = [/[a-z]/, /[A-Z]/, /\d/, /[^A-Za-z0-9]/].filter((pattern) =>
      pattern.test(password),
    ).length;
    const score =
      Number(password.length >= 8) +
      Number(password.length >= 12) +
      Number(variety >= 2) +
      Number(variety >= 3);
    const level = Math.max(1, Math.min(4, score));
    return {
      level,
      label: ["", "weak", "fair", "good", "strong"][level],
    };
  }

  protected async submit(): Promise<void> {
    if (!this.canSubmit) {
      this.form.markAllAsTouched();
      return;
    }

    this.busy = true;
    this.error = "";
    const { username, password } = this.form.getRawValue();
    try {
      await firstValueFrom(this.auth.setup(username.trim(), password));
      await this.router.navigateByUrl("/torrents");
    } catch (error) {
      this.error = this.errorMessage(error);
    } finally {
      this.busy = false;
    }
  }

  private errorMessage(error: unknown): string {
    if (error instanceof HttpErrorResponse) {
      const body: unknown = error.error;
      if (
        body !== null &&
        typeof body === "object" &&
        "error" in body &&
        typeof body.error === "string"
      ) {
        return body.error;
      }
    }
    return "Account setup failed. Please try again.";
  }
}
