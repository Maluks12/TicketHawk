import React from 'react';
import { View, Text, StyleSheet } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { Ionicons } from '@expo/vector-icons';

export default function TicketsScreen() {
  return (
    <SafeAreaView style={styles.safe}>
      <View style={styles.container}>
        <Ionicons name="ticket-outline" size={56} color="rgba(255,255,255,0.15)" />
        <Text style={styles.title}>My Tickets</Text>
        <Text style={styles.sub}>Your booked tickets will appear here.</Text>
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe:      { flex: 1, backgroundColor: '#0a0a0a' },
  container: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: 12 },
  title:     { fontSize: 22, fontWeight: '800', color: '#fff', letterSpacing: 0.3 },
  sub:       { fontSize: 14, color: 'rgba(255,255,255,0.4)', textAlign: 'center' },
});