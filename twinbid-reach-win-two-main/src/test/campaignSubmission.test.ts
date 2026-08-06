import { describe, expect, it, vi } from "vitest";
import { submitCreatedCampaignToModeration } from "@/lib/campaignSubmission";

describe("submitCreatedCampaignToModeration", () => {
  it("does not reject the completed create flow when moderation submission fails", async () => {
    const error = new Error("telegram moderation bot failed");
    const updateCampaign = vi.fn().mockRejectedValue(error);

    await expect(
      submitCreatedCampaignToModeration("campaign-1", updateCampaign),
    ).resolves.toBe(error);
    expect(updateCampaign).toHaveBeenCalledWith("campaign-1", { status: "moderation" });
  });
});
