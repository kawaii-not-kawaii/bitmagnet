import { provideHttpClient } from "@angular/common/http";
import {
  HttpTestingController,
  provideHttpClientTesting,
} from "@angular/common/http/testing";
import { TestBed } from "@angular/core/testing";
import { Router } from "@angular/router";
import { BROWSER_STORAGE } from "../browser-storage/browser-storage.service";
import { AuthService } from "./auth.service";

function fakeStorage(): Storage {
  const values = new Map<string, string>();
  return {
    get length() {
      return values.size;
    },
    clear: () => values.clear(),
    getItem: (key) => values.get(key) ?? null,
    key: (index) => Array.from(values.keys())[index] ?? null,
    removeItem: (key) => values.delete(key),
    setItem: (key, value) => values.set(key, value),
  };
}

describe("AuthService", () => {
  let http: HttpTestingController;
  let router: jasmine.SpyObj<Router>;
  let service: AuthService;
  let storage: Storage;

  beforeEach(() => {
    storage = fakeStorage();
    router = jasmine.createSpyObj<Router>(
      "Router",
      ["navigate", "navigateByUrl"],
      { url: "/dashboard" },
    );
    router.navigate.and.resolveTo(true);
    router.navigateByUrl.and.resolveTo(true);

    TestBed.configureTestingModule({
      providers: [
        provideHttpClient(),
        provideHttpClientTesting(),
        { provide: BROWSER_STORAGE, useValue: storage },
        { provide: Router, useValue: router },
      ],
    });
    http = TestBed.inject(HttpTestingController);
    service = TestBed.inject(AuthService);
  });

  afterEach(() => http.verify());

  it("uses the pinned REST contract with credentials", () => {
    service.setup("admin", "password123").subscribe();
    const setup = http.expectOne("/auth/setup");
    expect(setup.request.method).toBe("POST");
    expect(setup.request.body).toEqual({
      username: "admin",
      password: "password123",
    });
    expect(setup.request.withCredentials).toBeTrue();
    setup.flush({ ok: true });

    service.login({ apiKey: "bm_test" }).subscribe();
    const login = http.expectOne("/auth/login");
    expect(login.request.body).toEqual({ apiKey: "bm_test" });
    expect(login.request.withCredentials).toBeTrue();
    login.flush({ ok: true });

    service.logout().subscribe();
    const logout = http.expectOne("/auth/logout");
    expect(logout.request.withCredentials).toBeTrue();
    logout.flush(null);
  });

  it("routes a fresh deployment to setup", async () => {
    const bootstrapped = service.bootstrap();
    const request = http.expectOne("/auth/state");
    expect(request.request.withCredentials).toBeTrue();
    request.flush({
      authDisabled: false,
      needsSetup: true,
      trustedBypass: false,
    });

    await bootstrapped;
    expect(service.state()?.needsSetup).toBeTrue();
    expect(router.navigate.calls.mostRecent().args).toEqual([["/setup"]]);
  });

  it("routes a GraphQL 401 to login with the interrupted URL", () => {
    service.notifyAuthRequired();

    expect(router.navigate.calls.mostRecent().args).toEqual([
      ["/login"],
      { queryParams: { returnUrl: "/dashboard" } },
    ]);
  });

  it("migrates a legacy key once and deletes it after success", async () => {
    storage.setItem("bitmagnet.apiKey", "bm_legacy");
    const bootstrapped = service.bootstrap();

    const login = http.expectOne("/auth/login");
    expect(login.request.body).toEqual({ apiKey: "bm_legacy" });
    login.flush({ ok: true });
    await Promise.resolve();

    http.expectOne("/auth/state").flush({
      authDisabled: false,
      needsSetup: false,
      trustedBypass: false,
    });
    await bootstrapped;

    expect(storage.getItem("bitmagnet.apiKey")).toBeNull();
    http.expectNone("/auth/login");
    expect(router.navigate.calls.count()).toBe(0);
  });

  it("deletes a rejected legacy key and routes to login", async () => {
    storage.setItem("bitmagnet.apiKey", "wrong");
    const bootstrapped = service.bootstrap();

    http
      .expectOne("/auth/login")
      .flush(
        { error: "invalid credentials" },
        { status: 401, statusText: "Unauthorized" },
      );
    await Promise.resolve();

    http.expectOne("/auth/state").flush({
      authDisabled: false,
      needsSetup: false,
      trustedBypass: false,
    });
    await bootstrapped;

    expect(storage.getItem("bitmagnet.apiKey")).toBeNull();
    expect(router.navigate.calls.mostRecent().args).toEqual([
      ["/login"],
      { queryParams: { returnUrl: "/dashboard" } },
    ]);
  });
});
