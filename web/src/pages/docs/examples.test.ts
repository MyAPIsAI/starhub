import { describe, expect, it } from "vitest";
import { normalizeDocsBase, quickstartExamples } from "./examples";

describe("documentation configuration", () => {
  it("normalizes deployment endpoints and rejects secret-bearing or unsafe URLs", () => {
    expect(normalizeDocsBase("https://gateway.example/proxy/v1/")).toBe(
      "https://gateway.example/proxy/v1",
    );
    expect(normalizeDocsBase("http://127.0.0.1:3000/v1")).toBe(
      "http://127.0.0.1:3000/v1",
    );
    for (const input of [
      "https://user:secret@gateway.example/v1",
      "https://gateway.example/v1?key=secret",
      "http://gateway.example/v1",
      "https://gateway.example/$(command)/v1",
      "javascript:alert(1)",
      "https://gateway.example/v1#secret",
    ])
      expect(normalizeDocsBase(input)).toBeUndefined();
  });
  it("quotes model identifiers without changing the request or executing shell interpolation", () => {
    const examples = quickstartExamples(
      "https://gateway.example/v1",
      "model'$(example)\"fixture",
    );
    expect(examples.environment).toContain(`'model'"'"'$(example)"fixture'`);
    expect(examples.environmentWindows).toContain(
      "'model''$(example)\"fixture'",
    );
    expect(examples.curl).toContain("'\"'\"'");
    expect(examples.python).toContain('os.environ["MYAPIS_MODEL"]');
  });
});
