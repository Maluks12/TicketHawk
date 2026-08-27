'use client';
import { useEffect, useMemo, useState } from 'react';
import { useToast } from '../lib/toast';
import EventInfoFields from './eventInfoFields';
import {
  getInfoCatalogue, getEventInfo, replaceEventInfo,
  type InfoCatalogueEntry,
} from '../lib/eventContent';

/**
 * Information for an EXISTING event: loads by id, saves itself.
 *
 * The editing UI is in EventInfoFields, shared with the create wizard.
 */
export default function EventInfoEditor({ eventId }: { eventId: string }) {
  const toast = useToast();

  const [catalogue, setCatalogue] = useState<InfoCatalogueEntry[]>([]);
  const [chosen, setChosen] = useState<string[]>([]);
  const [values, setValues] = useState<Record<string, string>>({});
  const [original, setOriginal] = useState<Record<string, string>>({});
  const [isPublished, setIsPublished] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [cat, info] = await Promise.all([getInfoCatalogue(), getEventInfo(eventId)]);
        if (cancelled) return;
        setCatalogue(cat);
        setIsPublished(info.is_published);
        const v: Record<string, string> = {};
        info.items.forEach(i => { v[i.key] = i.value; });
        setValues(v);
        setOriginal(v);
        setChosen(info.items.map(i => i.key));
      } catch {
        if (!cancelled) toast('Could not load event information', 'error');
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [eventId, toast]);

  // Locked only when the event is LIVE. A draft has no buyers, so nothing on it
  // needs protecting from its own organiser.
  const lockedKeys = useMemo(
    () => (isPublished ? catalogue.filter(c => c.review_required_when_published).map(c => c.key) : []),
    [isPublished, catalogue],
  );

  const dirty = useMemo(() => {
    const keys = new Set([...Object.keys(values), ...Object.keys(original)]);
    for (const k of keys) {
      if ((values[k] ?? '').trim() !== (original[k] ?? '').trim()) return true;
    }
    return false;
  }, [values, original]);

  async function save() {
    setSaving(true);
    try {
      // Everything held is sent. Blank values are dropped server-side, which is
      // the mechanism that keeps unchosen fields row-less.
      const items = chosen.map(key => ({ key, value: values[key] ?? '' }));
      const res = await replaceEventInfo(eventId, items);
      const v: Record<string, string> = {};
      res.items.forEach(i => { v[i.key] = i.value; });
      setValues(v);
      setOriginal(v);
      setChosen(res.items.map(i => i.key));
      toast('Information saved', 'success');
    } catch (e: unknown) {
      const err = e as { response?: { status?: number; data?: { error?: string; code?: string } } };
      if (err.response?.status === 409 && err.response.data?.code === 'review_required') {
        // Not a validation failure — the organiser may make this change, just
        // not here. Say where, rather than showing a red field they cannot fix.
        toast(
          `${err.response.data.error ?? 'This field needs review'} — submit it as an edit request instead.`,
          'error',
        );
      } else {
        toast(err.response?.data?.error ?? 'Could not save information', 'error');
      }
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <div style={{ padding: 24 }}><div className="spinner" /></div>;

  return (
    <div>
      <EventInfoFields
        catalogue={catalogue}
        chosen={chosen}
        values={values}
        lockedKeys={lockedKeys}
        onChange={next => { setChosen(next.chosen); setValues(next.values); }}
      />

      <div style={{ marginTop: 22, display: 'flex', gap: 10, alignItems: 'center' }}>
        <button type="button" className="btn btn-primary" disabled={!dirty || saving} onClick={save}>
          {saving ? 'Saving…' : 'Save information'}
        </button>
        {dirty && !saving && <span style={{ fontSize: 12, color: 'var(--text-3)' }}>Unsaved changes</span>}
      </div>
    </div>
  );
}