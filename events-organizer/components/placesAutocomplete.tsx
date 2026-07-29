'use client';
// ── Google Places venue autocomplete ─────────────────────────────────────────
// A single search box that behaves like the Google Maps search bar: type
// anything — a street number, a road, a suburb, a venue name — and matching
// places appear in a dropdown as you type. Picking one fills the venue/street/
// city fields below from Google's canonical data and captures place_id +
// lat/lng.
//
// Why place_id matters: locations used to be free text with no coordinates, and
// every client built its map link as a fuzzy search (`maps/search/?api=1&
// query=<venue>, <city>`). Google then guessed, and often guessed wrong — a
// mismatch like "FNB Stadium, Sandton" (FNB is in Nasrec) could send buyers to
// the wrong side of the city. With a place_id the clients link to the EXACT
// place Google resolved.
//
// ── Why this was rewritten ───────────────────────────────────────────────────
//
// The previous version used google.maps.places.Autocomplete. As of 1 March
// 2025 that class is not available to NEW customers — it keeps working for API
// keys issued before then, which is why it can look fine in an older project
// and be dead in a fresh one. On a new key `places.Autocomplete` is simply
// undefined, the old attach() hit its `if (!Auto) return` guard, and the
// component sat on "Loading address search…" forever with no error shown.
//
// Two lessons are baked in below:
//   1. Use the current API — AutocompleteSuggestion + Place.fetchFields.
//   2. Never leave a status unresolved. Every path here ends in 'ready' or
//      'error'; the silent return is what made the original failure invisible.
//
// REQUIRES "Places API (New)" enabled on the Google Cloud project. The older
// "Places API" is not sufficient — requests fail with a 403.
//
// Falls back to plain manual entry whenever the key is missing or the API
// fails: the three fields below remain editable and authoritative.
import { useEffect, useRef, useState, useCallback } from 'react';
// Import the CSS prop type directly: with the modern JSX transform the
// global namespace isn't in scope, so qualifying it would fail to compile.
import type { CSSProperties, KeyboardEvent as ReactKeyboardEvent } from 'react';

const MAPS_KEY = process.env.NEXT_PUBLIC_GOOGLE_MAPS_KEY || '';
const SCRIPT_ID = 'google-maps-bootstrap';
const CALLBACK = '__thGoogleMapsReady';
const DEBOUNCE_MS = 250;

export interface PlaceResult {
  venueName: string;
  address: string;
  city: string;
  placeId: string;
  latitude: number;
  longitude: number;
  formatted: string;
}

/** The subset of the new Place object this component reads. */
interface FetchedPlace {
  fetchFields: (o: { fields: string[] }) => Promise<unknown>;
  id?: string;
  displayName?: string | null;
  formattedAddress?: string | null;
  location?: { lat: () => number; lng: () => number } | null;
  addressComponents?: Array<{ longText: string; shortText: string; types: string[] }> | null;
}

/** One row in the dropdown. */
interface Suggestion {
  placeId: string;
  main: string;
  secondary: string;
  /** Opaque handle back to Google's prediction, used to fetch full details. */
  prediction: { toPlace: () => FetchedPlace };
}

// ── Loader ───────────────────────────────────────────────────────────────────

/**
 * Loads the Maps JS bootstrap once, then resolves when google.maps.importLibrary
 * is available.
 *
 * `loading=async` requires a callback. Without one the API warns, and — more
 * importantly — script.onload fires before the libraries are usable. Resolving
 * on onload was the second reason the old component stalled: it attached before
 * `google.maps.places` existed.
 */
function loadMaps(): Promise<void> {
  if (typeof window === 'undefined') return Promise.reject(new Error('no window'));

  const w = window as unknown as {
    google?: { maps?: { importLibrary?: unknown } };
    __mapsPromise?: Promise<void>;
    [key: string]: unknown;
  };

  if (w.google?.maps?.importLibrary) return Promise.resolve();
  if (w.__mapsPromise) return w.__mapsPromise;
  if (!MAPS_KEY) return Promise.reject(new Error('NEXT_PUBLIC_GOOGLE_MAPS_KEY not set'));

  w.__mapsPromise = new Promise<void>((resolve, reject) => {
    const timeout = setTimeout(
      () => reject(new Error('Google Maps timed out')),
      15000,
    );

    w[CALLBACK] = () => {
      clearTimeout(timeout);
      resolve();
    };

    // A script tag already in flight will still fire the shared callback.
    if (document.getElementById(SCRIPT_ID)) return;

    const el = document.createElement('script');
    el.id = SCRIPT_ID;
    el.async = true;
    el.src =
      'https://maps.googleapis.com/maps/api/js' +
      `?key=${encodeURIComponent(MAPS_KEY)}` +
      '&v=weekly&loading=async' +
      `&callback=${CALLBACK}`;
    el.onerror = () => {
      clearTimeout(timeout);
      reject(new Error('Failed to load Google Maps'));
    };
    document.head.appendChild(el);
  });

  return w.__mapsPromise;
}

