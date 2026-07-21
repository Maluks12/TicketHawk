// ─────────────────────────────────────────────────────────────────────────────
// TicketHawk Studio hand-off.
//
// Phase 4: the Organiser Studio + door scanning moved out of the consumer app
// into a standalone "TicketHawk Studio" app (EventsStudio/). The consumer app
// only keeps a doorway: try the Studio deep link; if it isn't installed, tell
// the user how to get it.
//
// Once Studio is on the Play Store, set STUDIO_STORE_URL and the fallback
// becomes "open the store listing" instead of an instructional alert.
// ─────────────────────────────────────────────────────────────────────────────
import { Alert, Linking, Platform } from 'react-native';

const STUDIO_SCHEME = 'tickethawkstudio://';

// Fill in once published, e.g.
// 'https://play.google.com/store/apps/details?id=com.tickethawk.studio'
const STUDIO_STORE_URL: string | null = null;

export async function openStudioApp(): Promise<void> {
  try {
    const supported = await Linking.canOpenURL(STUDIO_SCHEME);
    if (supported) {
      await Linking.openURL(STUDIO_SCHEME);
      return;
    }
  } catch {
    /* fall through to the fallback below */
  }

  if (STUDIO_STORE_URL) {
    Linking.openURL(STUDIO_STORE_URL).catch(() => showInstallAlert());
    return;
  }
  showInstallAlert();
}

function showInstallAlert() {
  Alert.alert(
    'TicketHawk Studio',
    Platform.OS === 'android'
      ? 'Ticket scanning and the organiser dashboard now live in the TicketHawk Studio app. Install it to scan tickets at the door and track your events.'
      : 'Ticket scanning and the organiser dashboard now live in the TicketHawk Studio app. Install it to scan tickets at the door and track your events.',
    [{ text: 'OK' }],
  );
}