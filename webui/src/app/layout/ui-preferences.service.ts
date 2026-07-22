import { Injectable, inject, signal } from "@angular/core";
import { BrowserStorageService } from "../browser-storage/browser-storage.service";

export type Density = "comfortable" | "compact";
export type RowStyle = "divided" | "zebra" | "cards";

@Injectable({ providedIn: "root" })
export class UiPreferences {
  private storage = inject(BrowserStorageService);

  settingsOpen = signal(false);
  density = signal<Density>(
    this.storage.get("bitmagnet-density") === "compact"
      ? "compact"
      : "comfortable",
  );
  rowStyle = signal<RowStyle>(this.storedRowStyle());
  safeMode = signal(this.storage.get("bitmagnet-safe-mode") !== "false");
  pageSize = signal(this.storedPageSize());

  setDensity(value: Density) {
    this.density.set(value);
    this.storage.set("bitmagnet-density", value);
  }

  setRowStyle(value: RowStyle) {
    this.rowStyle.set(value);
    this.storage.set("bitmagnet-row-style", value);
  }

  setSafeMode(value: boolean) {
    this.safeMode.set(value);
    this.storage.set("bitmagnet-safe-mode", String(value));
  }

  setPageSize(value: number) {
    this.pageSize.set(value);
    this.storage.set("bitmagnet-page-size", String(value));
  }

  private storedRowStyle(): RowStyle {
    const value = this.storage.get("bitmagnet-row-style");
    return value === "zebra" || value === "cards" ? value : "divided";
  }

  private storedPageSize(): number {
    const value = Number(this.storage.get("bitmagnet-page-size"));
    return value === 25 || value === 100 ? value : 50;
  }
}
