import type { PublisherCodeDTO, PublisherPage, PublisherRow } from "@/api/publisher";
import { ApiError } from "@/api/http";

/** Read all server pages before filtering; never treat one page as the full inventory. */
export async function publisherAllPages<T>(
  load: (page: PublisherPage, signal?: AbortSignal) => Promise<PublisherPage & { items?: T[]; rows?: T[] }>,
  signal?: AbortSignal,
): Promise<T[]> {
  const items: T[] = [];
  let offset = 0;
  while (offset <= 1000000) {
    if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
    const page = await load({ limit: 500, offset }, signal);
    const rows = page.items || page.rows || [];
    items.push(...rows);
    if (rows.length < page.limit) return items;
    if (page.offset !== offset || page.limit < 1) break;
    offset += page.limit;
  }
  throw new ApiError(200, "Invalid publisher inventory pagination", "PUBLISHER_CONTRACT");
}

export function publisherRowFormat(row: PublisherRow, codes: PublisherCodeDTO[]): string {
  return row.format || codes.find(code => code.ad_code_id === row.ad_code_id)?.format || "";
}

export function publisherFormatLabel(format: string): string {
  return ({ popunder: "Popunder", banner: "Banner", native: "Native", inpage_push: "In-Page Push", video_popup: "Video Popup", instream: "Instream", outstream: "Outstream" } as Record<string, string>)[format] || format || "—";
}
