import { HttpClient } from "@angular/common/http";
import { Injectable, inject, signal } from "@angular/core";
import { Router } from "@angular/router";
import { Observable, firstValueFrom } from "rxjs";
import { BrowserStorageService } from "../browser-storage/browser-storage.service";

const LEGACY_API_KEY_STORAGE_KEY = "bitmagnet.apiKey";

export interface AuthState {
  authDisabled: boolean;
  needsSetup: boolean;
  trustedBypass: boolean;
}

export type LoginRequest =
  | { username: string; password: string }
  | { apiKey: string };

export interface AuthResponse {
  ok: boolean;
  authDisabled?: boolean;
}

@Injectable({ providedIn: "root" })
export class AuthService {
  private readonly http = inject(HttpClient);
  private readonly router = inject(Router);
  private readonly storage = inject(BrowserStorageService);
  private readonly currentState = signal<AuthState | null>(null);

  readonly state = this.currentState.asReadonly();

  loadState(): Observable<AuthState> {
    return this.http.get<AuthState>("/auth/state", { withCredentials: true });
  }

  setup(username: string, password: string): Observable<AuthResponse> {
    return this.http.post<AuthResponse>(
      "/auth/setup",
      { username, password },
      { withCredentials: true },
    );
  }

  login(credentials: LoginRequest): Observable<AuthResponse> {
    return this.http.post<AuthResponse>("/auth/login", credentials, {
      withCredentials: true,
    });
  }

  logout(): Observable<void> {
    return this.http.post<void>("/auth/logout", null, {
      withCredentials: true,
    });
  }

  async bootstrap(): Promise<void> {
    const legacyKey = this.storage.get(LEGACY_API_KEY_STORAGE_KEY);
    let legacyLogin: "none" | "succeeded" | "failed" = "none";

    if (legacyKey !== null) {
      try {
        await firstValueFrom(this.login({ apiKey: legacyKey }));
        legacyLogin = "succeeded";
      } catch {
        legacyLogin = "failed";
      } finally {
        this.storage.remove(LEGACY_API_KEY_STORAGE_KEY);
      }
    }

    let state: AuthState;
    try {
      state = await firstValueFrom(this.loadState());
    } catch {
      return;
    }
    this.currentState.set(state);

    if (state.needsSetup) {
      await this.router.navigate(["/setup"]);
      return;
    }
    if (legacyLogin === "failed") {
      await this.routeToLogin();
      return;
    }
    if (
      this.isAuthRoute(this.router.url) &&
      (legacyLogin === "succeeded" ||
        state.authDisabled ||
        state.trustedBypass ||
        this.router.url.startsWith("/setup"))
    ) {
      await this.router.navigateByUrl("/torrents");
    }
  }

  notifyAuthRequired(): void {
    void this.routeToLogin();
  }

  private routeToLogin(): Promise<boolean> {
    if (this.isAuthRoute(this.router.url)) {
      return Promise.resolve(false);
    }
    return this.router.navigate(["/login"], {
      queryParams: { returnUrl: this.router.url },
    });
  }

  private isAuthRoute(url: string): boolean {
    return url.startsWith("/login") || url.startsWith("/setup");
  }
}
