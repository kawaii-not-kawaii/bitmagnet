import { ComponentFixture, TestBed } from "@angular/core/testing";

import { appConfig } from "../app.config";
import { PaginatorComponent } from "./paginator.component";

describe("PaginatorComponent", () => {
  let component: PaginatorComponent;
  let fixture: ComponentFixture<PaginatorComponent>;

  beforeEach(() => {
    TestBed.configureTestingModule(appConfig);
    fixture = TestBed.createComponent(PaginatorComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it("should create", () => {
    expect(component).toBeTruthy();
  });

  it("windows numbered pages and emits direct navigation", () => {
    component.page = 6;
    component.pageSize = 50;
    component.pageLength = 50;
    component.totalLength = 1000;

    expect(component.visiblePages).toEqual([
      1,
      "ellipsis",
      4,
      5,
      6,
      7,
      8,
      "ellipsis",
      20,
    ]);

    let emitted: { page: number; pageSize: number } | undefined;
    component.paging.subscribe((event) => (emitted = event));
    component.goToPage(7);

    expect(emitted).toEqual({ page: 7, pageSize: 50 });
  });
});
