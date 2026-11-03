import { BANNER_SIZES } from "@/lib/creativeTarget";

interface BannerCreativeLike {
  id: string;
  name?: string;
  bannerSize?: string;
  sizeMismatch?: boolean;
  allBannerSizesGenerated?: boolean;
}

interface CreateBannerSizeVariantsOptions {
  startIndex: number;
  creativeLabel: string;
  sourceWidth: number;
  sourceHeight: number;
  createId: () => string;
}

export function createBannerSizeVariants<T extends BannerCreativeLike>(
  creative: T,
  options: CreateBannerSizeVariantsOptions,
): Array<T & {
  name: string;
  bannerSize: string;
  sizeMismatch: boolean;
  allBannerSizesGenerated: true;
}> {
  return BANNER_SIZES.map((bannerSize, sizeIndex) => {
    const [w, h] = bannerSize.split("x").map(Number);
    return {
      ...creative,
      id: sizeIndex === 0 ? creative.id : options.createId(),
      name: `${options.creativeLabel} ${options.startIndex + sizeIndex + 1}`,
      bannerSize,
      sizeMismatch: options.sourceWidth !== w || options.sourceHeight !== h,
      allBannerSizesGenerated: true,
    };
  });
}
