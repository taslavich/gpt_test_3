# Banner loader contract

The publisher cabinet gives every banner code the same script tag, to be included once per HTML page:

```html
<script async src="https://cdn.twinbid.io/static/adManager.js"></script>
```

Each independent placement has its own HTML container, with the code ID and size on that element:

```html
<div data-banner-code-id="banner-a" data-banner-size="300x250"></div>
<div data-banner-code-id="banner-b" data-banner-size="728x90"></div>
<div data-banner-code-id="banner-c" data-banner-size="300x250"></div>
```

The CDN script must discover **all** `data-banner-code-id` containers, read each ID and size, and request and render an ad for each container independently. It must not require a `data-ad-code-id` attribute on its own script element for banners. It should initialize a container once, even if scanning is repeated. The ID identifies a publisher ad code; site and zone resolution remains based on the request's domain and ad-code pair. Separate placements use separate ad codes, including two placements of the same size.

This repository contains the publisher UI and snippet generator, but not the CDN script or ad-serving implementation. Deploying the UI alone does not make the CDN loader honor this contract. Update and verify the actual `adManager.js` before using these generated snippets in production inventory. Non-banner formats continue to generate their existing code-specific script tag.
