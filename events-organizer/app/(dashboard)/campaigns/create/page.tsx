'use client';
// Ads/campaigns are DISABLED for launch. Feed placement is fully organic —
// published events appear in the TicketHawk feed without paid promotion.
// This stub keeps the route harmless (old links/bookmarks) by bouncing to the
// dashboard. Restore from git history when paid campaigns return.
import { useEffect } from 'react';
import { useRouter } from 'next/navigation';

export default function CampaignsDisabled() {
  const router = useRouter();
  useEffect(() => { router.replace('/dashboard'); }, [router]);
  return null;
}