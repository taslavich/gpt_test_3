import { describe, expect, it, vi } from "vitest";
import { publisherAllPages } from "@/lib/publisherInventory";

describe("Publisher inventory pagination", () => {
  it("honors the server page size and returns all pages", async () => {
    const load = vi.fn(async ({offset}: {offset:number}) => ({
      limit:2,offset,rows:[1,2,3,4].slice(offset,offset+2),
    }));
    expect(await publisherAllPages(load)).toEqual([1,2,3,4]);
    expect(load.mock.calls.map(([page])=>page.offset)).toEqual([0,2,4]);
  });
  it("rejects an incomplete scan instead of reporting partial matches", async () => {
    const load = vi.fn(async ({offset}: {offset:number}) => {
      if (offset) throw new Error("Unavailable");
      return {limit:1,offset,rows:[1]};
    });
    await expect(publisherAllPages(load)).rejects.toThrow("Unavailable");
  });
  it("stops before requesting a page when canceled", async () => {
    const controller = new AbortController();
    controller.abort();
    const load = vi.fn();
    await expect(publisherAllPages(load,controller.signal)).rejects.toMatchObject({name:"AbortError"});
    expect(load).not.toHaveBeenCalled();
  });
  it("rejects a repeating full page with the wrong offset", async () => {
    const load = vi.fn(async () => ({limit:1,offset:0,items:[1]}));
    await expect(publisherAllPages(load)).rejects.toMatchObject({code:"PUBLISHER_CONTRACT"});
    expect(load).toHaveBeenCalledTimes(2);
  });
});
