// ─────────────────────────────────────────────────────────────────────────────
// Universal-link landing pad:  https://ticket-hawk.com/events/<id>
//
// The share sheet emits the WEB url (see lib/shareLinks.ts) so the link works
// for people who don't have the app. When the app IS installed, iOS/Android
// hand that same URL to expo-router, which resolves it against the file tree —
// so the app needs a route at the web path, /events/[id], not just its own
// internal /event/[id].
//
// Rather than duplicate the event screen, this redirects. `<Redirect>` replaces
// the entry in the history stack instead of pushing on top of it, so the back
// gesture from the event screen exits to the tabs rather than bouncing through
// an invisible intermediate route.
//
// The web path is PLURAL (/events/:id, matching web-mobile and web-desktop) and
// the native path is SINGULAR (/event/:id). Keep them that way: changing the web
// path breaks every link already shared, and changing the native path breaks
// every router.push in the app.
// ─────────────────────────────────────────────────────────────────────────────

import { Redirect, useLocalSearchParams } from 'expo-router';

export default function EventDeepLinkRedirect() {
  const { id } = useLocalSearchParams<{ id: string }>();

  // A malformed link (/events/ with nothing after it) goes home rather than to
  // an event screen that would render "EVENT NOT FOUND".
  if (!id) return <Redirect href="/(tabs)" />;

  return <Redirect href={`/event/${id}`} />;
}