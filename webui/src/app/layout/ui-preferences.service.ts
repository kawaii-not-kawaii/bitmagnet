import { Injectable, inject, signal } from "@angular/core";
import { BrowserStorageService } from "../browser-storage/browser-storage.service";

export type Density = "comfortable" | "compact";
export type RowStyle = "divided" | "zebra" | "cards";
export type HealthStyle = "bars" | "number" | "dot";

// Seeder counts separating the swarm-health tiers. The design handoff flags these
// as placeholders — against real DHT distributions most results land in the
// middle tier, and `seedHealthy` is expected to drop toward ~200 — so they are
// tunable preferences rather than constants.
export const DEFAULT_SEED_HEALTHY = 500;
export const DEFAULT_SEED_FAIR = 50;

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
  healthStyle = signal<HealthStyle>(this.storedHealthStyle());
  seedHealthy = signal(
    this.storedThreshold("bitmagnet-seed-healthy", DEFAULT_SEED_HEALTHY),
  );
  seedFair = signal(
    this.storedThreshold("bitmagnet-seed-fair", DEFAULT_SEED_FAIR),
  );

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

  setHealthStyle(value: HealthStyle) {
    this.healthStyle.set(value);
    this.storage.set("bitmagnet-health-style", value);
  }

  setSeedHealthy(value: number) {
    this.seedHealthy.set(value);
    this.storage.set("bitmagnet-seed-healthy", String(value));
  }

  setSeedFair(value: number) {
    this.seedFair.set(value);
    this.storage.set("bitmagnet-seed-fair", String(value));
  }

  private storedHealthStyle(): HealthStyle {
    const value = this.storage.get("bitmagnet-health-style");
    return value === "number" || value === "dot" ? value : "bars";
  }

  // A stored threshold that is absent, non-numeric, or not a positive integer
  // falls back to the default rather than breaking every row.
  private storedThreshold(key: string, fallback: number): number {
    const value = Number(this.storage.get(key));
    return Number.isInteger(value) && value > 0 ? value : fallback;
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
