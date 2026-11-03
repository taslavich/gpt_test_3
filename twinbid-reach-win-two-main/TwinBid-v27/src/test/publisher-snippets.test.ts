import { describe, expect, it } from "vitest";
import { buildJsSnippet } from "@/lib/publisherSnippets";

describe("Publisher installation metadata", () => {
  it("produces the exact food tag from server installation metadata", () => {
    expect(buildJsSnippet("tb_EXISTING_CODE", "https://carloverrcvbg.site/static/pageHelper.js")).toBe(
      '<script async src="https://carloverrcvbg.site/static/pageHelper.js" food="tb_EXISTING_CODE"></script>',
    );
  });

  it.each([
    "https://stable.example.test/sdk.js",
    "http://pub.example.test/custom-loader.js",
    "https://twinbid.io/sdk.js",
  ])("preserves the configured server URL %s", (url) => {
    expect(buildJsSnippet("tb_actual", url)).toBe(
      `<script async src="${url}" food="tb_actual"></script>`,
    );
  });

  it.each([
    "https://twinbid.io/static/adManager.js",
    "https://cdn.twinbid.io/static/adManager.js",
    "https://twinbidexchange.com/static/adManager.js",
    "https://pub.example.test/static/adManager.js?version=1#loader",
  ])("rejects the obsolete loader %s without rewriting it", (url) => {
    expect(() => buildJsSnippet("tb_actual", url)).toThrow();
  });

  it("escapes both HTML attributes without changing server values", () => {
    expect(buildJsSnippet('tb_"<>&', 'https://stable.example.test/sdk.js?a=1&b="<>')).toBe(
      '<script async src="https://stable.example.test/sdk.js?a=1&amp;b=&quot;&lt;&gt;" food="tb_&quot;&lt;&gt;&amp;"></script>',
    );
  });

  it.each([
    "javascript:alert(1)",
    "data:text/javascript,alert(1)",
    "https://user:password@example.test/a.js",
    "https://user@example.test/a.js",
    "not a URL",
    "",
  ])("retains URL validation for %s", (url) => {
    expect(() => buildJsSnippet("tb_actual", url)).toThrow();
  });

  it("rejects a missing short code", () => {
    expect(() => buildJsSnippet("  ", "https://stable.example.test/sdk.js")).toThrow();
  });
});
