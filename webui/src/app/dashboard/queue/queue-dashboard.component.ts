import { Component, ViewChild } from "@angular/core";
import { RouterOutlet } from "@angular/router";
import { AppModule } from "../../app.module";

type Refreshable = {
  refresh: () => void;
};

@Component({
  selector: "app-queue-dashboard",
  imports: [AppModule],
  templateUrl: "./queue-dashboard.component.html",
  styleUrl: "./queue-dashboard.component.scss",
})
export class QueueDashboardComponent {
  @ViewChild(RouterOutlet) private outlet?: RouterOutlet;

  protected refresh() {
    const component: unknown = this.outlet?.component;
    if (this.isRefreshable(component)) {
      component.refresh();
    }
  }

  private isRefreshable(component: unknown): component is Refreshable {
    return (
      typeof component === "object" &&
      component !== null &&
      "refresh" in component &&
      typeof component.refresh === "function"
    );
  }
}
