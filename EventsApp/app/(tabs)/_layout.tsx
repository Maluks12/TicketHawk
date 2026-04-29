import React from 'react';
import { Tabs } from 'expo-router';
import { View, StyleSheet } from 'react-native';
import { Feather, Ionicons } from '@expo/vector-icons';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

// ── Design tokens ─────────────────────────────────────────────────────────────
const ACTIVE   = '#0a0a0a';
const INACTIVE = 'rgba(10,10,10,0.30)';
const BAR_BG   = '#ffffff';
const DOT_CLR  = '#ff3c6e';
const SZ       = 26;

function Dot() {
  return <View style={styles.dot} />;
}

function HomeIcon({ focused }: { focused: boolean }) {
  return (
    <View style={styles.iconWrap}>
      <Feather name="home" size={SZ} color={focused ? ACTIVE : INACTIVE} />
    </View>
  );
}

function MerchIcon({ focused }: { focused: boolean }) {
  return (
    <View style={styles.iconWrap}>
      <Feather name="shopping-bag" size={SZ} color={focused ? ACTIVE : INACTIVE} />
    </View>
  );
}

function SearchIcon({ focused }: { focused: boolean }) {
  return (
    <View style={styles.iconWrap}>
      <Feather name="search" size={SZ} color={focused ? ACTIVE : INACTIVE} />
    </View>
  );
}

function TicketsIcon({ focused }: { focused: boolean }) {
  return (
    <View style={styles.iconWrap}>
      <Ionicons
        name="ticket-outline"
        size={SZ + 2}
        color={focused ? ACTIVE : INACTIVE}
      />
    </View>
  );
}

function ProfileIcon({ focused, hasBadge }: { focused: boolean; hasBadge?: boolean }) {
  return (
    <View style={styles.iconWrap}>
      <View style={[styles.avatarRing, focused && styles.avatarRingActive]}>
        <Feather name="user" size={SZ - 4} color={focused ? ACTIVE : INACTIVE} />
      </View>
      {hasBadge && <Dot />}
    </View>
  );
}

export default function TabLayout() {
  const insets = useSafeAreaInsets();

  return (
    <Tabs
      screenOptions={{
        headerShown:             false,
        tabBarShowLabel:         false,
        tabBarActiveTintColor:   ACTIVE,
        tabBarInactiveTintColor: INACTIVE,
        tabBarStyle: {
          ...styles.tabBar,
          height:        55 + insets.bottom,
          paddingBottom: insets.bottom,
          paddingTop:    10,
        },
        tabBarItemStyle: styles.tabItem,
      }}
    >
      <Tabs.Screen
        name="index"
        options={{ tabBarIcon: ({ focused }) => <HomeIcon focused={focused} /> }}
      />
      <Tabs.Screen
        name="merch"
        options={{ tabBarIcon: ({ focused }) => <MerchIcon focused={focused} /> }}
      />
      <Tabs.Screen
        name="search"
        options={{ tabBarIcon: ({ focused }) => <SearchIcon focused={focused} /> }}
      />
      <Tabs.Screen
        name="tickets"
        options={{ tabBarIcon: ({ focused }) => <TicketsIcon focused={focused} /> }}
      />
      <Tabs.Screen
        name="profile"
        options={{ tabBarIcon: ({ focused }) => <ProfileIcon focused={focused} hasBadge /> }}
      />
    </Tabs>
  );
}

const styles = StyleSheet.create({
  tabBar: {
    backgroundColor:  BAR_BG,
    borderTopWidth:   StyleSheet.hairlineWidth,
    borderTopColor:   'rgba(10,10,10,0.12)',
    borderRadius:     0,
    marginHorizontal: 0,
    marginBottom:     0,
    shadowColor:      '#000',
    shadowOffset:     { width: 0, height: -2 },
    shadowOpacity:    0.05,
    shadowRadius:     8,
    elevation:        4,
  },

  tabItem: {
    flex:           1,
    alignItems:     'center',
    justifyContent: 'flex-start',
  },

  iconWrap: {
    position:       'relative',
    alignItems:     'center',
    justifyContent: 'center',
    width:          44,
    height:         44,
  },

  dot: {
    position:        'absolute',
    top:             6,
    right:           4,
    width:           8,
    height:          8,
    borderRadius:    4,
    backgroundColor: DOT_CLR,
    borderWidth:     1.5,
    borderColor:     BAR_BG,
  },

  avatarRing: {
    width:          36,
    height:         36,
    borderRadius:   18,
    alignItems:     'center',
    justifyContent: 'center',
    borderWidth:    1.5,
    borderColor:    'transparent',
  },
  avatarRingActive: {
    borderColor: ACTIVE,
  },
});