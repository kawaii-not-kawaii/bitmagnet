import { waitForAsync, ComponentFixture, TestBed } from "@angular/core/testing";

import { appConfig } from "../app.config";
import { TorrentsSearchComponent } from "./torrents-search.component";

describe("TableComponent", () => {
  let component: TorrentsSearchComponent;
  let fixture: ComponentFixture<TorrentsSearchComponent>;

  beforeEach(waitForAsync(async () => {
    await TestBed.configureTestingModule(appConfig).compileComponents();
  }));

  beforeEach(() => {
    fixture = TestBed.createComponent(TorrentsSearchComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it("should compile", () => {
    expect(component).toBeTruthy();
  });

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
