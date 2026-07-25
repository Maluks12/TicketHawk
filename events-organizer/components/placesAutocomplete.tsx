'use client';
// ── Google Places venue autocomplete ─────────────────────────────────────────
// Replaces three free-text location boxes (venue / street / city) with a single
// search that returns a REAL place, then fills the three fields from Google's
// own canonical data and captures place_id + lat/lng.
//
// Why this exists: locations used to be typed by hand and stored as text with
// no coordinates, and every client built its map link as a fuzzy text search
// (`maps/search/?api=1&query=<venue>, <city>`). Google then guessed, and often
// guessed wrong — a venue/city mismatch like "FNB Stadium, Sandton" (FNB is in
// Nasrec) could send buyers anywhere. With a place_id the clients can link to
// the EXACT place Google resolved.
//
// Loads the Places library lazily from the JS API, on first focus, so the
// script cost is only paid on the create/edit screens. Falls back to plain
// manual entry if the key is missing or the script fails — the organiser can
// always still type an address by hand.
import { useEffect, useRef, useState, useCallback } from 'react';
// Import the CSS prop type directly: with the modern JSX transform the
// global namespace isn't in scope, so qualifying it would fail to compile.
import type { CSSProperties } from 'react';

const MAPS_KEY = process.env.NEXT_PUBLIC_GOOGLE_MAPS_KEY || '';
const SCRIPT_ID = 'google-maps-places';

export interface PlaceResult {
  venueName: string;
  address: string;
  city: string;
  placeId: string;
  latitude: number;
  longitude: number;
  formatted: string;
}

/** Loads the Google Maps JS API (places library) exactly once per page. */
function loadPlaces(): Promise<void> {
  if (typeof window === 'undefined') return Promise.reject(new Error('no window'));
  const w = window as unknown as { google?: { maps?: { places?: unknown } }; __placesPromise?: Promise<void> };
  if (w.google?.maps?.places) return Promise.resolve();
  if (w.__placesPromise) return w.__placesPromise;
  if (!MAPS_KEY) return Promise.reject(new Error('NEXT_PUBLIC_GOOGLE_MAPS_KEY not set'));

  w.__placesPromise = new Promise<void>((resolve, reject) => {
    const existing = document.getElementById(SCRIPT_ID) as HTMLScriptElement | null;
    if (existing) { existing.addEventListener('load', () => resolve()); existing.addEventListener('error', () => reject(new Error('script error'))); return; }
    const el = document.createElement('script');
    el.id = SCRIPT_ID;
    el.async = true;
    el.defer = true;
    el.src = `https://maps.googleapis.com/maps/api/js?key=${encodeURIComponent(MAPS_KEY)}&libraries=places&loading=async`;
    el.onload = () => resolve();
    el.onerror = () => reject(new Error('Failed to load Google Maps'));
    document.head.appendChild(el);
  });
  return w.__placesPromise;
}

/** Pull a named component out of a Places address_components array. */
function comp(
  components: Array<{ long_name: string; short_name: string; types: string[] }> | undefined,
  ...types: string[]
): string {
  if (!components) return '';
  for (const t of types) {
    const hit = components.find((c) => c.types.includes(t));
    if (hit) return hit.long_name;
  }
  return '';
}

interface Props {
  /** Called when the organiser picks a real place. */
  onSelect: (p: PlaceResult) => void;
  /** Shown when a place is already chosen (edit mode / restored draft). */
  initialValue?: string;
  /** Bias results to a country (ISO-3166). Defaults to South Africa. */
  country?: string;
  style?: CSSProperties;
}

export default function PlacesAutocomplete({ onSelect, initialValue = '', country = 'za', style }: Props) {
  const inputRef = useRef<HTMLInputElement | null>(null);
  const acRef = useRef<unknown>(null);
  const [status, setStatus] = useState<'idle' | 'loading' | 'ready' | 'error'>('idle');
  const [errMsg, setErrMsg] = useState('');

  const attach = useCallback(() => {
    const w = window as unknown as {
      google?: { maps?: { places?: { Autocomplete: new (el: HTMLInputElement, opts: unknown) => unknown } } };
    };
    const Auto = w.google?.maps?.places?.Autocomplete;
    if (!Auto || !inputRef.current || acRef.current) return;

    const ac = new Auto(inputRef.current, {
      // 'establishment' + 'geocode' = named venues AND plain street addresses.
      types: ['establishment', 'geocode'],
      componentRestrictions: country ? { country } : undefined,
      fields: ['place_id', 'name', 'formatted_address', 'address_components', 'geometry'],
    }) as { addListener: (ev: string, cb: () => void) => void; getPlace: () => Record<string, unknown> };

    ac.addListener('place_changed', () => {
      const place = ac.getPlace() as {
        place_id?: string;
        name?: string;
        formatted_address?: string;
        address_components?: Array<{ long_name: string; short_name: string; types: string[] }>;
        geometry?: { location?: { lat: () => number; lng: () => number } };
      };
      // No geometry = the user pressed Enter on raw text rather than choosing a
      // suggestion. Ignore it; the manual fields below stay authoritative.
      if (!place?.place_id || !place.geometry?.location) return;

      const streetNo = comp(place.address_components, 'street_number');
      const route = comp(place.address_components, 'route');
      const street = [streetNo, route].filter(Boolean).join(' ');
      const city =
        comp(place.address_components, 'locality') ||
        comp(place.address_components, 'postal_town') ||
        comp(place.address_components, 'administrative_area_level_2') ||
        comp(place.address_components, 'administrative_area_level_1');

      onSelect({
        placeId: place.place_id,
        venueName: place.name ?? '',
        // Prefer the parsed street line; fall back to the full formatted string
        // so a rural/!addressed venue still gets something useful.
        address: street || (place.formatted_address ?? '').split(',')[0] || '',
        city,
        latitude: place.geometry.location.lat(),
        longitude: place.geometry.location.lng(),
        formatted: place.formatted_address ?? '',
      });
    });

    acRef.current = ac;
    setStatus('ready');
  }, [country, onSelect]);

  // Lazy-load on first focus so the script isn't fetched on every page view.
  const ensure = useCallback(() => {
    if (status === 'ready' || status === 'loading') return;
    setStatus('loading');
    loadPlaces()
      .then(() => attach())
      .catch((e: Error) => {
        setStatus('error');
        setErrMsg(
          e.message.includes('NEXT_PUBLIC_GOOGLE_MAPS_KEY')
            ? 'Address search is not configured — enter the venue details manually below.'
            : 'Address search is unavailable right now — enter the venue details manually below.',
        );
      });
  }, [status, attach]);

  useEffect(() => {
    return () => { acRef.current = null; };
  }, []);

  return (
    <div className="field">
      <label>
        Search for the venue{' '}
        <small style={{ color: 'var(--text-3)', fontWeight: 400 }}>
          (fills the fields below and pins the exact spot on maps)
        </small>
      </label>
      <input
        ref={inputRef}
        defaultValue={initialValue}
        onFocus={ensure}
        placeholder="Start typing a venue or address — e.g. FNB Stadium"
        style={style}
        autoComplete="off"
        // Browsers try to autofill address inputs and fight the dropdown.
        name="venue-search"
      />
      {status === 'loading' && (
        <small style={{ color: 'var(--text-3)' }}>Loading address search…</small>
      )}
      {status === 'error' && (
        <small style={{ color: '#c79249' }}>{errMsg}</small>
      )}
    </div>
  );
}