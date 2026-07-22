import { Component, inject } from "@angular/core";
import { AppModule } from "../app.module";
import { HealthModule } from "../health/health.module";
import { HealthService } from "../health/health.service";

@Component({
  selector: "app-dashboard",
  standalone: true,
  imports: [AppModule, HealthModule],
  templateUrl: "./dashboard.component.html",
  styleUrl: "./dashboard.component.scss",
})
export class DashboardComponent {
  health = inject(HealthService);
}
