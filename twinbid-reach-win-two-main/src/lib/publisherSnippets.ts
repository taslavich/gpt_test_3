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

/** Return the server's public link verbatim after checking that it is safe to copy. */
export function publisherDirectLink(value?: string | null): string {
  if (!value || !/^https:\/\//i.test(value) || /[\s\u0000-\u001f\u007f]/u.test(value))
    throw new Error("Invalid direct_link_url");
  const url = new URL(value);
  if (url.protocol !== "https:" || !url.hostname || url.username || url.password)
    throw new Error("Invalid direct_link_url");
  return value;
}
