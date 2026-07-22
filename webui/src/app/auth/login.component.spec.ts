import { ComponentFixture, TestBed } from "@angular/core/testing";
import { ActivatedRoute, Router, convertToParamMap } from "@angular/router";
import { of } from "rxjs";
import { AuthService } from "./auth.service";
import { LoginComponent } from "./login.component";

describe("LoginComponent", () => {
  let auth: jasmine.SpyObj<AuthService>;
  let fixture: ComponentFixture<LoginComponent>;
  let element: HTMLElement;
  let router: jasmine.SpyObj<Router>;

  beforeEach(async () => {
    auth = jasmine.createSpyObj<AuthService>("AuthService", ["login"]);
    auth.login.and.returnValue(of({ ok: true }));
    router = jasmine.createSpyObj<Router>("Router", ["navigateByUrl"]);
    router.navigateByUrl.and.resolveTo(true);

    await TestBed.configureTestingModule({
      imports: [LoginComponent],
      providers: [
        { provide: AuthService, useValue: auth },
        { provide: Router, useValue: router },
        {
          provide: ActivatedRoute,
          useValue: {
            snapshot: {
              queryParamMap: convertToParamMap({
                returnUrl: "/dashboard?tab=metrics",
              }),
            },
          },
        },
      ],
    }).compileComponents();
    fixture = TestBed.createComponent(LoginComponent);
    element = fixture.nativeElement as HTMLElement;
    fixture.detectChanges();
  });

  it("signs in with username and password and returns to the interrupted URL", async () => {
    enter("#login-username", "admin");
    enter("#login-password", "secret");
    fixture.detectChanges();
    submit();
    await fixture.whenStable();

    expect(auth.login.calls.mostRecent().args).toEqual([
      { username: "admin", password: "secret" },
    ]);
    expect(router.navigateByUrl.calls.mostRecent().args).toEqual([
      "/dashboard?tab=metrics",
    ]);
  });

  it("switches modes and signs in with an API key", async () => {
    const toggle = element.querySelector<HTMLButtonElement>(
      ".mode-toggle button",
    )!;
    toggle.click();
    fixture.detectChanges();

    expect(element.querySelector("#login-username")).toBeNull();
    expect(element.textContent).toContain("the same key torznab clients use");
    enter("#login-api-key", "bm_operator_key");
    fixture.detectChanges();
    submit();
    await fixture.whenStable();

    expect(auth.login.calls.mostRecent().args).toEqual([
      { apiKey: "bm_operator_key" },
    ]);
  });

  function enter(selector: string, value: string): void {
    const input = element.querySelector<HTMLInputElement>(selector)!;
    input.value = value;
    input.dispatchEvent(new Event("input"));
  }

  function submit(): void {
    const button = element.querySelector<HTMLButtonElement>(
      "button[type=submit]",
    )!;
    button.click();
  }
});
