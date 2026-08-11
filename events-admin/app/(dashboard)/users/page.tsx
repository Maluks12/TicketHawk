'use client';
import { useCallback, useEffect, useState } from 'react';
import { api } from '../../../lib/api';
import { useToast } from '../../../lib/toast';
import { useConfirm } from '../../../lib/confirm';
import {
  useStaff, AccessDenied, maskAccount,
  ROLE_LABEL, ROLE_DESCRIPTION, StaffRole,
} from '../../../lib/permissions';

// ─────────────────────────────────────────────────────────────────────────────
// People — Users and Staff
//
// Two tabs, because they answer two different questions. "Which organisers
// exist" is asked constantly by support; "who can sign in to this console" is
// asked rarely and only by admins. Mixed into one list, the second was
// effectively invisible — you could not answer "who can see our users' data"
// without reading every row.
//
// THIS COMPONENT DOES NOT DECIDE PERMISSIONS.
//
// The server sends a different SHAPE per capability: a read_only staff member's
// payload contains no email key at all. The gating here is presentational — it
// stops the table rendering a column with nothing in it. Hiding a field the API
// already sent would be a curtain, not a control, and anyone with devtools
// could pull it back.
//
// That is why every gated field is optional below, and why the column set comes
// from `fields` in the response rather than from the role.
// ─────────────────────────────────────────────────────────────────────────────

interface Person {
  id: string;
  display_name: string;
  role: string;
  staff_role?: StaffRole | null;
  is_organiser: boolean;
  // Present only when the caller holds the matching capability.
  email?: string;
  phone?: string;
  kyc_level?: number;
  kyc_status?: string;
  bank_tail?: string;
  bank_name?: string;
}

interface Fields { contact: boolean; bank: boolean; kyc: boolean }

type Tab = 'users' | 'staff';

