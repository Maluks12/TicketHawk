// Overlay that marks a feed card as an event COLLECTION rather than a single
// event: a stack icon, "COLLECTION" and the number of dates. Bottom-left of the
// card image by default, so it never competes with the price chip (top-right)
// or the LIVE / rank markers (top-left). Same look as the web apps' badge.
import { View, Text, StyleSheet } from 'react-native';
import { Ionicons } from '@expo/vector-icons';
import type { EventCollectionRef } from '../lib/api';
import { collectionCountLabel } from '../lib/eventCollections';

export function CollectionBadge({ collection, position = 'bottom-left' }: {
  collection: EventCollectionRef;
  /** Cards that already use the bottom-left corner pick another. */
  position?: 'bottom-left' | 'top-left';
}) {
  return (
    <View
      style={[styles.badge, position === 'top-left' ? styles.topLeft : styles.bottomLeft]}
      accessibilityLabel={`Collection of ${collection.event_count} dates`}
    >
      <Ionicons name="layers" size={11} color="#ffffff" />
      <Text style={styles.text}>COLLECTION · {collectionCountLabel(collection)}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  badge: {
    position: 'absolute',
    flexDirection: 'row',
    alignItems: 'center',
    gap: 5,
    height: 20,
    paddingHorizontal: 7,
    borderRadius: 5,
    backgroundColor: 'rgba(0,0,0,0.72)',
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: 'rgba(255,255,255,0.25)',
  },
  bottomLeft: { left: 10, bottom: 10 },
  topLeft: { left: 12, top: 12 },
  text: {
    color: '#ffffff',
    fontSize: 8.5,
    fontWeight: '800',
    letterSpacing: 1.1,
  },
});