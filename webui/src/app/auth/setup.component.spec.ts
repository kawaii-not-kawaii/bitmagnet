import { ComponentFixture, TestBed } from "@angular/core/testing";
import { Router } from "@angular/router";
import { of } from "rxjs";
import { AuthService } from "./auth.service";
import { SetupComponent } from "./setup.component";

describe("SetupComponent", () => {
  let auth: jasmine.SpyObj<AuthService>;
  let fixture: ComponentFixture<SetupComponent>;
  let element: HTMLElement;
  let router: jasmine.SpyObj<Router>;

  beforeEach(async () => {
    auth = jasmine.createSpyObj<AuthService>("AuthService", ["setup"]);
    auth.setup.and.returnValue(of({ ok: true }));
    router = jasmine.createSpyObj<Router>("Router", ["navigateByUrl"]);
    router.navigateByUrl.and.resolveTo(true);

    await TestBed.configureTestingModule({
      imports: [SetupComponent],
      providers: [
        { provide: AuthService, useValue: auth },
        { provide: Router, useValue: router },
      ],
    }).compileComponents();
    fixture = TestBed.createComponent(SetupComponent);
    element = fixture.nativeElement as HTMLElement;
    fixture.detectChanges();
  });

  it("creates the account and continues into the app", async () => {
    enter("#setup-username", "admin");
    enter("#setup-password", "correct horse battery staple");
    enter("#setup-confirm", "correct horse battery staple");
    fixture.detectChanges();

    expect(element.querySelector(".match")?.textContent).toContain(
      "passwords match",
    );
    const button = element.querySelector<HTMLButtonElement>(
      "button[type=submit]",
    )!;
    expect(button.disabled).toBeFalse();
    button.click();
    await fixture.whenStable();

    expect(auth.setup.calls.count()).toBe(1);
    expect(auth.setup.calls.mostRecent().args).toEqual([
      "admin",
      "correct horse battery staple",
    ]);
    expect(router.navigateByUrl.calls.mostRecent().args).toEqual(["/torrents"]);
  });

  it("requires at least eight matching password characters", () => {
    enter("#setup-username", "admin");
    enter("#setup-password", "short");
    enter("#setup-confirm", "short");
    fixture.detectChanges();

    const button = element.querySelector<HTMLButtonElement>(
      "button[type=submit]",
    )!;
    expect(button.disabled).toBeTrue();
    expect(auth.setup.calls.count()).toBe(0);
  });

  function enter(selector: string, value: string): void {
    const input = element.querySelector<HTMLInputElement>(selector)!;
    input.value = value;
    input.dispatchEvent(new Event("input"));
  }
});
