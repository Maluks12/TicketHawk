'use client';
// ─────────────────────────────────────────────────────────────────────────────
// Staff permissions (client)
//
// THE SERVER IS THE AUTHORITY. This module does not decide what anyone may do —
// it decides what to DRAW. Every gate here has a matching check in
// internal/middleware/staff_middleware.go, and that one is the control.
//
// Which is why capabilities are FETCHED rather than derived from the role in a
// local copy of the matrix. Two copies of a permission table drift, and the
// drift is silent: the console renders a button the API then refuses, or hides
// one the user is entitled to. Asking the server what it will allow means the
// UI can be wrong about presentation but never about policy.
//
// Hiding vs disabling: an action the user can never perform is HIDDEN, because
// a permanently greyed-out button is just noise. An action blocked by state
// rather than permission — an approval they can't give because it's their own
// request — is DISABLED with the reason, because that one is temporary and the
// reason is the useful part.
// ─────────────────────────────────────────────────────────────────────────────
import { create } from 'zustand';
import { api } from './api';

export type StaffRole = 'read_only' | 'access' | 'editor' | 'admin';

export type Capability =
  | 'event.view' | 'user.list' | 'user.contact' | 'bank.view_tail'
  | 'kyc.status' | 'kyc.document' | 'audit.view'
  | 'event.edit' | 'event.approve' | 'event.feature' | 'kyc.verify'
  | 'user.edit_contact' | 'user.request_otp' | 'user.reset_credentials'
  | 'request.create' | 'request.act' | 'request.approve'
  | 'staff.view' | 'staff.manage';

/** Human labels for roles, used in badges and the staff table. */
export const ROLE_LABEL: Record<StaffRole, string> = {
  read_only: 'Read only',
  access: 'Access',
  editor: 'Editor',
  admin: 'Admin',
};

/** What each role is FOR — shown when granting a role, so the choice is informed. */
export const ROLE_DESCRIPTION: Record<StaffRole, string> = {
  read_only: 'View events and the user list. No personal details, no changes.',
  access: 'Everything above, plus emails, KYC status and the last 4 digits of bank accounts. Can approve initial KYC. Cannot edit anything.',
  editor: 'Everything above, plus editing events and acting on accounts. Dangerous actions — bank details, takedowns — are logged as requests for an admin to approve.',
  admin: 'Everything an editor can do, without needing approval, plus approving other people\u2019s requests and managing staff.',
};

interface StaffState {
  role: StaffRole | null;
  capabilities: Capability[];
  email: string | null;
  /**
   * Break-glass right to read identity documents. NOT a capability — it lives
   * on its own column (migration 000071) and is reported separately, because
   * console rank and document access are deliberately different axes. Being
   * promoted to admin never grants it.
   *
   * Presentational only. Every endpoint it reveals re-checks it server-side.
   */
  isMasterAdmin: boolean;
  /**
   * True when this account predates the staff model and holds no staff_role.
   * The server treats it as admin for console purposes so the owner is not
   * locked out of their own dashboard, and the console says so plainly rather
   * than letting it look like a granted role.
   */
  legacyAdmin: boolean;
  loaded: boolean;
  failed: boolean;
  load: (force?: boolean) => Promise<void>;
  can: (c: Capability) => boolean;
}

/**
 * Shared in-flight promise.
 *
 * Every gated page calls load() in an effect, so navigating fired one identical
 * request per screen — and on a cold load, several at once. The store is a
 * singleton, so one fetch is enough; concurrent callers await the same promise
 * and later ones return immediately from cache.
 */
let inFlight: Promise<void> | null = null;

