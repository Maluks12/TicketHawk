import React from 'react';
import { View, Text, Modal, ScrollView, TouchableOpacity, StyleSheet } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useAppTheme } from '../theme/colors';

/**
 * PolicySheet — full-text refund policy or terms, opened from a link row.
 *
 * ── Why a sheet and not the text inline ─────────────────────────────────────
 *
 * These two fields are long by nature — a refund policy that is short is a
 * refund policy that will be argued about. Rendered inline they were adding
 * roughly a screen and a half to the bottom of every event page, which pushes
 * the venue and the CTA further from the reader and buries the line-up nobody
 * scrolled far enough to see.
 *
 * They are also read at a specific moment — deciding to buy, or after something
 * went wrong — rather than while browsing. A link is the honest shape for
 * content like that: present, one tap away, not in the way.
 *
 * ── Why a sheet and not a route ─────────────────────────────────────────────
 *
 * A route would need the policy text passed as a param or re-fetched. The text
 * is already on the event object the screen is holding, so a modal renders it
 * with no navigation, no param serialisation and no second network call.
 */

interface Props {
  visible: boolean;
  title: string;
  body: string;
  onClose: () => void;
}

export function PolicySheet({ visible, title, body, onClose }: Props) {
  const { colors } = useAppTheme();
  const insets = useSafeAreaInsets();

  return (
    <Modal
      visible={visible}
      animationType="slide"
      presentationStyle="pageSheet"
      onRequestClose={onClose}
    >
      <View style={{ flex: 1, backgroundColor: colors.background, paddingTop: insets.top ? 0 : 12 }}>
        <View style={[styles.header, { borderBottomColor: colors.dividerColor }]}>
          <Text style={[styles.title, { color: colors.text }]} numberOfLines={1}>{title}</Text>
          <TouchableOpacity onPress={onClose} hitSlop={14} style={styles.close}>
            <Text style={[styles.closeIcon, { color: colors.subText }]}>✕</Text>
          </TouchableOpacity>
        </View>

        <ScrollView
          contentContainerStyle={{ padding: 20, paddingBottom: insets.bottom + 40 }}
          showsVerticalScrollIndicator={false}
        >
          <Text style={[styles.body, { color: colors.text }]}>{body}</Text>
        </ScrollView>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  header: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: 20,
    paddingVertical: 16,
    borderBottomWidth: 1,
  },
  title: { flex: 1, fontSize: 17, fontWeight: '800', letterSpacing: 0.2 },
  close: { width: 32, height: 32, alignItems: 'center', justifyContent: 'center' },
  closeIcon: { fontSize: 18, fontWeight: '600' },
  body: { fontSize: 15, lineHeight: 24 },
});