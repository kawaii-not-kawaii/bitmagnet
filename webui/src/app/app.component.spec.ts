import { TestBed } from "@angular/core/testing";
import { TranslocoModule } from "@jsverse/transloco";
import { AppComponent } from "./app.component";
import { AuthService } from "./auth/auth.service";
import { appConfig } from "./app.config";

describe("AppComponent", () => {
  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [AppComponent, TranslocoModule],
      providers: [
        ...appConfig.providers,
        {
          provide: AuthService,
          useValue: { bootstrap: () => Promise.resolve() },
        },
      ],
    }).compileComponents();
  });

  it("should create the app", () => {
    const fixture = TestBed.createComponent(AppComponent);
    const app = fixture.componentInstance;
    expect(app).toBeTruthy();
  });

  it(`should have the 'bitmagnet' title`, () => {
    const fixture = TestBed.createComponent(AppComponent);
    const app = fixture.componentInstance;
    expect(app.title).toEqual("bitmagnet");
  });
});