export const useStaff = create<StaffState>((set, get) => ({
  role: null,
  capabilities: [],
  email: null,
  isMasterAdmin: false,
  legacyAdmin: false,
  loaded: false,
  failed: false,

  load: async (force = false) => {
    if (!force && get().loaded && !get().failed) return;
    if (inFlight) return inFlight;

    inFlight = (async () => {
      try {
        const { data } = await api.get('/admin/me/capabilities');
        set({
          role: data?.staff_role ?? null,
          capabilities: data?.capabilities ?? [],
          email: data?.email ?? null,
          isMasterAdmin: data?.is_master_admin === true,
          legacyAdmin: data?.legacy_admin === true,
          loaded: true,
          failed: false,
        });
      } catch {
        // FAIL CLOSED — the opposite of the organiser's legal gate.
        //
        // There, a failed check cost a prompt. Here it would decide whether to
        // render bank details and approval buttons, so an unreachable endpoint
        // must mean "no permissions", never "assume the best". An empty
        // capability list hides everything gated, which is the safe shape.
        //
        // isMasterAdmin false is part of that: a failed check must never draw
        // the identity-document unlock form.
        set({
          role: null, capabilities: [], email: null,
          isMasterAdmin: false, legacyAdmin: false,
          loaded: true, failed: true,
        });
      } finally {
        inFlight = null;
      }
    })();

    return inFlight;
  },

  can: (c) => get().capabilities.includes(c),
}));

/**
 * Renders children only when the capability is held.
 *
 * Default is to render NOTHING when denied. Pass `fallback` for the cases where
 * the absence itself needs explaining — a masked field, say, where showing
 * nothing would read as "this organiser has no bank account" rather than "you
 * can't see it".
 */
export function Can({
  do: capability, children, fallback = null,
}: {
  do: Capability;
  children: React.ReactNode;
  fallback?: React.ReactNode;
}) {
  const can = useStaff((s) => s.can(capability));
  return <>{can ? children : fallback}</>;
}

/**
 * Full-page denial, for a route the user reached but may not use.
 *
 * Names the capability and the role. A bare "Access denied" produces a support
 * ticket; telling someone they need `request.approve` and currently hold
 * `editor` lets them ask for the right thing.
 */
export function AccessDenied({ need }: { need?: Capability }) {
  const role = useStaff((s) => s.role);
  return (
    <div className="page">
      <div style={{
        display: 'flex', flexDirection: 'column', alignItems: 'center',
        justifyContent: 'center', gap: 14, padding: '80px 24px', textAlign: 'center',
      }}>
        <div style={{
          width: 60, height: 60, borderRadius: 20,
          background: 'var(--danger-bg)', border: '1px solid var(--danger-border)',
          display: 'grid', placeItems: 'center', color: 'var(--danger-fg)',
        }}>
          <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor"
            strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <rect x="3" y="11" width="18" height="11" rx="2.5" />
            <path d="M7 11V7a5 5 0 0110 0v4" />
          </svg>
        </div>

        <div style={{ fontSize: 18, fontWeight: 800, color: 'var(--text)' }}>
          You don&apos;t have access to this
        </div>

        <div style={{
          fontSize: 13.5, color: 'var(--text-2)', maxWidth: 420, lineHeight: 1.65,
        }}>
          {need
            ? <>This section needs the <code style={{
                background: 'var(--surface-strong)', padding: '1px 6px',
                borderRadius: 5, fontSize: 12.5,
              }}>{need}</code> permission.</>
            : <>This section needs a higher permission level.</>}
          {role && <> Your role is <strong style={{ color: 'var(--text)' }}>{ROLE_LABEL[role]}</strong>.</>}
          {' '}Ask an admin if you need it.
        </div>

        {/* Recorded server-side too. Saying so is not a threat — it tells an
            honest person who hit the wrong link that nothing is wrong, and
            tells a dishonest one that the attempt was not invisible. */}
        <div style={{ fontSize: 12, color: 'var(--text-3)', marginTop: 4 }}>
          This attempt has been logged.
        </div>
      </div>
    </div>
  );
}

/**
 * Masks a bank account to its last four digits.
 *
 * Lives here rather than in a component so there is ONE implementation. Every
 * place that renders an account number goes through this, which means no screen
 * can accidentally print a full one — the server also never sends it, so this
 * is the second of two independent guards.
 */
export function maskAccount(tail: string | null | undefined): string {
  if (!tail) return '—';
  const digits = tail.replace(/\D/g, '');
  if (digits.length <= 4) return `•••• ${digits}`;
  return `•••• ${digits.slice(-4)}`;
}