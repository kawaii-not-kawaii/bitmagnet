import { Clipboard } from "@angular/cdk/clipboard";
import { ComponentFixture, TestBed } from "@angular/core/testing";

import { appConfig } from "../app.config";
import { TorrentsBulkActionsComponent } from "./torrents-bulk-actions.component";

describe("TorrentsBulkActionsComponent", () => {
  let component: TorrentsBulkActionsComponent;
  let fixture: ComponentFixture<TorrentsBulkActionsComponent>;

  beforeEach(async () => {
    await TestBed.configureTestingModule(appConfig).compileComponents();

    fixture = TestBed.createComponent(TorrentsBulkActionsComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it("should create", () => {
    expect(component).toBeTruthy();
  });

  // navigator.clipboard is undefined over plain HTTP, where the old
  // `navigator.clipboard?.writeText()` silently no-opped while still flashing
  // "copied". The CDK service works via execCommand and reports failure.
  it("copies the selected magnet links through the CDK clipboard", () => {
    const clipboard = TestBed.inject(Clipboard);
    const spy = spyOn(clipboard, "copy").and.returnValue(true);
    component.selectedItems = [
      { torrent: { magnetUri: "magnet:?xt=urn:btih:aaa" } },
      { torrent: { magnetUri: "magnet:?xt=urn:btih:bbb" } },
    ] as never;

    component.copySelected();

    expect(spy).toHaveBeenCalledWith(
      "magnet:?xt=urn:btih:aaa\nmagnet:?xt=urn:btih:bbb",
    );
    expect(component.copied).toBeTrue();
  });

  it("does not report success when the copy fails", () => {
    const clipboard = TestBed.inject(Clipboard);
    spyOn(clipboard, "copy").and.returnValue(false);
    component.selectedItems = [
      { torrent: { magnetUri: "magnet:?xt=urn:btih:aaa" } },
    ] as never;

    component.copySelected();

    expect(component.copied).toBeFalse();
  });
});
