import { Injectable, InjectionToken, inject } from "@angular/core";

export const BROWSER_STORAGE = new InjectionToken<Storage>("Browser Storage", {
  providedIn: "root",
  factory: () => localStorage,
});

@Injectable({ providedIn: "root" })
export class BrowserStorageService {
  storage = inject<Storage>(BROWSER_STORAGE);

  get(key: string) {
    return this.storage.getItem(key);
  }

  set(key: string, value: string) {
    this.storage.setItem(key, value);
  }

  remove(key: string) {
    this.storage.removeItem(key);
  }

  clear() {
    this.storage.clear();
  }
}
