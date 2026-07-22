import { Injectable, inject, signal } from "@angular/core";
import { BrowserStorageService } from "../browser-storage/browser-storage.service";

// Storage key for the persisted API key. Namespaced to avoid clashing with
// other localStorage entries.
const API_KEY_STORAGE_KEY = "bitmagnet.apiKey";

// Header the backend expects (canonical). The server also accepts
// Authorization: Bearer, but the web UI uses the X-Api-Key form to match the
// *arr ecosystem convention and to avoid implying OAuth2 semantics.
export const API_KEY_HEADER = "X-Api-Key";

/**
 * AuthService holds the API key the web UI presents on every GraphQL request,
 * and tracks whether the server has rejected the current key (or the lack of
 * one) with a 401.
 *
 * The key is stored in localStorage so it survives reloads. `authRequired` is a
 * signal the shell watches to prompt for a key; it is set by the Apollo error
 * link on a 401 and cleared once a key is (re)entered.
 */
@Injectable({ providedIn: "root" })
export class AuthService {
  private readonly storage = inject(BrowserStorageService);

  // Whether the server has signalled that authentication is required and the
  // current credential is missing or rejected. Read by the shell to show the
  // key prompt.
  readonly authRequired = signal(false);

  getApiKey(): string | null {
    return this.storage.get(API_KEY_STORAGE_KEY);
  }

  setApiKey(key: string): void {
    const trimmed = key.trim();
    if (trimmed.length === 0) {
      this.clearApiKey();
      return;
    }
    this.storage.set(API_KEY_STORAGE_KEY, trimmed);
    // A freshly entered key clears the prompt; if it is still wrong the next
    // request's 401 will set it again.
    this.authRequired.set(false);
  }

  clearApiKey(): void {
    this.storage.remove(API_KEY_STORAGE_KEY);
  }

  /**
   * Called by the Apollo error link when a request comes back 401. Records that
   * a credential is needed so the shell can prompt. Deliberately does NOT clear
   * the stored key: the operator may have simply mistyped, and clearing would
   * lose a possibly-correct key on a transient failure.
   */
  notifyAuthRequired(): void {
    this.authRequired.set(true);
  }
}
