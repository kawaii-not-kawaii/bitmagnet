import { Component, inject } from "@angular/core";
import { Apollo } from "apollo-angular";
import { catchError, map, of } from "rxjs";
import { AppModule } from "../app.module";
import { ErrorsService } from "../errors/errors.service";
import * as generated from "../graphql/generated";
import { GraphQLModule } from "../graphql/graphql.module";
import { HealthModule } from "../health/health.module";
import { HealthService } from "../health/health.service";

@Component({
  selector: "app-dashboard-home",
  standalone: true,
  imports: [AppModule, GraphQLModule, HealthModule],
  templateUrl: "./dashboard-home.component.html",
  styleUrl: "./dashboard-home.component.scss",
})
export class DashboardHomeComponent {
  health = inject(HealthService);
  private errors = inject(ErrorsService);
  private numberFormat = new Intl.NumberFormat();

  dashboard$ = inject(Apollo)
    .watchQuery<generated.DashboardDataQuery>({
      query: generated.DashboardDataDocument,
      fetchPolicy: "network-only",
      pollInterval: 30000,
    })
    .valueChanges.pipe(
      map((result) => result.data.dashboard),
      catchError((error: Error) => {
        this.errors.addError(error.message);
        return of(null);
      }),
    );

  format(value: number) {
    return this.numberFormat.format(value);
  }

  indexedTrend(current: number, previous: number) {
    if (previous === 0) return current === 0 ? "steady" : "new activity";
    const change = Math.round(((current - previous) / previous) * 100);
    return `${change >= 0 ? "↑" : "↓"} ${Math.abs(change)}% vs prev`;
  }

  queueWidth(value: number, total: number) {
    return `${total > 0 ? (100 * value) / total : 0}%`;
  }
}
