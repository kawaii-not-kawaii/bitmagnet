import { Component, inject } from "@angular/core";
import { toSignal } from "@angular/core/rxjs-interop";
import { MatIconRegistry } from "@angular/material/icon";
import { DomSanitizer } from "@angular/platform-browser";
import { NavigationEnd, Router, RouterOutlet } from "@angular/router";
import { filter, map } from "rxjs";
import { LayoutComponent } from "./layout/layout.component";
import { initializeIcons } from "./app.icons";

@Component({
  selector: "app-root",
  imports: [RouterOutlet, LayoutComponent],
  templateUrl: "./app.component.html",
  styleUrl: "./app.component.scss",
})
export class AppComponent {
  title = "bitmagnet";

  private readonly router = inject(Router);
  protected readonly isAuthRoute = toSignal(
    this.router.events.pipe(
      filter((event): event is NavigationEnd => event instanceof NavigationEnd),
      map((event) => this.isAuthUrl(event.urlAfterRedirects)),
    ),
    { initialValue: this.isAuthUrl(this.router.url) },
  );

  constructor() {
    const iconRegistry = inject(MatIconRegistry);
    const domSanitizer = inject(DomSanitizer);

    initializeIcons(iconRegistry, domSanitizer);
  }

  private isAuthUrl(url: string): boolean {
    return url.startsWith("/login") || url.startsWith("/setup");
  }
}
