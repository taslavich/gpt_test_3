export async function submitCreatedCampaignToModeration(
  campaignId: string,
  updateCampaign: (id: string, updates: { status: "moderation" }) => Promise<void>,
): Promise<unknown | null> {
  try {
    await updateCampaign(campaignId, { status: "moderation" });
    return null;
  } catch (error: unknown) {
    // Campaign creation has already succeeded. Return the secondary
    // moderation error instead of rejecting the complete create flow.
    return error;
  }
}