/** Pull a named component out of a new-API addressComponents array. */
function comp(
  components: Array<{ longText: string; shortText: string; types: string[] }> | null | undefined,
  ...types: string[]
): string {
  if (!components) return '';
  for (const t of types) {
    const hit = components.find((c) => c.types.includes(t));
    if (hit) return hit.longText;
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

export default function PlacesAutocomplete({
  onSelect,
  initialValue = '',
  country = 'za',
  style,
}: Props) {
  const [query, setQuery] = useState(initialValue);
  const [suggestions, setSuggestions] = useState<Suggestion[]>([]);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);
  const [status, setStatus] = useState<'idle' | 'loading' | 'ready' | 'error'>('idle');
  const [errMsg, setErrMsg] = useState('');

  const boxRef = useRef<HTMLDivElement | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const sessionRef = useRef<unknown>(null);
  // Guards against a slow response for an old query overwriting a newer one.
  const seqRef = useRef(0);

  // ── Fetch predictions ──────────────────────────────────────────────────────
  const fetchSuggestions = useCallback(
    async (input: string) => {
      const seq = ++seqRef.current;
      if (input.trim().length < 2) {
        setSuggestions([]);
        setOpen(false);
        setStatus('ready');
        return;
      }

      try {
        await loadMaps();

        const g = (window as unknown as {
          google: { maps: { importLibrary: (n: string) => Promise<Record<string, unknown>> } };
        }).google;

        const places = await g.maps.importLibrary('places');
        const AutocompleteSuggestion = places.AutocompleteSuggestion as {
          fetchAutocompleteSuggestions: (
            r: Record<string, unknown>,
          ) => Promise<{ suggestions: unknown[] }>;
        } | undefined;
        const AutocompleteSessionToken = places.AutocompleteSessionToken as
          | (new () => unknown)
          | undefined;

        if (!AutocompleteSuggestion || !AutocompleteSessionToken) {
          throw new Error('Places API (New) is not enabled for this key');
        }

        // One session token spans the keystrokes of a single search and is
        // consumed by the fetchFields call on selection. Google bills the
        // session rather than each keystroke, so reusing it matters.
        if (!sessionRef.current) sessionRef.current = new AutocompleteSessionToken();

        const { suggestions: raw } =
          await AutocompleteSuggestion.fetchAutocompleteSuggestions({
            input,
            sessionToken: sessionRef.current,
            // Region bias only. Deliberately NO type restriction: the old code
            // passed ['establishment', 'geocode'], which Google rejects as an
            // invalid combination, and any single restriction would exclude
            // either venues or street addresses. Unrestricted is what makes
            // this behave like the Maps search bar.
            ...(country ? { includedRegionCodes: [country.toUpperCase()] } : {}),
          });

        if (seq !== seqRef.current) return; // a newer query already came back

        const mapped: Suggestion[] = [];
        for (const s of raw) {
          const pp = (s as { placePrediction?: Record<string, unknown> }).placePrediction;
          if (!pp) continue;
          const mainText = pp.mainText as { toString: () => string } | undefined;
          const secondaryText = pp.secondaryText as { toString: () => string } | undefined;
          const text = pp.text as { toString: () => string } | undefined;
          mapped.push({
            placeId: String(pp.placeId ?? ''),
            main: mainText ? mainText.toString() : text ? text.toString() : '',
            secondary: secondaryText ? secondaryText.toString() : '',
            prediction: pp as unknown as Suggestion['prediction'],
          });
        }

        setSuggestions(mapped);
        setOpen(mapped.length > 0);
        setActive(-1);
        setStatus('ready');
      } catch (e) {
        if (seq !== seqRef.current) return;
        const msg = (e as Error).message || '';
        setStatus('error');
        setErrMsg(
          msg.includes('NEXT_PUBLIC_GOOGLE_MAPS_KEY')
            ? 'Address search is not configured — enter the venue details manually below.'
            : msg.includes('Places API (New)')
              ? 'Address search needs "Places API (New)" enabled on the Google Cloud key — enter the details manually below.'
              : 'Address search is unavailable right now — enter the venue details manually below.',
        );
        setSuggestions([]);
        setOpen(false);
      }
    },
    [country],
  );

  // ── Typing ─────────────────────────────────────────────────────────────────
  const onChange = (value: string) => {
    setQuery(value);
    if (status !== 'error') setStatus('loading');
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => fetchSuggestions(value), DEBOUNCE_MS);
  };

  // ── Selecting ──────────────────────────────────────────────────────────────
  const choose = useCallback(
    async (s: Suggestion) => {
      setOpen(false);
      try {
        const place = s.prediction.toPlace();
        await place.fetchFields({
          fields: ['id', 'displayName', 'formattedAddress', 'addressComponents', 'location'],
        });

        // Selection ends the billing session; the next search starts a new one.
        sessionRef.current = null;

        const streetNo = comp(place.addressComponents, 'street_number');
        const route = comp(place.addressComponents, 'route');
        const street = [streetNo, route].filter(Boolean).join(' ');
        const city =
          comp(place.addressComponents, 'locality') ||
          comp(place.addressComponents, 'postal_town') ||
          comp(place.addressComponents, 'sublocality') ||
          comp(place.addressComponents, 'administrative_area_level_2') ||
          comp(place.addressComponents, 'administrative_area_level_1');

        const formatted = place.formattedAddress ?? '';
        const displayName = place.displayName ?? '';

        setQuery(formatted || displayName || s.main);

        onSelect({
          placeId: place.id ?? s.placeId,
          // A street address has no business name; Google returns the street
          // line as displayName in that case, which is a reasonable venue name
          // and the organiser can edit it in the field below regardless.
          venueName: displayName || s.main,
          // Prefer the parsed street line; fall back to the first segment of
          // the formatted address so a rural or unaddressed venue still gets
          // something usable.
          address: street || formatted.split(',')[0] || '',
          city,
          latitude: place.location ? place.location.lat() : 0,
          longitude: place.location ? place.location.lng() : 0,
          formatted,
        });
      } catch {
        setStatus('error');
        setErrMsg('Could not load that place — enter the venue details manually below.');
      }
    },
    [onSelect],
  );

  // ── Keyboard ───────────────────────────────────────────────────────────────
  const onKeyDown = (e: ReactKeyboardEvent<HTMLInputElement>) => {
    if (!open || suggestions.length === 0) return;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setActive((i) => (i + 1) % suggestions.length);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setActive((i) => (i <= 0 ? suggestions.length - 1 : i - 1));
    } else if (e.key === 'Enter') {
      // Only swallow Enter when a row is highlighted, so it can never
      // accidentally block form submission.
      if (active >= 0) {
        e.preventDefault();
        choose(suggestions[active]);
      }
    } else if (e.key === 'Escape') {
      setOpen(false);
    }
  };

  // ── Dismiss on outside click ───────────────────────────────────────────────
  useEffect(() => {
    const onDoc = (e: MouseEvent) => {
      if (boxRef.current && !boxRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, []);

  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, []);

  const showSpinner = status === 'loading' && query.trim().length >= 2;

  return (
    <div className="field" ref={boxRef} style={{ position: 'relative' }}>
      <label>
        Search for the venue or address{' '}
        <small style={{ color: 'var(--text-3)', fontWeight: 400 }}>
          (fills the fields below and pins the exact spot on maps)
        </small>
      </label>

      <input
        value={query}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={onKeyDown}
        onFocus={() => { if (suggestions.length) setOpen(true); }}
        placeholder="Start typing — e.g. 43 Sloane Street, or FNB Stadium"
        style={style}
        autoComplete="off"
        // Browsers try to autofill address inputs and fight the dropdown.
        name="venue-search"
        role="combobox"
        aria-expanded={open}
        aria-autocomplete="list"
      />

      {open && suggestions.length > 0 && (
        <ul
          role="listbox"
          style={{
            position: 'absolute',
            zIndex: 40,
            left: 0,
            right: 0,
            top: '100%',
            marginTop: 4,
            listStyle: 'none',
            background: 'var(--surface)',
            border: '1px solid var(--border-2)',
            borderRadius: 'var(--r-lg)',
            boxShadow: 'var(--shadow)',
            maxHeight: 300,
            overflowY: 'auto',
            padding: 4,
          }}
        >
          {suggestions.map((s, i) => (
            <li
              key={s.placeId || `${s.main}-${i}`}
              role="option"
              aria-selected={i === active}
              onMouseEnter={() => setActive(i)}
              // mousedown, not click: the input's blur would close the list
              // before a click ever landed.
              onMouseDown={(e) => { e.preventDefault(); choose(s); }}
              style={{
                display: 'flex',
                alignItems: 'baseline',
                gap: 8,
                padding: '9px 10px',
                borderRadius: 'var(--r)',
                cursor: 'pointer',
                background: i === active ? 'var(--surface-3)' : 'transparent',
              }}
            >
              <span aria-hidden style={{ color: 'var(--text-3)', fontSize: 13 }}>📍</span>
              <span style={{ minWidth: 0 }}>
                <span style={{ fontSize: 14, color: 'var(--text)', fontWeight: 600 }}>
                  {s.main}
                </span>
                {s.secondary && (
                  <span style={{ fontSize: 13, color: 'var(--text-2)', marginLeft: 6 }}>
                    {s.secondary}
                  </span>
                )}
              </span>
            </li>
          ))}
        </ul>
      )}

      {showSpinner && <small style={{ color: 'var(--text-3)' }}>Searching…</small>}
      {status === 'error' && <small style={{ color: '#c79249' }}>{errMsg}</small>}
    </div>
  );
}