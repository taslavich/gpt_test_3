const escapeAttribute = (value: string) =>
  value
    .replace(/&/g, "&amp;")
    .replace(/"/g, "&quot;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
/** Use only the SDK URL and short code supplied by the publisher API. */
export function buildJsSnippet(
  shortCode: string,
  publicScriptUrl: string,
): string {
  const url = new URL(publicScriptUrl);
  if (
    !shortCode.trim() ||
    !["https:", "http:"].includes(url.protocol) ||
    url.username ||
    url.password
  )
    throw new Error("Invalid installation metadata");
  if (url.pathname === "/static/adManager.js")
    throw new Error("Unsupported legacy installation URL");
  return `<script async src="${escapeAttribute(publicScriptUrl)}" food="${escapeAttribute(shortCode)}"></script>`;
}
