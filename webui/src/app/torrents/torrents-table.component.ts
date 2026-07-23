import { Component, EventEmitter, Input, OnInit, Output } from "@angular/core";
import { SelectionModel } from "@angular/cdk/collections";
import { AsyncPipe, LowerCasePipe } from "@angular/common";
import { TranslocoDirective } from "@jsverse/transloco";
import { FilesizePipe } from "../pipes/filesize.pipe";
import { TimeAgoPipe } from "../pipes/time-ago.pipe";
import * as generated from "../graphql/generated";
import { TorrentsSearchDatasource } from "./torrents-search.datasource";
import { contentTypeInfo } from "./content-types";
import { TorrentChipsComponent } from "./torrent-chips.component";
import { TorrentContentComponent } from "./torrent-content.component";
import { TorrentsSearchController } from "./torrents-search.controller";

@Component({
  selector: "app-torrents-table",
  imports: [
    AsyncPipe,
    LowerCasePipe,
    TranslocoDirective,
    FilesizePipe,
    TimeAgoPipe,
    TorrentChipsComponent,
    TorrentContentComponent,
  ],
  templateUrl: "./torrents-table.component.html",
  styleUrl: "./torrents-table.component.scss",
})
export class TorrentsTableComponent implements OnInit {
  contentTypeInfo = contentTypeInfo;

  @Input() dataSource: TorrentsSearchDatasource;
  @Input() controller: TorrentsSearchController;
  @Input() multiSelection: SelectionModel<string>;
  @Input() selectMode = false;
  @Output() updated = new EventEmitter<string>();

  items = Array<generated.TorrentContent>();
  copiedHash?: string;

  ngOnInit() {
    this.dataSource.items$.subscribe((items) => {
      this.items = items;
    });
  }

  isAllSelected() {
    return (
      this.items.length > 0 &&
      this.items.every((item) => this.multiSelection.isSelected(item.infoHash))
    );
  }

  toggleAllRows() {
    if (this.isAllSelected()) {
      this.multiSelection.clear();
    } else {
      this.multiSelection.select(...this.items.map((item) => item.infoHash));
    }
  }

  toggleSelectedTorrent(infoHash: string) {
    this.controller.update((controls) => ({
      ...controls,
      selectedTorrent:
        controls.selectedTorrent?.infoHash === infoHash
          ? undefined
          : { infoHash, tab: controls.selectedTorrent?.tab },
    }));
  }

  // Keyboard toggle for the row itself; Enter on nested controls (buttons,
  // links) bubbles here too, so only act when the row is the actual target.
  rowKeyToggle(event: Event, infoHash: string) {
    if (event.target === event.currentTarget) {
      this.toggleSelectedTorrent(infoHash);
    }
  }

  toggleSelection(event: Event, item: generated.TorrentContent) {
    event.stopPropagation();
    this.multiSelection.toggle(item.infoHash);
  }

  copyMagnet(event: Event, item: generated.TorrentContent) {
    event.stopPropagation();
    void navigator.clipboard?.writeText(item.torrent.magnetUri);
    this.copiedHash = item.infoHash;
    window.setTimeout(() => {
      if (this.copiedHash === item.infoHash) {
        this.copiedHash = undefined;
      }
    }, 1400);
  }

  typeAbbreviation(contentType?: generated.ContentType | null) {
    const labels: Partial<Record<generated.ContentType, string>> = {
      movie: "MOV",
      tv_show: "TV",
      music: "MUS",
      game: "GAM",
      software: "SW",
      ebook: "BK",
      audiobook: "AUD",
      comic: "COM",
      xxx: "XXX",
    };
    return contentType ? labels[contentType] : "?";
  }

  sourceNames(item: generated.TorrentContent) {
    return item.torrent.sources.map((source) => source.name).join(", ") || "—";
  }
}
