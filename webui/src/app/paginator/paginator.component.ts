import {
  Component,
  EventEmitter,
  Input,
  numberAttribute,
  Output,
} from "@angular/core";
import { AppModule } from "../app.module";
import { IntEstimatePipe } from "../pipes/int-estimate.pipe";
import type { PageEvent } from "./paginator.types";

@Component({
  selector: "app-paginator",
  templateUrl: "./paginator.component.html",
  standalone: true,
  styleUrls: ["./paginator.component.scss"],
  imports: [AppModule, IntEstimatePipe],
})
export class PaginatorComponent {
  @Input({ transform: numberAttribute }) page = 1;
  @Input({ transform: numberAttribute }) pageSize = 10;
  @Input() pageSizes: number[] = [10, 20, 50, 100];
  @Input({ transform: numberAttribute }) pageLength = 0;
  @Input() totalLength: number | null = null;
  @Input() totalIsEstimate = false;
  @Input() hasNextPage: boolean | null | undefined = null;
  @Input() itemName: string | null = null;

  @Output() paging = new EventEmitter<PageEvent>();

  get firstItemIndex() {
    return (this.page - 1) * this.pageSize + 1;
  }

  get lastItemIndex() {
    return (this.page - 1) * this.pageSize + this.pageLength;
  }

  get hasTotalLength() {
    return typeof this.totalLength === "number";
  }

  get hasPreviousPage() {
    return this.page > 1;
  }

  get pageCount(): number | null {
    if (typeof this.totalLength !== "number") {
      return null;
    }
    return Math.ceil(this.totalLength / this.pageSize);
  }

  get visiblePages(): (number | "ellipsis")[] {
    const count = this.pageCount;
    if (!count) {
      return [this.page];
    }
    if (count <= 7) {
      return Array.from({ length: count }, (_, index) => index + 1);
    }

    let start = Math.max(2, this.page - 2);
    let end = Math.min(count - 1, this.page + 2);
    if (start === 2) {
      end = Math.min(count - 1, 6);
    }
    if (end === count - 1) {
      start = Math.max(2, count - 5);
    }

    return [
      1,
      ...(start > 2 ? (["ellipsis"] as const) : []),
      ...Array.from({ length: end - start + 1 }, (_, index) => start + index),
      ...(end < count - 1 ? (["ellipsis"] as const) : []),
      count,
    ];
  }

  goToPage(page: number) {
    if (page === this.page) {
      return;
    }
    this.page = page;
    this.emitChange();
  }

  get actuallyHasNextPage() {
    if (typeof this.hasNextPage === "boolean") {
      return this.hasNextPage;
    }
    if (typeof this.totalLength !== "number") {
      return false;
    }
    return this.page * this.pageSize < this.totalLength;
  }

  emitChange() {
    this.paging.emit({
      page: this.page,
      pageSize: this.pageSize,
    });
  }
}
