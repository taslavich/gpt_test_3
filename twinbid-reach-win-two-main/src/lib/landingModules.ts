// Share the loaded route with the transition. A preloaded destination renders
// synchronously, without showing the Suspense fallback for a frame.
let publisherLandingModule: typeof import("@/pages/PublisherLanding") | null = null;

export const loadPublisherLanding = () => import("@/pages/PublisherLanding").then(module => {
  publisherLandingModule = module;
  return module;
});

export const getPublisherLanding = () => publisherLandingModule?.default;
