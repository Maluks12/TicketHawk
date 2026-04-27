'use client';
import { useEffect, useState } from 'react';
import { api } from '../../../lib/api';
import { Badge } from '../../../lib/utils';
import { useToast } from '../../../lib/toast';
import { useConfirm } from '../../../lib/confirm';

interface User { id: string; email: string; first_name?: string; last_name?: string; role: string; phone_number?: string; phone?: string; }

export default function UsersPage() {
  const toast = useToast();
  const confirm = useConfirm();
  const [all, setAll] = useState<User[]>([]);
  const [filtered, setFiltered] = useState<User[]>([]);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState('');

  async function loadUsers() {
    setLoading(true);
    try {
      const { data } = await api.get('/admin/users?limit=100');
      const users = data.users || data || [];
      setAll(users); setFiltered(users);
    } catch { toast('Failed to load users', 'error'); }
    finally { setLoading(false); }
  }

  useEffect(() => { loadUsers(); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    const q = search.toLowerCase();
    if (!q) { setFiltered(all); return; }
    setFiltered(all.filter(u => u.email.toLowerCase().includes(q) || `${u.first_name || ''} ${u.last_name || ''}`.toLowerCase().includes(q)));
  }, [search, all]);

  function promote(id: string, email: string) {
    confirm('Promote User', `Grant admin privileges to ${email}?`, async () => {
      await api.put(`/admin/users/promote/${id}`);
      toast('User promoted to admin', 'success'); loadUsers();
    });
  }

  function demote(id: string, email: string) {
    confirm('Demote User', `Remove admin privileges from ${email}?`, async () => {
      await api.put(`/admin/users/demote/${id}`);
      toast('User demoted', 'success'); loadUsers();
    });
  }

  function deleteUser(id: string, email: string) {
    confirm('Delete User', `Permanently delete user "${email}"? This cannot be undone.`, async () => {
      await api.delete(`/admin/users/delete/${id}`);
      toast('User deleted', 'success'); loadUsers();
    });
  }

  return (
    <div style={{ padding: '1.5rem', maxWidth: '1400px', display: 'flex', flexDirection: 'column', flex: 1 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.5rem' }}>
        <div>
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: '1.1rem', fontWeight: 600 }}>Users</div>
          <div style={{ fontSize: '.8rem', color: 'var(--text-2)', marginTop: '.15rem' }}>Manage user roles and accounts</div>
        </div>
        <button className="btn btn-sm" onClick={loadUsers}>↺ Refresh</button>
      </div>

      <div className="table-wrap">
        <div className="table-toolbar">
          <input className="search-input" type="text" placeholder="Filter users…" value={search} onChange={e => setSearch(e.target.value)} />
          <div style={{ marginLeft: 'auto', fontFamily: 'var(--font-mono)', fontSize: '.72rem', color: 'var(--text-3)' }}>{filtered.length} users</div>
        </div>
        <div style={{ overflowX: 'auto', flex: 1 }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '.85rem', tableLayout: 'fixed' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid var(--border)' }}>
                {[['Name / Email','30%'],['Email','30%'],['Role','12%'],['Phone','15%'],['Actions','13%']].map(([h, w]) => (
                  <th key={h} style={{ padding: '.65rem 1rem', textAlign: 'left', fontFamily: 'var(--font-mono)', fontSize: '.65rem', textTransform: 'uppercase', letterSpacing: '.07em', color: 'var(--text-3)', fontWeight: 500, width: w }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={5} style={{ padding: '2rem', textAlign: 'center' }}><div className="spinner" style={{ margin: 'auto' }} /></td></tr>
              ) : filtered.length === 0 ? (
                <tr><td colSpan={5} style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-3)' }}>No users found.</td></tr>
              ) : filtered.map(u => {
                const displayName = (u.first_name || u.last_name) ? `${u.first_name || ''} ${u.last_name || ''}`.trim() : u.email;
                const phone = u.phone_number || u.phone || '—';
                return (
                  <tr key={u.id} style={{ borderBottom: '1px solid rgba(45,45,61,.5)' }}>
                    <td style={{ padding: '.65rem 1rem', fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{displayName}</td>
                    <td style={{ padding: '.65rem 1rem', color: 'var(--text-2)', fontSize: '.82rem', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{u.email}</td>
                    <td style={{ padding: '.65rem 1rem' }}><Badge status={u.role || 'attendee'} /></td>
                    <td style={{ padding: '.65rem 1rem', color: 'var(--text-2)', fontFamily: 'var(--font-mono)', fontSize: '.78rem' }}>{phone}</td>
                    <td style={{ padding: '.65rem 1rem', overflow: 'visible', whiteSpace: 'nowrap' }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '.3rem' }}>
                        {u.role === 'admin'
                          ? <button className="btn btn-sm" onClick={() => demote(u.id, u.email)}>Demote</button>
                          : <button className="btn btn-success btn-sm" onClick={() => promote(u.id, u.email)}>Promote</button>}
                        <button className="btn-icon" onClick={() => deleteUser(u.id, u.email)} style={{ color: 'var(--danger)' }} title="Delete user">✕</button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}