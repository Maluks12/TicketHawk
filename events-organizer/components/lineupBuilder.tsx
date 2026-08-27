'use client';
import { useEffect, useState } from 'react';
import { useToast } from '../lib/toast';
import LineupFields, { LineupLiveNotice } from './lineupFields';
import { getEventLineup, replaceEventLineup, type LineupEntry } from '../lib/eventContent';

/**
 * Line-up for an EXISTING event: loads by id, saves itself.
 *
 * All the editing UI lives in LineupFields, which the create wizard also uses
 * from local state. This wrapper is only load + save, so there is exactly one
 * line-up editor to keep correct rather than two that drift.
 */
export default function LineupBuilder({
  eventId,
  eventStart,
  isSport,
}: {
  eventId: string;
  eventStart?: string;
  isSport?: boolean;
}) {
  const toast = useToast();

  const [entries, setEntries] = useState<LineupEntry[]>([]);
  const [isPublished, setIsPublished] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await getEventLineup(eventId);
        if (cancelled) return;
        setEntries(res.entries);
        setIsPublished(res.is_published);
      } catch {
        if (!cancelled) toast('Could not load line-up', 'error');
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [eventId, toast]);

  async function save() {
    // Blank rows are dropped rather than rejected — a half-typed row somebody
    // abandoned is not a mistake worth blocking a save over.
    const cleaned = entries
      .map(e => ({ ...e, name: e.name.trim() }))
      .filter(e => e.name.length > 0)
      .map((e, i) => ({ ...e, sort_order: i }));

    setSaving(true);
    try {
      const res = await replaceEventLineup(
        eventId,
        cleaned.map(({ id: _id, ...rest }) => rest),
      );
      setEntries(res.entries);
      setDirty(false);
      toast(
        isPublished ? 'Line-up saved — ticket holders will be notified' : 'Line-up saved',
        'success',
      );
    } catch (e: unknown) {
      const err = e as { response?: { data?: { error?: string } } };
      toast(err.response?.data?.error ?? 'Could not save line-up', 'error');
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <div style={{ padding: 24 }}><div className="spinner" /></div>;

  return (
    <div>
      <LineupFields
        entries={entries}
        onChange={next => { setEntries(next); setDirty(true); }}
        eventStart={eventStart}
        isSport={isSport}
        notice={isPublished ? <LineupLiveNotice /> : null}
      />

      <div style={{ marginTop: 22, display: 'flex', gap: 10, alignItems: 'center' }}>
        <button type="button" className="btn btn-primary" disabled={!dirty || saving} onClick={save}>
          {saving ? 'Saving…' : 'Save line-up'}
        </button>
        {dirty && !saving && <span style={{ fontSize: 12, color: 'var(--text-3)' }}>Unsaved changes</span>}
      </div>
    </div>
  );
}