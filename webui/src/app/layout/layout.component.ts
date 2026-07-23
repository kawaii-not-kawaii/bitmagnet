import {
  Component,
  ElementRef,
  HostListener,
  ViewChild,
  inject,
} from "@angular/core";
import { NavigationEnd, Router } from "@angular/router";
import { filter } from "rxjs";
import { Title } from "@angular/platform-browser";
import { ThemeManager } from "../themes/theme-manager.service";
import { VersionComponent } from "../version/version.component";
import { TranslateManager } from "../i18n/translate-manager.service";
import { HealthModule } from "../health/health.module";
import { HealthService } from "../health/health.service";
import { AppModule } from "../app.module";
import { UiPreferences } from "./ui-preferences.service";

@Component({
  selector: "app-layout",
  templateUrl: "./layout.component.html",
  styleUrl: "./layout.component.scss",
  imports: [AppModule, HealthModule, VersionComponent],
})
export class LayoutComponent {
  themeManager = inject(ThemeManager);
  translateManager = inject(TranslateManager);
  preferences = inject(UiPreferences);
  title = inject(Title);
  router = inject(Router);
  health = inject(HealthService);

  @ViewChild("globalSearch") globalSearch?: ElementRef<HTMLInputElement>;

  searchQuery = "";
  darkThemes = this.themeManager.themes.filter((theme) => theme.dark);
  lightThemes = this.themeManager.themes.filter((theme) => !theme.dark);

  constructor() {
    this.syncSearchQuery();
    this.router.events
      .pipe(filter((event) => event instanceof NavigationEnd))
      .subscribe(() => this.syncSearchQuery());
  }

  search() {
    const query = this.searchQuery.trim();
    void this.router.navigate(["/torrents"], {
      queryParams: { query: query ? encodeURIComponent(query) : undefined },
    });
  }

  setPageSize(limit: number) {
    this.preferences.setPageSize(limit);
    void this.router.navigate([], {
      queryParams: { limit, page: undefined },
      queryParamsHandling: "merge",
    });
  }

  toggleSafeMode() {
    const enabled = !this.preferences.safeMode();
    this.preferences.setSafeMode(enabled);
    if (enabled && this.router.url.includes("content_type=xxx")) {
      void this.router.navigate([], {
        queryParams: { content_type: undefined, page: undefined },
        queryParamsHandling: "merge",
      });
    }
  }

  @HostListener("document:keydown", ["$event"])
  focusSearch(event: KeyboardEvent) {
    const target = event.target as HTMLElement;
    if (
      event.key !== "/" ||
      target.matches("input, textarea, select, [contenteditable='true']")
    ) {
      return;
    }
    event.preventDefault();
    this.globalSearch?.nativeElement.focus();
  }

  private syncSearchQuery() {
    const param = this.router.parseUrl(this.router.url).queryParams[
      "query"
    ] as unknown;
    const query = typeof param === "string" ? param : "";
    try {
      this.searchQuery = decodeURIComponent(query);
    } catch {
      this.searchQuery = query;
    }
  }
}
