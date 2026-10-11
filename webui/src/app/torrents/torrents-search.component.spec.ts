import {
  waitForAsync,
  ComponentFixture,
  TestBed,
  fakeAsync,
  tick,
} from "@angular/core/testing";
import { Router } from "@angular/router";

import { appConfig } from "../app.config";
import { TorrentsSearchComponent } from "./torrents-search.component";

describe("TableComponent", () => {
  let component: TorrentsSearchComponent;
  let fixture: ComponentFixture<TorrentsSearchComponent>;

  beforeEach(waitForAsync(async () => {
    await TestBed.configureTestingModule(appConfig).compileComponents();
  }));

  beforeEach(() => {
    spyOn(TestBed.inject(Router), "navigate").and.resolveTo(true);
    fixture = TestBed.createComponent(TorrentsSearchComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it("stops pending search control updates when the display is destroyed", fakeAsync(() => {
    fixture.destroy();
    fixture = TestBed.createComponent(TorrentsSearchComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
    const pages: number[] = [];
    component.controller.params$.subscribe(({ input }) =>
      pages.push(input.page ?? 1),
    );
    component.controller.update((controls) => ({ ...controls, page: 2 }));
    fixture.destroy();
    tick(100);
    expect(pages).toEqual([1]);
  }));

  describe("showContentType", () => {
    function withAggregations(values: Array<string | null>, counts = 1): void {
      component.dataSource.result.aggregations.contentType = values.map(
        (value) => ({ value, count: counts, isEstimate: false }),
      ) as never;
    }

    it("offers the unclassified bucket, which arrives with a null value", () => {
      withAggregations([null, "movie"]);
      expect(component.showContentType("null")).toBeTrue();
    });

    it("hides a content type absent from the aggregations", () => {
      withAggregations(["movie"]);
      expect(component.showContentType("music")).toBeFalse();
    });

    it("still hides xxx while safe mode is on", () => {
      withAggregations(["xxx", null]);
      component.preferences.setSafeMode(true);
      expect(component.showContentType("xxx")).toBeFalse();
      // Safe mode must not suppress the unclassified bucket.
      expect(component.showContentType("null")).toBeTrue();
    });
  });
});
