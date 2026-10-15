'use client';
import { useCallback, useEffect, useRef, useState } from 'react';
import ImageCropper from '../../../components/imageCropper';
// Only icons this project already imports elsewhere — node_modules is not
// available to verify anything beyond that list, and a wrong name renders as a
// blank box. Arrows and pluses are plain characters for the same reason.
import {
  X, Warning, ArrowSquareOut, ImageSquare,
} from '@phosphor-icons/react';
import { useToast } from '../../../lib/toast';
import {
  getMyPublicProfile, updateMyPublicProfile, replaceMyLinks,
  getLinkPlatforms, uploadCover, uploadLogo,
  type OwnerProfile, type LinkPlatform,
} from '../../../lib/organiserProfile';

/**
 * Organiser public page editor.
 *
 * This is the page attendees reach from the ORGANISED BY row on any of the
 * organiser's events, and from search. It carries their name, bio, cover,
 * socials and their whole catalogue.
 *
 * ── Why the socials live here and not on the event page ─────────────────────
 *
 * An event page is about one event, and a row of outbound links on it competes
 * with the only action that matters there — buying a ticket. Someone who wants
 * to know who is behind the event has already tapped through to this page,
 * where a link off the platform costs nothing.
 */

const PLATFORM_LABEL: Record<string, string> = {
  instagram: 'Instagram', x: 'X', facebook: 'Facebook', tiktok: 'TikTok',
  youtube: 'YouTube', linkedin: 'LinkedIn', whatsapp: 'WhatsApp',
  spotify: 'Spotify', website: 'Website',
};

const PLATFORM_PLACEHOLDER: Record<string, string> = {
  instagram: '@yourhandle  or  https://instagram.com/yourhandle',
  x:         '@yourhandle',
  facebook:  '@yourpage',
  tiktok:    '@yourhandle',
  youtube:   '@yourchannel',
  linkedin:  'your-company',
  whatsapp:  'https://wa.me/27821234567',
  spotify:   'https://open.spotify.com/artist/…',
  website:   'yourdomain.co.za',
};

// ── Cover crop ratio ─────────────────────────────────────────────────────────
//
// 3:1, and a compromise on purpose — the three surfaces that render this cover
// disagree: web-desktop shows a 220px band across the full width (~6.5:1 on a
// laptop), web-mobile 150px across a phone (~2.6:1), EventsApp 0.42 x screen
// width (~2.4:1). No single ratio matches all three. 3:1 keeps nearly all of the
// crop on phones and the middle band on desktop, which is why the hint tells the
// organiser to keep anything important in the centre.
const CROP_PROFILE_COVER = 3;

// ── Unsaved-edit draft ───────────────────────────────────────────────────────
//
// Description, visibility and links are only written on Save, so anything typed
// and not saved used to vanish on reload or on leaving the page. The unsaved
// form is mirrored to localStorage (per organiser) and restored on return.
// Local only — nothing reaches the public page until Save is pressed.
interface PageDraft {
  description: string;
  enabled: boolean;
  links: { platform: string; url: string }[];
}
const draftKey = (organiserId: string) => `org_page_draft:${organiserId}`;

function readDraft(organiserId: string): PageDraft | null {
  try {
    const raw = localStorage.getItem(draftKey(organiserId));
    return raw ? JSON.parse(raw) as PageDraft : null;
  } catch { return null; }
}
function writeDraft(organiserId: string, d: PageDraft | null) {
  try {
    if (d) localStorage.setItem(draftKey(organiserId), JSON.stringify(d));
    else localStorage.removeItem(draftKey(organiserId));
  } catch { /* storage blocked or full — the form still works, just unsaved */ }
}

const linksOf = (p: OwnerProfile) =>
  (p.links ?? []).map(l => ({ platform: l.platform as string, url: l.url }));
const sameLinks = (a: { platform: string; url: string }[], b: { platform: string; url: string }[]) =>
  JSON.stringify(a) === JSON.stringify(b);

