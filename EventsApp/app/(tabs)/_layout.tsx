import React from 'react';
import { Tabs } from 'expo-router';
import { View, StyleSheet } from 'react-native';
import { Feather, Ionicons } from '@expo/vector-icons';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useAppTheme } from '../../theme/colors';

// ── Design tokens ─────────────────────────────────────────────────────────────
const DOT_CLR  = '#ff3c6e';
const SZ       = 26;

function Dot({ barBg }: { barBg: string }) {
  return <View style={[styles.dot, { borderColor: barBg }]} />;
}

function HomeIcon({ focused, colors }: { focused: boolean; colors: any }) {
  return (
    <View style={styles.iconWrap}>
      <Feather name="home" size={SZ} color={focused ? colors.text : colors.iconColor} />
    </View>
  );
}

function MerchIcon({ focused, colors }: { focused: boolean; colors: any }) {
  return (
    <View style={styles.iconWrap}>
      <Feather name="shopping-bag" size={SZ} color={focused ? colors.text : colors.iconColor} />
    </View>
  );
}

function SearchIcon({ focused, colors }: { focused: boolean; colors: any }) {
  return (
    <View style={styles.iconWrap}>
      <Feather name="search" size={SZ} color={focused ? colors.text : colors.iconColor} />
    </View>
  );
}

function TicketsIcon({ focused, colors }: { focused: boolean; colors: any }) {
  return (
    <View style={styles.iconWrap}>
      <Ionicons name="ticket-outline" size={SZ + 2} color={focused ? colors.text : colors.iconColor} />
    </View>
  );
}

function ProfileIcon({ focused, hasBadge, colors, barBg }: { focused: boolean; hasBadge?: boolean; colors: any; barBg: string }) {
  return (
    <View style={styles.iconWrap}>
      <View style={[styles.avatarRing, focused && { borderColor: colors.text }]}>
        <Feather name="user" size={SZ - 4} color={focused ? colors.text : colors.iconColor} />
      </View>
      {hasBadge && <Dot barBg={barBg} />}
    </View>
  );
}

export default function TabLayout() {
  const insets = useSafeAreaInsets();
  const { colors } = useAppTheme();

  return (
    <Tabs
      screenOptions={{
        headerShown:             false,
        tabBarShowLabel:         false,
        tabBarActiveTintColor:   colors.text,
        tabBarInactiveTintColor: colors.iconColor,
        tabBarStyle: {
          ...styles.tabBar,
          backgroundColor: colors.headerBg,
          borderTopColor:  colors.dividerColor,
          height:        55 + insets.bottom,
          paddingBottom: insets.bottom,
          paddingTop:    10,
        },
        tabBarItemStyle: styles.tabItem,
      }}
    >
      <Tabs.Screen name="index"   options={{ tabBarIcon: ({ focused }) => <HomeIcon focused={focused} colors={colors} /> }} />
      <Tabs.Screen name="merch"   options={{ tabBarIcon: ({ focused }) => <MerchIcon focused={focused} colors={colors} /> }} />
      <Tabs.Screen name="search"  options={{ tabBarIcon: ({ focused }) => <SearchIcon focused={focused} colors={colors} /> }} />
      <Tabs.Screen name="tickets" options={{ tabBarIcon: ({ focused }) => <TicketsIcon focused={focused} colors={colors} /> }} />
      <Tabs.Screen name="profile" options={{ tabBarIcon: ({ focused }) => <ProfileIcon focused={focused} colors={colors} barBg={colors.headerBg} hasBadge /> }} />
    </Tabs>
  );
}

const styles = StyleSheet.create({
  tabBar: {
    borderTopWidth:   StyleSheet.hairlineWidth,
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
});