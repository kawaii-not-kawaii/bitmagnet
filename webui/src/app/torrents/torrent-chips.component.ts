import { Component, Input } from "@angular/core";
import { TranslocoDirective } from "@jsverse/transloco";
import * as generated from "../graphql/generated";

@Component({
  selector: "app-torrent-chips",
  imports: [TranslocoDirective],
  templateUrl: "./torrent-chips.component.html",
  styleUrl: "./torrent-chips.component.scss",
})
export class TorrentChipsComponent {
  @Input() torrentContent: generated.TorrentContent;
}
