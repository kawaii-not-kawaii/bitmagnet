import { HttpErrorResponse } from "@angular/common/http";
import { Component, inject } from "@angular/core";
import { FormBuilder, ReactiveFormsModule, Validators } from "@angular/forms";
import { ActivatedRoute, Router } from "@angular/router";
import { firstValueFrom } from "rxjs";
import { AuthShellComponent } from "./auth-shell.component";
import { AuthService } from "./auth.service";
import type { LoginRequest } from "./auth.service";

type LoginMode = "password" | "apiKey";

@Component({
  selector: "app-login",
  standalone: true,
  imports: [AuthShellComponent, ReactiveFormsModule],
  templateUrl: "./login.component.html",
  styleUrl: "./login.component.scss",
})
export class LoginComponent {
  private readonly auth = inject(AuthService);
  private readonly formBuilder = inject(FormBuilder);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly required = Validators.required.bind(Validators);

  protected readonly form = this.formBuilder.nonNullable.group({
    username: ["", this.required],
    password: ["", this.required],
    apiKey: ["", this.required],
  });
  protected mode: LoginMode = "password";
  protected busy = false;
  protected error = "";

  protected get canSubmit(): boolean {
    if (this.busy) {
      return false;
    }
    return this.mode === "password"
      ? this.form.controls.username.value.trim().length > 0 &&
          this.form.controls.password.value.length > 0
      : this.form.controls.apiKey.value.trim().length > 0;
  }

  protected toggleMode(): void {
    this.mode = this.mode === "password" ? "apiKey" : "password";
    this.error = "";
  }

  protected async submit(): Promise<void> {
    if (!this.canSubmit) {
      return;
    }

    this.busy = true;
    this.error = "";
    try {
      await firstValueFrom(this.auth.login(this.credentials()));
      await this.router.navigateByUrl(this.returnUrl());
    } catch (error) {
      this.error = this.errorMessage(error);
    } finally {
      this.busy = false;
    }
  }

  private credentials(): LoginRequest {
    const { username, password, apiKey } = this.form.getRawValue();
    return this.mode === "password"
      ? { username: username.trim(), password }
      : { apiKey: apiKey.trim() };
  }

  private returnUrl(): string {
    const returnUrl = this.route.snapshot.queryParamMap.get("returnUrl");
    return returnUrl?.startsWith("/") && !returnUrl.startsWith("//")
      ? returnUrl
      : "/torrents";
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
    return "Sign-in failed. Please try again.";
  }
}
