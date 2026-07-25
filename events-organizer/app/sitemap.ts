import type { MetadataRoute } from 'next';
import { SITE_URL } from './site';

// Serves /sitemap.xml. Only the public landing page belongs here — the rest of
// the app is behind auth and is disallowed in robots.ts.
export default function sitemap(): MetadataRoute.Sitemap {
  return [
    {
      url: SITE_URL,
      lastModified: new Date(),
      changeFrequency: 'weekly',
      priority: 1,
    },
  ];
}