import { Component, inject } from "@angular/core";
import { RouterLink, RouterLinkActive, RouterOutlet } from "@angular/router";
import { HealthService } from "../health/health.service";

@Component({
  selector: "app-dashboard",
  imports: [RouterLink, RouterLinkActive, RouterOutlet],
  templateUrl: "./dashboard.component.html",
  styleUrl: "./dashboard.component.scss",
})
export class DashboardComponent {
  health = inject(HealthService);
}
