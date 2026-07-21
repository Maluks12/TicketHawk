'use client';
// This page was the AD BILLING surface (ad accounts, card payment methods,
// billing records) — ads are DISABLED for launch, so the whole page is
// retired. Organiser payouts themselves are unaffected: the payout worker
// settles completed events server-side. Restore from git history when paid
// campaigns return.
import { useEffect } from 'react';
import { useRouter } from 'next/navigation';

export default function BillingDisabled() {
  const router = useRouter();
  useEffect(() => { router.replace('/dashboard'); }, [router]);
  return null;
}