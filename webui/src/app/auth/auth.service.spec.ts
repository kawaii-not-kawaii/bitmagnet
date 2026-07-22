import { TestBed } from "@angular/core/testing";
import { BROWSER_STORAGE } from "../browser-storage/browser-storage.service";
import { AuthService } from "./auth.service";

// fakeStorage is a minimal in-memory Storage for deterministic tests.
function fakeStorage(): Storage {
  const map = new Map<string, string>();
  return {
    get length() {
      return map.size;
    },
    clear: () => map.clear(),
    getItem: (k: string) => (map.has(k) ? (map.get(k) as string) : null),
    key: (i: number) => Array.from(map.keys())[i] ?? null,
    removeItem: (k: string) => map.delete(k),
    setItem: (k: string, v: string) => map.set(k, v),
  };
}

describe("AuthService", () => {
  let service: AuthService;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [{ provide: BROWSER_STORAGE, useValue: fakeStorage() }],
    });
    service = TestBed.inject(AuthService);
  });

  it("stores and returns the API key", () => {
    expect(service.getApiKey()).toBeNull();
    service.setApiKey("s3cret");
    expect(service.getApiKey()).toBe("s3cret");
  });

  it("trims whitespace and treats an empty key as clearing", () => {
    service.setApiKey("  padded  ");
    expect(service.getApiKey()).toBe("padded");

    service.setApiKey("   ");
    expect(service.getApiKey()).toBeNull();
  });

  it("clears the key", () => {
    service.setApiKey("s3cret");
    service.clearApiKey();
    expect(service.getApiKey()).toBeNull();
  });

  it("raises authRequired on a 401 signal and lowers it when a key is set", () => {
    expect(service.authRequired()).toBe(false);

    service.notifyAuthRequired();
    expect(service.authRequired()).toBe(true);

    service.setApiKey("s3cret");
    expect(service.authRequired()).toBe(false);
  });

  it("does not clear a stored key merely because auth was required", () => {
    service.setApiKey("maybe-mistyped");
    service.notifyAuthRequired();
    // The key is preserved; the operator may have hit a transient failure.
    expect(service.getApiKey()).toBe("maybe-mistyped");
  });
});
