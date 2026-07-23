import { Component, EventEmitter, inject, Input, Output } from "@angular/core";
import { catchError, EMPTY, tap } from "rxjs";
import { NgOptimizedImage } from "@angular/common";
import * as generated from "../graphql/generated";
import { GraphQLService } from "../graphql/graphql.service";
import { ErrorsService } from "../errors/errors.service";
import { AppModule } from "../app.module";
import { TorrentFilesTableComponent } from "./torrent-files-table.component";
import { TorrentEditTagsComponent } from "./torrent-edit-tags.component";
import { TorrentTab, TorrentTabSelection } from "./torrents-search.controller";
import { TorrentReprocessComponent } from "./torrent-reprocess.component";

@Component({
  selector: "app-torrent-content",
  templateUrl: "./torrent-content.component.html",
  styleUrl: "./torrent-content.component.scss",
  imports: [
    AppModule,
    NgOptimizedImage,
    TorrentEditTagsComponent,
    TorrentFilesTableComponent,
    TorrentReprocessComponent,
  ],
})
export class TorrentContentComponent {
  @Input() torrentContent: generated.TorrentContent;
  @Input() heading = true;
  @Input() size = true;
  @Input() peers = true;
  @Input() published = true;
  @Input() selectedTab: TorrentTabSelection = undefined;

  @Output() updated = new EventEmitter<null>();
  @Output() tabSelected = new EventEmitter<TorrentTabSelection>();

  graphQL = inject(GraphQLService);
  errors = inject(ErrorsService);
  copied = false;

  get activeTab(): TorrentTab {
    return this.selectedTab ?? "files";
  }

  selectTab(tab: TorrentTab) {
    this.selectedTab = tab;
    this.tabSelected.emit(tab);
  }

  copyMagnet(event: Event) {
    event.stopPropagation();
    void navigator.clipboard?.writeText(this.torrentContent.torrent.magnetUri);
    this.copied = true;
    window.setTimeout(() => {
      this.copied = false;
    }, 1400);
  }

  delete() {
    this.graphQL
      .torrentDelete({ infoHashes: [this.torrentContent.infoHash] })
      .pipe(
        catchError((error: Error) => {
          this.errors.addError(`Error deleting torrent: ${error.message}`);
          return EMPTY;
        }),
        tap(() => this.updated.emit(null)),
      )
      .subscribe();
  }

  getAttribute(key: string, source?: string): string | undefined {
    return this.torrentContent.content?.attributes?.find(
      (attribute) =>
        attribute.key === key &&
        (source === undefined || attribute.source === source),
    )?.value;
  }

  getCollections(type: string): string[] | undefined {
    const collections = this.torrentContent.content?.collections
      ?.filter((collection) => collection.type === type)
      .map((collection) => collection.name);
    return collections?.length ? collections.sort() : undefined;
  }

  filesCount(): number | undefined {
    if (this.torrentContent.torrent.filesStatus === "single") {
      return 1;
    }
    return this.torrentContent.torrent.filesCount ?? undefined;
  }
}
