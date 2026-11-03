import { publisherApi, type PublisherCodeDTO } from "@/api/publisher";
import { usePublisherResource } from "@/contexts/PublisherContext";
import { publisherAllPages } from "@/lib/publisherInventory";

export function usePublisherCodeDirectory(enabled = true) {
  return usePublisherResource(["code-directory"], signal =>
    publisherAllPages<PublisherCodeDTO>(publisherApi.codes, signal), enabled);
}