export default function OrganiserPageEditor() {
  const toast = useToast();
  const coverInput = useRef<HTMLInputElement | null>(null);
  const logoInput = useRef<HTMLInputElement | null>(null);

  const [profile, setProfile] = useState<OwnerProfile | null>(null);
  const [platforms, setPlatforms] = useState<LinkPlatform[]>([]);
  const [loading, setLoading] = useState(true);

  const [description, setDescription] = useState('');
  const [enabled, setEnabled] = useState(true);
  const [links, setLinks] = useState<{ platform: string; url: string }[]>([]);

  const [savingProfile, setSavingProfile] = useState(false);
  const [savingLinks, setSavingLinks] = useState(false);
  const [uploading, setUploading] = useState<'cover' | 'logo' | null>(null);
  // The picked cover file, waiting in the cropper. Null = cropper closed.
  const [croppingCover, setCroppingCover] = useState<File | null>(null);
  // True when the form was filled from a local draft rather than the server.
  const [restoredDraft, setRestoredDraft] = useState(false);

  // Runs ONCE on mount. It is the only thing that fills the form fields from
  // the server — an image upload used to call it again, which overwrote any
  // unsaved description, links or visibility change with the stored values.
  const load = useCallback(async () => {
    try {
      const [p, pf] = await Promise.all([getMyPublicProfile(), getLinkPlatforms()]);
      setProfile(p);
      setPlatforms(pf);

      const server: PageDraft = {
        description: p.description ?? '',
        enabled: p.public_profile_enabled,
        links: linksOf(p),
      };
      const draft = readDraft(p.id);
      const differs = !!draft && (
        draft.description !== server.description
        || draft.enabled !== server.enabled
        || !sameLinks(draft.links ?? [], server.links)
      );
      const use = differs && draft ? { ...server, ...draft, links: draft.links ?? server.links } : server;
      if (!differs) writeDraft(p.id, null);
      setRestoredDraft(differs);

      setDescription(use.description);
      setEnabled(use.enabled);
      setLinks(use.links);
    } catch {
      toast('Could not load your public page', 'error');
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => { load(); }, [load]);

  // ── Dirty tracking ────────────────────────────────────────────────────────
  // Measured against `profile`, which is always the last thing the server
  // confirmed — so a successful save clears it without extra bookkeeping.
  const aboutDirty = !!profile && (
    description !== (profile.description ?? '') || enabled !== profile.public_profile_enabled
  );
  const linksDirty = !!profile && !sameLinks(links, linksOf(profile));
  const dirty = aboutDirty || linksDirty;

  // Mirror unsaved edits to storage. Writing to an external store is what an
  // effect is for; no React state is set here.
  const organiserId = profile?.id;
  useEffect(() => {
    if (!organiserId) return;
    writeDraft(organiserId, dirty ? { description, enabled, links } : null);
  }, [organiserId, dirty, description, enabled, links]);

  // A browser reload or closing the tab with unsaved edits asks first. The
  // draft above already survives it — this is so the organiser knows the
  // change is not live yet, not only that it was kept.
  useEffect(() => {
    if (!dirty) return;
    const onBeforeUnload = (e: BeforeUnloadEvent) => { e.preventDefault(); };
    window.addEventListener('beforeunload', onBeforeUnload);
    return () => window.removeEventListener('beforeunload', onBeforeUnload);
  }, [dirty]);

  function discardDraft() {
    if (!profile) return;
    setDescription(profile.description ?? '');
    setEnabled(profile.public_profile_enabled);
    setLinks(linksOf(profile));
    setRestoredDraft(false);
    writeDraft(profile.id, null);
  }

  async function saveProfile() {
    setSavingProfile(true);
    try {
      const updated = await updateMyPublicProfile({
        description,
        public_profile_enabled: enabled,
      });
      setProfile(updated);
      setRestoredDraft(false);
      toast('Page saved', 'success');
    } catch (e: unknown) {
      const err = e as { response?: { data?: { error?: string } } };
      toast(err.response?.data?.error ?? 'Could not save', 'error');
    } finally {
      setSavingProfile(false);
    }
  }

  async function saveLinks() {
    setSavingLinks(true);
    try {
      // Blank rows are dropped rather than sent — a half-filled row somebody
      // added and abandoned should not fail the whole save.
      const payload = links
        .map(l => ({ platform: l.platform, url: l.url.trim() }))
        .filter(l => l.url.length > 0);

      const res = await replaceMyLinks(payload);
      // Take the SERVER's version back. It normalises "@handle" into a full
      // URL, so showing what was typed would make the field look unsaved.
      setLinks(res.links.map(l => ({ platform: l.platform, url: l.url })));
      // Move the saved baseline too, or the form would read as still dirty.
      setProfile(p => (p ? { ...p, links: res.links } : p));
      setRestoredDraft(false);
      toast('Links saved', 'success');
    } catch (e: unknown) {
      const err = e as { response?: { data?: { error?: string } } };
      // The server names the offending platform and why — "instagram: link does
      // not point at that platform". That is fixable by the reader; a generic
      // message is not.
      toast(err.response?.data?.error ?? 'Could not save links', 'error');
    } finally {
      setSavingLinks(false);
    }
  }

  async function onImage(kind: 'cover' | 'logo', file: File | null) {
    if (!file) return;
    setUploading(kind);
    try {
      const res = kind === 'cover' ? await uploadCover(file) : await uploadLogo(file);
      if (!res.saved) {
        toast(res.error ?? 'Image uploaded but not saved — try again', 'error');
      } else {
        // Patch ONLY the image on the profile. Reloading the whole profile here
        // is what used to wipe an unsaved description and links.
        setProfile(p => (p ? {
          ...p,
          ...(kind === 'cover' ? { cover_image_url: res.url } : { logo_url: res.url }),
        } : p));
        toast(kind === 'cover' ? 'Cover updated' : 'Logo updated', 'success');
      }
    } catch {
      toast('Upload failed', 'error');
    } finally {
      setUploading(null);
    }
  }

  const usedPlatforms = links.map(l => l.platform);
  const availablePlatforms = platforms.filter(p => !usedPlatforms.includes(p));

  if (loading) return <div className="page-loader"><div className="spinner spinner-lg" /></div>;

  if (!profile) {
    return (
      <div style={{ padding: 24 }}>
        <div className="alert alert-danger">
          Could not load your public page. Refresh to try again.
        </div>
      </div>
    );
  }

  return (
    <div style={{ padding: '1.5rem', maxWidth: 1080 }}>
      <input ref={coverInput} type="file" accept="image/jpeg,image/png,image/webp"
        style={{ display: 'none' }}
        onChange={e => {
          // Cover goes through the cropper first; the cropped file is what
          // gets uploaded.
          const f = e.target.files?.[0] ?? null;
          e.target.value = '';
          if (f) setCroppingCover(f);
        }} />

      {croppingCover && (
        <ImageCropper
          file={croppingCover}
          aspect={CROP_PROFILE_COVER}
          title="Crop your cover"
          hint="Drag to reposition, scroll to zoom. Wide screens show the middle band, so keep text and faces centred."
          maxOutputWidth={2400}
          onCancel={() => setCroppingCover(null)}
          onConfirm={f => { setCroppingCover(null); onImage('cover', f); }}
        />
      )}
      <input ref={logoInput} type="file" accept="image/jpeg,image/png,image/webp"
        style={{ display: 'none' }}
        onChange={e => { onImage('logo', e.target.files?.[0] ?? null); e.target.value = ''; }} />

      {/* ── Header ───────────────────────────────────────────────────────── */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16, marginBottom: 18 }}>
        <div>
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: '1.1rem', fontWeight: 600 }}>
            Your public page
          </div>
          <div style={{ fontSize: '.8rem', color: 'var(--text-2)', marginTop: 2, maxWidth: 560 }}>
            What attendees see when they tap your name on one of your events, or
            find you in search.
          </div>
        </div>
        {profile.is_live && (
          <a className="btn btn-sm btn-ghost" target="_blank" rel="noreferrer"
            href={`${(process.env.NEXT_PUBLIC_WEB_URL ?? '')}${profile.public_url_path}`}>
            <ArrowSquareOut size={13} weight="bold" /> View live page
          </a>
        )}
      </div>

      {/* ── Not-live warning only ────────────────────────────────────────
          The green "Live at …" banner is gone.

          It restated three things already visible or better placed elsewhere:
          the URL is in the header link, the event count is on My Events, and
          the follower count belongs on Analytics next to the other numbers
          somebody actually compares over time. A permanent green bar saying
          everything is fine is a bar nobody reads after the first week.

          The WARNING stays, because it is the one state that needs acting on
          and is not visible anywhere else. */}
      {!profile.is_live && (
        <div className="alert alert-warning" style={{ marginBottom: 18 }}>
          <Warning size={16} weight="fill" />
          <div>
              {profile.blocked_reason === 'kyc_level_required' && (
                <>Your page is not visible yet. Complete identity verification to
                publish it — you can build it here in the meantime.</>
              )}
              {profile.blocked_reason === 'disabled_by_organiser' && (
                <>Your page is switched off. Turn it back on below to make it
                visible.</>
              )}
              {profile.blocked_reason === 'account_suspended' && (
                <>Your account is suspended, so your page is hidden. Contact
                support.</>
              )}
            {!profile.blocked_reason && <>Your page is not currently visible.</>}
          </div>
        </div>
      )}

      {/* No preview tab.
          It framed the public organiser page in an iframe, and that page does
          not exist yet in either web app — so it only ever rendered a 404 as a
          blank white rectangle. View live page in the header goes to the real
          thing once it does exist. */}
      {restoredDraft && dirty && (
        <div className="alert alert-info" style={{ marginBottom: 18, alignItems: 'center' }}>
          <Warning size={16} weight="fill" />
          <div style={{ flex: 1 }}>
            Restored the unsaved changes from your last visit. They are not live
            until you press Save.
          </div>
          <button type="button" className="btn btn-sm" onClick={discardDraft}>
            Discard changes
          </button>
        </div>
      )}

      <div style={{ display: 'grid', gap: 18 }}>

          {/* ── Images ───────────────────────────────────────────────────── */}
          <section className="card">
            <div className="card-header"><div className="card-title">Images</div></div>
            <div className="card-body" style={{ display: 'grid', gap: 16 }}>
              <div>
                <label className="field-hint" style={{ display: 'block', marginBottom: 6 }}>
                  Cover — the wide image across the top of your page (cropped to 3:1)
                </label>
                <button type="button" onClick={() => coverInput.current?.click()}
                  disabled={uploading !== null}
                  style={{
                    width: '100%', height: 150, borderRadius: 'var(--r-lg)',
                    border: '1px dashed var(--border)', background: 'var(--surface-2)',
                    cursor: 'pointer', overflow: 'hidden', padding: 0,
                    display: 'grid', placeItems: 'center',
                  }}>
                  {profile.cover_image_url
                    // eslint-disable-next-line @next/next/no-img-element
                    ? <img src={profile.cover_image_url} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
                    : <span style={{ color: 'var(--text-3)', fontSize: 13 }}>
                        {uploading === 'cover' ? 'Uploading…' : 'Click to upload a cover'}
                      </span>}
                </button>
              </div>

              <div>
                <label className="field-hint" style={{ display: 'block', marginBottom: 6 }}>
                  Logo — also shown on every one of your event pages
                </label>
                <button type="button" onClick={() => logoInput.current?.click()}
                  disabled={uploading !== null}
                  style={{
                    width: 92, height: 92, borderRadius: 18,
                    border: '1px dashed var(--border)', background: 'var(--surface-2)',
                    cursor: 'pointer', overflow: 'hidden', padding: 0,
                    display: 'grid', placeItems: 'center',
                  }}>
                  {profile.logo_url
                    // eslint-disable-next-line @next/next/no-img-element
                    ? <img src={profile.logo_url} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
                    : uploading === 'logo'
                      ? <span style={{ fontSize: 11, color: 'var(--text-3)' }}>…</span>
                      : <ImageSquare size={22} color="var(--text-3)" />}
                </button>
              </div>
            </div>
          </section>

          {/* ── About ────────────────────────────────────────────────────── */}
          <section className="card">
            <div className="card-header"><div className="card-title">About</div></div>
            <div className="card-body">
              <div className="field">
                <label>Business name</label>
                <input value={profile.business_name} disabled />
                <small className="field-hint">
                  Changing your business name is an account change, not a page
                  edit — it is on the account settings screen. Your page URL is
                  fixed at <code>{profile.public_url_path}</code> and does not
                  follow a rename, so links already shared keep working.
                </small>
              </div>

              <div className="field">
                <label>Description</label>
                <textarea rows={4} maxLength={2000} value={description}
                  onChange={e => setDescription(e.target.value)}
                  placeholder="Who you are and what you run. Two or three sentences is plenty." />
                <small className="field-hint">{description.length} / 2000</small>
              </div>

              <div className="field">
                <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', fontWeight: 400 }}>
                  <input type="checkbox" checked={enabled} style={{ width: 'auto' }}
                    onChange={e => setEnabled(e.target.checked)} />
                  <span style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                    
                    Page is visible to the public
                  </span>
                </label>
                <small className="field-hint">
                  Switching this off hides your page and removes the organiser
                  block from your event pages — those events stay on sale.
                </small>
              </div>

              <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                <button type="button" className="btn btn-primary" disabled={savingProfile} onClick={saveProfile}>
                  {savingProfile ? 'Saving…' : 'Save'}
                </button>
                {aboutDirty && (
                  <span style={{ fontSize: 12.5, color: 'var(--warning-fg)' }}>Unsaved changes</span>
                )}
              </div>
            </div>
          </section>

          {/* ── Links ────────────────────────────────────────────────────── */}
          <section className="card">
            <div className="card-header">
              <div className="card-title">Links</div>
            </div>
            <div className="card-body">
              <div className="field-hint" style={{ marginBottom: 12 }}>
                Shown as icons on your page. You can type a handle —
                <code> @yourname </code>— and we will turn it into the full link.
                A link labelled Instagram has to point at Instagram; anything
                else is rejected, which is what stops a page carrying our
                branding from sending people somewhere unexpected.
              </div>

              {links.length === 0 && (
                <div style={{ fontSize: 13, color: 'var(--text-3)', padding: '8px 0 14px' }}>
                  No links yet.
                </div>
              )}

              <div style={{ display: 'grid', gap: 10 }}>
                {links.map((l, i) => (
                  <div key={`${l.platform}-${i}`} className="link-row" style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                    <div className="link-row-label" style={{
                      width: 104, flexShrink: 0, fontSize: 12.5, fontWeight: 600,
                      color: 'var(--text-2)',
                    }}>
                      {PLATFORM_LABEL[l.platform] ?? l.platform}
                    </div>
                    <input
                      className="input"
                      style={{ flex: 1, minWidth: 0, padding: '7px 10px', fontSize: 13 }}
                      value={l.url}
                      placeholder={PLATFORM_PLACEHOLDER[l.platform]}
                      onChange={e => setLinks(ls => ls.map((x, j) => j === i ? { ...x, url: e.target.value } : x))}
                    />
                    <button type="button" className="btn btn-xs" disabled={i === 0}
                      onClick={() => setLinks(ls => {
                        const n = [...ls]; [n[i - 1], n[i]] = [n[i], n[i - 1]]; return n;
                      })}>
                      ↑
                    </button>
                    <button type="button" className="btn btn-xs" disabled={i === links.length - 1}
                      onClick={() => setLinks(ls => {
                        const n = [...ls]; [n[i + 1], n[i]] = [n[i], n[i + 1]]; return n;
                      })}>
                      ↓
                    </button>
                    <button type="button" className="btn btn-xs btn-danger"
                      onClick={() => setLinks(ls => ls.filter((_, j) => j !== i))}>
                      <X size={12} weight="bold" />
                    </button>
                  </div>
                ))}
              </div>

              {availablePlatforms.length > 0 && (
                <div style={{ marginTop: 14, display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                  {availablePlatforms.map(p => (
                    <button key={p} type="button" className="btn btn-xs"
                      onClick={() => setLinks(ls => [...ls, { platform: p, url: '' }])}>
                      + {PLATFORM_LABEL[p] ?? p}
                    </button>
                  ))}
                </div>
              )}

              <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginTop: 18 }}>
                <button type="button" className="btn btn-primary"
                  disabled={savingLinks} onClick={saveLinks}>
                  {savingLinks ? 'Saving…' : 'Save links'}
                </button>
                {linksDirty && (
                  <span style={{ fontSize: 12.5, color: 'var(--warning-fg)' }}>Unsaved changes</span>
                )}
              </div>
            </div>
          </section>
      </div>
    </div>
  );
}