export default function PeoplePage() {
  const toast = useToast();
  const confirm = useConfirm();

  const capsLoaded = useStaff((s) => s.loaded);
  const can = useStaff((s) => s.can);
  const loadCaps = useStaff((s) => s.load);

  const [tab, setTab] = useState<Tab>('users');
  const [people, setPeople] = useState<Person[]>([]);
  const [fields, setFields] = useState<Fields>({ contact: false, bank: false, kyc: false });
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(true);
  const [denied, setDenied] = useState(false);

  useEffect(() => { loadCaps(); }, [loadCaps]);

  const load = useCallback(async (which: Tab, q: string) => {
    setLoading(true);
    try {
      const { data } = await api.get('/admin/people', { params: { tab: which, q } });
      setPeople(data?.people ?? []);
      setFields(data?.fields ?? { contact: false, bank: false, kyc: false });
      setDenied(false);
    } catch (e) {
      // A 403 is a permission ANSWER, not a failure, so it gets the deny screen
      // rather than an error toast — nothing went wrong.
      if ((e as { response?: { status?: number } })?.response?.status === 403) {
        setDenied(true);
      } else {
        toast('Failed to load', 'error');
      }
      setPeople([]);
    } finally {
      setLoading(false);
    }
  }, [toast]);

  // Debounced so typing doesn't fire a request per keystroke. Search runs
  // server-side because the client only ever holds the page it was sent.
  useEffect(() => {
    const t = setTimeout(() => { load(tab, search); }, search ? 300 : 0);
    return () => clearTimeout(t);
  }, [tab, search, load]);

  async function setStaffRole(p: Person, role: StaffRole | null) {
    const verb = role
      ? `make ${p.display_name} a ${ROLE_LABEL[role]}`
      : `revoke ${p.display_name}'s console access`;

    confirm('Change console access', `Are you sure you want to ${verb}?`, async () => {
      try {
        await api.put(`/admin/staff/${p.id}/role`, { staff_role: role });
        toast('Console access updated', 'success');
        load(tab, search);
      } catch (e) {
        // The server refuses self-changes and last-admin removal with a 409 and
        // an explanation. Showing it verbatim beats a generic failure, because
        // the reason is the actionable part.
        const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
        toast(msg ?? 'Could not update access', 'error');
      }
    });
  }

  if (!capsLoaded) {
    return <div className="page-loader"><div className="spinner spinner-lg" /></div>;
  }
  if (!can('user.list')) {
    return <AccessDenied need="user.list" />;
  }

  const showStaffTab = can('staff.view');

  return (
    <div style={{ padding: '24px 28px 48px', maxWidth: 1440, margin: '0 auto' }}>
      <div style={{ marginBottom: 18 }}>
        <div className="page-title">People</div>
        <div className="page-sub">
          {tab === 'staff'
            ? 'Who can sign in to this console, and what they can do'
            : 'Platform accounts — attendees and organisers'}
        </div>
      </div>

      {/* The Staff tab is HIDDEN, not disabled, without staff.view. A greyed-out
          tab advertises that a staff roster exists and invites probing; an
          absent one says nothing at all. */}
      <div className="tabs">
        <button className={`tab${tab === 'users' ? ' active' : ''}`} onClick={() => setTab('users')}>
          Users
        </button>
        {showStaffTab && (
          <button className={`tab${tab === 'staff' ? ' active' : ''}`} onClick={() => setTab('staff')}>
            Staff
          </button>
        )}
      </div>

      {denied ? (
        <AccessDenied need={tab === 'staff' ? 'staff.view' : 'user.list'} />
      ) : (
        <>
          <div className="table-toolbar">
            <input
              className="search-input"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={fields.contact ? 'Search by name or email…' : 'Search by name…'}
            />
            <span style={{ fontSize: 12.5, color: 'var(--text-3)' }}>
              {loading ? 'Loading…' : `${people.length} ${people.length === 1 ? 'person' : 'people'}`}
            </span>
          </div>

          {/* Says WHY the table is sparse. Without it, a read_only member sees
              two columns and reasonably concludes the tool is broken rather
              than that they're seeing exactly what they're entitled to. */}
          {!fields.contact && (
            <div className="alert alert-info" style={{ fontSize: 12.5 }}>
              <span>
                You&apos;re seeing names and roles only. Contact details and bank
                information need a higher access level.
              </span>
            </div>
          )}

          <div className="card">
            <div className="table-wrap">
              <table className="meta-table">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Role</th>
                    {tab === 'staff' && <th>Console access</th>}
                    {fields.contact && <th>Email</th>}
                    {fields.kyc && <th>KYC</th>}
                    {fields.bank && <th>Bank</th>}
                    {tab === 'staff' && can('staff.manage') && (
                      <th style={{ textAlign: 'right' }}>Access level</th>
                    )}
                  </tr>
                </thead>
                <tbody>
                  {!loading && people.length === 0 && (
                    <tr>
                      <td colSpan={8} style={{ textAlign: 'center', color: 'var(--text-3)', padding: 30 }}>
                        {search ? 'Nobody matches that search.' : 'Nobody here yet.'}
                      </td>
                    </tr>
                  )}

                  {people.map((p) => (
                    <tr key={p.id}>
                      <td style={{ fontWeight: 600 }}>
                        {p.display_name}
                        {p.is_organiser && (
                          <span className="badge badge-organizer" style={{ marginLeft: 8 }}>
                            Organiser
                          </span>
                        )}
                      </td>

                      <td>
                        <span className={`badge badge-${
                          p.role === 'admin' ? 'admin'
                          : p.role === 'organizer' ? 'organizer'
                          : 'attendee'}`}>
                          {p.role}
                        </span>
                      </td>

                      {tab === 'staff' && (
                        <td>
                          {p.staff_role ? (
                            <span className="badge badge-admin" title={ROLE_DESCRIPTION[p.staff_role]}>
                              {ROLE_LABEL[p.staff_role]}
                            </span>
                          ) : <span style={{ color: 'var(--text-3)' }}>—</span>}
                        </td>
                      )}

                      {fields.contact && (
                        <td style={{ color: 'var(--text-2)' }}>{p.email ?? '—'}</td>
                      )}

                      {fields.kyc && (
                        <td>
                          {p.kyc_status
                            ? <span className={`badge badge-${p.kyc_status === 'verified' ? 'active' : 'pending'}`}>
                                {p.kyc_status}
                              </span>
                            : <span style={{ color: 'var(--text-3)' }}>—</span>}
                        </td>
                      )}

                      {fields.bank && (
                        <td style={{ fontVariantNumeric: 'tabular-nums', color: 'var(--text-2)' }}>
                          {/* maskAccount is the only place an account number is
                              formatted, and the server never sends more than
                              four digits — two independent guards. */}
                          {p.bank_tail ? (
                            <>
                              {maskAccount(p.bank_tail)}
                              {p.bank_name && (
                                <div style={{ fontSize: 11.5, color: 'var(--text-3)' }}>{p.bank_name}</div>
                              )}
                            </>
                          ) : <span style={{ color: 'var(--text-3)' }}>—</span>}
                        </td>
                      )}

                      {tab === 'staff' && can('staff.manage') && (
                        <td style={{ textAlign: 'right' }}>
                          <select
                            className="filter-select"
                            value={p.staff_role ?? ''}
                            onChange={(e) => setStaffRole(p, (e.target.value || null) as StaffRole | null)}
                            style={{ minWidth: 130 }}
                          >
                            <option value="">No access</option>
                            <option value="read_only">Read only</option>
                            <option value="access">Access</option>
                            <option value="editor">Editor</option>
                            <option value="admin">Admin</option>
                          </select>
                        </td>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          {tab === 'staff' && can('staff.manage') && <RoleLegend />}
        </>
      )}
    </div>
  );
}

/**
 * What each console role means, on the page where roles are granted.
 *
 * Granting permissions from a dropdown of four words is how someone ends up an
 * Admin because it sounded senior. The consequences belong next to the control,
 * not in a document nobody opens.
 */
function RoleLegend() {
  const roles: StaffRole[] = ['read_only', 'access', 'editor', 'admin'];
  return (
    <div className="card" style={{ marginTop: 18 }}>
      <div className="card-header">
        <div className="card-title">What each access level means</div>
      </div>
      <div className="card-body" style={{ display: 'grid', gap: 12 }}>
        {roles.map((r) => (
          <div key={r}>
            <div style={{ fontSize: 13, fontWeight: 800, color: 'var(--text)' }}>{ROLE_LABEL[r]}</div>
            <div style={{ fontSize: 12.5, color: 'var(--text-2)', marginTop: 2, lineHeight: 1.6 }}>
              {ROLE_DESCRIPTION[r]}
            </div>
          </div>
        ))}
        <div style={{
          fontSize: 12.5, color: 'var(--text-3)', lineHeight: 1.6,
          borderTop: '1px solid var(--border)', paddingTop: 12,
        }}>
          Nobody — including Admin — can view ID documents or full bank numbers
          through this console. Dangerous actions are logged as requests and need
          a second admin to approve them.
        </div>
      </div>
    </div>
  );
}