'use client';
import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { api } from '../../../../lib/api';
import { useToast } from '../../../../lib/toast';
import { validateRequired, validateMinLength, firstError } from '../../../../lib/validation';

interface Category { id: number; name: string; slug: string; }

export default function CreateEventPage() {
  const router = useRouter();
  const toast = useToast();
  const [categories, setCategories] = useState<Category[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});

  // Form fields
  const [title,       setTitle]       = useState('');
  const [description, setDescription] = useState('');
  const [categoryId,  setCategoryId]  = useState('');
  const [venueName,   setVenueName]   = useState('');
  const [address,     setAddress]     = useState('');
  const [city,        setCity]        = useState('');
  const [timezone,    setTimezone]    = useState('Africa/Johannesburg');
  const [startTime,   setStartTime]   = useState('');
  const [endTime,     setEndTime]     = useState('');
  const [expiresAt,   setExpiresAt]   = useState('');
  const [price,       setPrice]       = useState('0');
  const [currency,    setCurrency]    = useState('ZAR');
  const [maxCapacity,      setMaxCapacity]      = useState('');
  const [maxTicketsPerUser, setMaxTicketsPerUser] = useState('');
  const [venueType,         setVenueType]         = useState<'auto'|'stadium'|'auditorium'|'none'>('auto');
  const [isFree,      setIsFree]      = useState(true);

  useEffect(() => {
    api.get('/v1/categories').then(r => setCategories(r.data.categories ?? r.data ?? [])).catch(() => {});
  }, []);

  function validate() {
    return firstError({
      title:       validateMinLength(title, 5, 'Title'),
      description: validateMinLength(description, 50, 'Description'),
      category:    validateRequired(categoryId, 'Category'),
      venueName:   validateRequired(venueName, 'Venue name'),
      address:     validateRequired(address, 'Address'),
      city:        validateRequired(city, 'City'),
      startTime:   validateRequired(startTime, 'Start time'),
      endTime:     validateRequired(endTime, 'End time'),
      expiresAt:   validateRequired(expiresAt, 'Booking deadline'),
    });
  }

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const err = validate();
    if (err) { toast(err, 'error'); return; }

    if (new Date(endTime) <= new Date(startTime)) {
      toast('End time must be after start time', 'error'); return;
    }

    setSubmitting(true);
    try {
      const payload = {
        title:        title.trim(),
        description:  description.trim(),
        category_id:  parseInt(categoryId),
        venue_name:   venueName.trim(),
        address:      address.trim(),
        city:         city.trim(),
        timezone,
        start_time:   new Date(startTime).toISOString(),
        end_time:     new Date(endTime).toISOString(),
        expires_at:   new Date(expiresAt).toISOString(),
        price:        isFree ? 0 : parseFloat(price) || 0,
        currency,
        max_capacity: maxCapacity ? parseInt(maxCapacity) : undefined,
        max_tickets_per_user: maxTicketsPerUser ? parseInt(maxTicketsPerUser) : undefined,
        venue_type: venueType !== 'auto' ? venueType : undefined,
      };
      await api.post('/organiser/events/create', payload);
      toast('Event created as draft — pending admin approval', 'success');
      router.push('/events');
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string; details?: string } } })?.response?.data?.error
        ?? 'Failed to create event';
      toast(msg, 'error');
    } finally { setSubmitting(false); }
  }

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <div style={{ fontSize: 12, color: '#606770', marginBottom: 4 }}>
            <span style={{ cursor: 'pointer', color: '#606770' }} onClick={() => router.push('/events')}>My Events</span>
            {' → '}<span>Create event</span>
          </div>
          <div className="page-title">Create a new event</div>
          <div style={{ fontSize: 13, color: '#606770', marginTop: 2 }}>
            Events are created as drafts and require admin approval before going live.
          </div>
        </div>
      </div>

      <div className="page-content">
        <form onSubmit={handleSubmit} noValidate>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 320px', gap: 20, maxWidth: 960 }}>

            {/* Left: main form */}
            <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>

              {/* Basic info */}
              <FormCard title="Event details" icon="📋">
                <div className="field">
                  <label>Event title <Required /></label>
                  <input value={title} onChange={e => setTitle(e.target.value)}
                    placeholder="e.g. Thetsongaboys Summer Festival 2025"
                    style={inputStyle} />
                  <div className="field-hint">{title.length}/255 · minimum 5 characters</div>
                </div>
                <div className="field" style={{ marginBottom: 0 }}>
                  <label>Description <Required /></label>
                  <textarea value={description} onChange={e => setDescription(e.target.value)}
                    placeholder="Describe your event in detail — what attendees can expect, lineup, dress code, etc. (minimum 50 characters)"
                    rows={5}
                    style={{ ...inputStyle, resize: 'vertical' }} />
                  <div className="field-hint">{description.length}/∞ · minimum 50 characters</div>
                </div>
              </FormCard>

              {/* Date & time */}
              <FormCard title="Date & time" icon="📅">
                <div className="field">
                  <label>Timezone</label>
                  <select value={timezone} onChange={e => setTimezone(e.target.value)} style={inputStyle}>
                    <option value="Africa/Johannesburg">Africa/Johannesburg (SAST)</option>
                    <option value="Africa/Cape_Town">Africa/Cape_Town</option>
                    <option value="UTC">UTC</option>
                  </select>
                </div>
                <div className="field-row">
                  <div className="field">
                    <label>Start time <Required /></label>
                    <input type="datetime-local" value={startTime} onChange={e => setStartTime(e.target.value)} style={inputStyle} />
                  </div>
                  <div className="field">
                    <label>End time <Required /></label>
                    <input type="datetime-local" value={endTime} onChange={e => setEndTime(e.target.value)} style={inputStyle} />
                  </div>
                </div>
                <div className="field" style={{ marginBottom: 0 }}>
                  <label>Booking deadline <Required /></label>
                  <input type="datetime-local" value={expiresAt} onChange={e => setExpiresAt(e.target.value)} style={inputStyle} />
                  <div className="field-hint">Tickets stop selling after this date</div>
                </div>
              </FormCard>

              {/* Location */}
              <FormCard title="Location" icon="📍">
                <div className="field">
                  <label>Venue name <Required /></label>
                  <input value={venueName} onChange={e => setVenueName(e.target.value)}
                    placeholder="e.g. The Venue at Melrose Arch" style={inputStyle} />
                </div>
                <div className="field">
                  <label>Address <Required /></label>
                  <input value={address} onChange={e => setAddress(e.target.value)}
                    placeholder="Street address" style={inputStyle} />
                </div>
                <div className="field" style={{ marginBottom: 0 }}>
                  <label>City <Required /></label>
                  <input value={city} onChange={e => setCity(e.target.value)}
                    placeholder="e.g. Johannesburg" style={inputStyle} />
                </div>
              </FormCard>

              {/* Tickets & pricing */}
              <FormCard title="Tickets & pricing" icon="🎟">
                <div className="field">
                  <label>Category <Required /></label>
                  <select value={categoryId} onChange={e => setCategoryId(e.target.value)} style={inputStyle}>
                    <option value="">— Select a category —</option>
                    {categories.map(c => (
                      <option key={c.id} value={c.id}>{c.name}</option>
                    ))}
                  </select>
                </div>

                <div style={{ display: 'flex', gap: 12, marginBottom: 12 }}>
                  {[true, false].map(free => (
                    <label key={String(free)} style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '10px 14px', border: `2px solid ${isFree === free ? '#1877f2' : '#dddfe2'}`, borderRadius: 6, background: isFree === free ? '#e7f3ff' : '#fff', cursor: 'pointer', flex: 1 }}>
                      <input type="radio" name="pricing" checked={isFree === free} onChange={() => setIsFree(free)} style={{ accentColor: '#1877f2' }} />
                      <div>
                        <div style={{ fontWeight: 600, fontSize: 14 }}>{free ? 'Free event' : 'Paid event'}</div>
                        <div style={{ fontSize: 12, color: '#606770' }}>{free ? 'No ticket charge' : 'Set a ticket price'}</div>
                      </div>
                    </label>
                  ))}
                </div>

                {!isFree && (
                  <div className="field-row">
                    <div className="field">
                      <label>Ticket price (ZAR)</label>
                      <div style={{ position: 'relative' }}>
                        <span style={{ position: 'absolute', left: 12, top: '50%', transform: 'translateY(-50%)', color: '#606770', fontWeight: 600 }}>R</span>
                        <input type="number" value={price} onChange={e => setPrice(e.target.value)} min="0" style={{ ...inputStyle, paddingLeft: 28 }} />
                      </div>
                    </div>
                    <div className="field">
                      <label>Currency</label>
                      <select value={currency} onChange={e => setCurrency(e.target.value)} style={inputStyle}>
                        <option value="ZAR">ZAR — South African Rand</option>
                        <option value="USD">USD — US Dollar</option>
                        <option value="GBP">GBP — British Pound</option>
                      </select>
                    </div>
                  </div>
                )}

                <div className="field" style={{ marginBottom: 0 }}>
                  <label>Maximum capacity (optional)</label>
                  <input type="number" value={maxCapacity} onChange={e => setMaxCapacity(e.target.value)}
                    placeholder="Leave blank for unlimited tickets" min="1" style={inputStyle} />
                </div>
              </FormCard>
            </div>

                <div className="field">
                  <label>Max Tickets Per User <small>(blank = no limit)</small></label>
                  <input type="number" min="1" max="20" value={maxTicketsPerUser}
                    onChange={e => setMaxTicketsPerUser(e.target.value)} placeholder="No limit" />
                </div>

                <div className="field">
                  <label>Venue Type</label>
                  <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginTop: 6 }}>
                    {([
                      { value: "auto",       label: "⚡ Auto-detect" },
                      { value: "stadium",    label: "🏟️ Stadium" },
                      { value: "auditorium", label: "🎭 Hall / Theatre" },
                      { value: "none",       label: "🎫 None (skip seat map)" },
                    ] as const).map(opt => (
                      <button key={opt.value} type="button"
                        onClick={() => setVenueType(opt.value)}
                        style={{
                          padding: "6px 12px", borderRadius: 6, cursor: "pointer", fontSize: 13,
                          fontWeight: venueType === opt.value ? 600 : 400,
                          border: venueType === opt.value ? "2px solid var(--blue)" : "1px solid var(--border)",
                          background: venueType === opt.value ? "var(--blue-light, #e8f0fe)" : "transparent",
                          color: venueType === opt.value ? "var(--blue, #1877f2)" : "inherit",
                        }}>
                        {opt.label}
                      </button>
                    ))}
                  </div>
                </div>

            {/* Right: help sidebar */}
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <div style={{ background: '#fff', border: '1px solid #dddfe2', borderRadius: 8, padding: 16, position: 'sticky', top: 0 }}>
                <div style={{ fontSize: 14, fontWeight: 700, marginBottom: 12 }}>Before you submit</div>
                {[
                  { done: title.length >= 5, label: 'Add a title (5+ chars)' },
                  { done: description.length >= 50, label: 'Write a description (50+ chars)' },
                  { done: !!categoryId, label: 'Choose a category' },
                  { done: !!venueName && !!city, label: 'Set venue and city' },
                  { done: !!startTime && !!endTime, label: 'Set start and end times' },
                  { done: !!expiresAt, label: 'Set a booking deadline' },
                ].map(item => (
                  <div key={item.label} style={{ display: 'flex', gap: 8, alignItems: 'flex-start', marginBottom: 8 }}>
                    <span style={{ width: 16, height: 16, borderRadius: '50%', background: item.done ? '#1a7f37' : '#ebedf0', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 10, color: item.done ? '#fff' : '#8a8d91', flexShrink: 0, marginTop: 1 }}>
                      {item.done ? '✓' : '·'}
                    </span>
                    <span style={{ fontSize: 13, color: item.done ? '#8a8d91' : '#1c1e21', textDecoration: item.done ? 'line-through' : 'none' }}>
                      {item.label}
                    </span>
                  </div>
                ))}
                <div style={{ height: 1, background: '#ebedf0', margin: '12px 0' }} />
                <div className="alert alert-info" style={{ fontSize: 12, padding: '8px 10px' }}>
                  <svg viewBox="0 0 20 20" fill="currentColor" width="14" height="14" style={{ flexShrink: 0 }}>
                    <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd"/>
                  </svg>
                  <span>Events need admin approval before going live. You'll be notified once approved.</span>
                </div>
              </div>
            </div>
          </div>

          {/* Footer buttons */}
          <div style={{ display: 'flex', gap: 8, marginTop: 20, paddingTop: 20, borderTop: '1px solid #dddfe2', maxWidth: 640 }}>
            <button type="button" className="btn" onClick={() => router.push('/events')}>Cancel</button>
            <button type="submit" className="btn btn-primary" disabled={submitting}>
              {submitting ? 'Submitting…' : 'Submit for review'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

const inputStyle: React.CSSProperties = {
  width: '100%', padding: '9px 12px', border: '1px solid #ccd0d5',
  borderRadius: 6, fontSize: 14, color: '#1c1e21', background: '#fff',
  outline: 'none', fontFamily: 'inherit', boxSizing: 'border-box',
};

function Required() {
  return <span style={{ color: '#c41c1c', marginLeft: 2 }}>*</span>;
}

function FormCard({ title, icon, children }: { title: string; icon: string; children: React.ReactNode }) {
  return (
    <div style={{ background: '#fff', border: '1px solid #dddfe2', borderRadius: 8, overflow: 'hidden' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '12px 16px', borderBottom: '1px solid #ebedf0', background: '#fafafa' }}>
        <span style={{ fontSize: 16 }}>{icon}</span>
        <span style={{ fontSize: 15, fontWeight: 700, color: '#1c1e21' }}>{title}</span>
      </div>
      <div style={{ padding: 16 }}>{children}</div>
    </div>
  );
}