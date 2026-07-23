import { Component, ViewChild } from "@angular/core";
import { RouterLink, RouterLinkActive, RouterOutlet } from "@angular/router";

type Refreshable = {
  refresh: () => void;
};

@Component({
  selector: "app-queue-dashboard",
  imports: [RouterLink, RouterLinkActive, RouterOutlet],
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
