import {
  LLM_PRESETS_KEY,
  deletePreset,
  loadPresets,
  presetFormValue,
  savePreset,
} from "./dashboard-llm.presets";
import type { LlmPreset } from "./dashboard-llm.presets";
import { REDACTED_VALUE } from "./dashboard-llm.service";
import type { LlmConfigFormValue } from "./dashboard-llm.service";

describe("dashboard LLM presets", () => {
  let storage: Storage;

  beforeEach(() => {
    storage = fakeStorage();
  });

  it("round trips a saved preset", () => {
    const saved = savePreset(" local ", formValue(), storage);

    expect(saved).toEqual([{ name: "local", value: formValue() }]);
    expect(loadPresets(storage)).toEqual(saved);
  });

  it("redacts API keys in serialized storage unconditionally", () => {
    const secret = "sk-real-secret";

    savePreset("hosted", formValue({ apiKey: secret }), storage);

    const serialized = storage.getItem(LLM_PRESETS_KEY) ?? "";
    expect(serialized).not.toContain(secret);
    expect(serialized).toContain(REDACTED_VALUE);
  });

  it("replaces an exact name in its original position", () => {
    savePreset("first", formValue({ model: "old" }), storage);
    savePreset("second", formValue({ model: "second" }), storage);

    const updated = savePreset(
      "first",
      formValue({ model: "new", maxTokens: 512 }),
      storage,
    );

    expect(updated.map(({ name }) => name)).toEqual(["first", "second"]);
    expect(updated[0].value.model).toBe("new");
    expect(updated[0].value.maxTokens).toBe(512);
    expect(storage.getItem(LLM_PRESETS_KEY)).toBe(JSON.stringify(updated));
  });

  it("does not write empty or whitespace-only names", () => {
    const existing = savePreset("kept", formValue(), storage);
    const serialized = storage.getItem(LLM_PRESETS_KEY);

    expect(savePreset("", formValue({ model: "empty" }), storage)).toEqual(
      existing,
    );
    expect(
      savePreset(" \t ", formValue({ model: "whitespace" }), storage),
    ).toEqual(existing);
    expect(storage.getItem(LLM_PRESETS_KEY)).toBe(serialized);
  });

  it("rejects missing, malformed, or wrongly shaped storage", () => {
    const missingModel = { ...formValue(), model: undefined };
    const cases: Array<{ name: string; serialized?: string }> = [
      { name: "missing key" },
      { name: "invalid JSON", serialized: "not json" },
      { name: "non-array object", serialized: "{}" },
      {
        name: "non-object array entry",
        serialized: JSON.stringify([preset("valid"), null]),
      },
      {
        name: "missing value field",
        serialized: JSON.stringify([{ name: "missing", value: missingModel }]),
      },
      {
        name: "wrong-typed value field",
        serialized: JSON.stringify([
          {
            name: "wrong type",
            value: { ...formValue(), concurrency: "8" },
          },
        ]),
      },
    ];

    for (const testCase of cases) {
      storage.clear();
      if (testCase.serialized !== undefined) {
        storage.setItem(LLM_PRESETS_KEY, testCase.serialized);
      }

      expect(loadPresets(storage)).withContext(testCase.name).toEqual([]);
    }
  });

  it("deletes only the exact preset name", () => {
    savePreset("local", formValue(), storage);
    savePreset("Local", formValue(), storage);
    savePreset("hosted", formValue(), storage);

    const remaining = deletePreset("local", storage);

    expect(remaining.map(({ name }) => name)).toEqual(["Local", "hosted"]);
    expect(loadPresets(storage)).toEqual(remaining);
  });

  it("does not write when deleting an unknown name", () => {
    const existing = savePreset("kept", formValue(), storage);
    const setItem = spyOn(storage, "setItem").and.callThrough();

    expect(deletePreset("missing", storage)).toEqual(existing);
    expect(setItem).not.toHaveBeenCalled();
  });

  it("preserves a redacted key for the same provider identity", () => {
    const current = formValue({ apiKey: REDACTED_VALUE });
    const selected = preset("same", { model: "new-model" });

    expect(presetFormValue(selected, current)).toEqual({
      ...selected.value,
      apiKey: REDACTED_VALUE,
    });
  });

  it("clears the key when the provider name differs", () => {
    const current = formValue({ apiKey: "current-secret" });
    const selected = preset("other", { providerName: "other" });

    expect(presetFormValue(selected, current)).toEqual({
      ...selected.value,
      apiKey: "",
    });
  });

  it("clears the key when the base URL differs", () => {
    const current = formValue({ apiKey: "current-secret" });
    const selected = preset("other", { baseUrl: "http://other" });

    expect(presetFormValue(selected, current)).toEqual({
      ...selected.value,
      apiKey: "",
    });
  });

  it("ignores whitespace and trailing slashes in provider identity", () => {
    const current = formValue({ apiKey: "current-secret" });
    const selected = preset("same", {
      providerName: " openai ",
      baseUrl: " http://x/// ",
    });

    expect(presetFormValue(selected, current).apiKey).toBe("current-secret");
  });

  it("returns safe fallbacks when storage access fails", () => {
    spyOn(storage, "getItem").and.throwError("storage blocked");
    expect(loadPresets(storage)).toEqual([]);

    storage = fakeStorage();
    const existing = savePreset("kept", formValue(), storage);
    spyOn(storage, "setItem").and.throwError("storage full");

    expect(savePreset("new", formValue(), storage)).toEqual(existing);
    expect(deletePreset("kept", storage)).toEqual(existing);
  });

  it("leaves the current form untouched for a malformed preset", () => {
    const current = formValue({ apiKey: "current-secret" });
    const malformed = { name: "bad", value: {} } as unknown as LlmPreset;

    expect(presetFormValue(malformed, current)).toBe(current);
  });
});

function formValue(
  overrides: Partial<LlmConfigFormValue> = {},
): LlmConfigFormValue {
  return {
    enabled: true,
    concurrency: 8,
    autoScale: true,
    providerName: "openai",
    baseUrl: "http://x",
    model: "model",
    apiKey: REDACTED_VALUE,
    batchSize: 1,
    maxContext: 16000,
    maxTokens: 256,
    intervalSeconds: 5,
    timeoutSeconds: 30,
    ...overrides,
  };
}

function preset(
  name: string,
  overrides: Partial<LlmConfigFormValue> = {},
): LlmPreset {
  return { name, value: formValue(overrides) };
}

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
