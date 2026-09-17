export const CAMPAIGN_VIDEO_UNSUPPORTED_CODE = "CAMPAIGN_VIDEO_UNSUPPORTED";

export class CampaignValidationError extends Error {
  readonly code: string;

  constructor(code: string, message: string) {
    super(message);
    this.name = "CampaignValidationError";
    this.code = code;
  }
}

export function assertCampaignFormatSupported(formatType: string): void {
  if (formatType === "video") {
    throw new CampaignValidationError(
      CAMPAIGN_VIDEO_UNSUPPORTED_CODE,
      "Video campaigns are no longer supported. Choose banner, popunder, native, or push.",
    );
  }
}

export function isCampaignVideoUnsupportedError(error: unknown): boolean {
  return error instanceof CampaignValidationError
    && error.code === CAMPAIGN_VIDEO_UNSUPPORTED_CODE;
}
