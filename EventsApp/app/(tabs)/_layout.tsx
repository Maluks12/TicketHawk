import React from 'react';
import { Tabs } from 'expo-router';
import { View, StyleSheet } from 'react-native';
import { House, Storefront, MagnifyingGlass, Ticket, User, type Icon as PhosphorIcon } from 'phosphor-react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useAppTheme } from '../../theme/colors';

// ── Design tokens ─────────────────────────────────────────────────────────────
const SZ = 30;

// One component for every tab: Phosphor regular weight when idle,
// fill weight + text color when focused (Instagram/Spotify pattern).
function TabIcon({ icon: IconCmp, focused, colors }: { icon: PhosphorIcon; focused: boolean; colors: any }) {
  return (
    <View style={styles.iconWrap}>
      <IconCmp
        size={SZ}
        color={focused ? colors.text : colors.iconColor}
        weight={focused ? 'fill' : 'regular'}
      />
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
      <Tabs.Screen
        name="index"
        options={{ tabBarIcon: ({ focused }) => <TabIcon icon={House} focused={focused} colors={colors} /> }}
      />
      {/* Merch is hidden for launch (href: null removes it from the tab bar and
          deep links) but the screen + code stay intact for a future update.
          To re-enable, swap this options object for:
          {{ tabBarIcon: ({ focused }) => <TabIcon icon={Storefront} focused={focused} colors={colors} /> }} */}
      <Tabs.Screen name="merch" options={{ href: null }} />
      <Tabs.Screen
        name="search"
        options={{ tabBarIcon: ({ focused }) => <TabIcon icon={MagnifyingGlass} focused={focused} colors={colors} /> }}
      />
      <Tabs.Screen
        name="tickets"
        options={{ tabBarIcon: ({ focused }) => <TabIcon icon={Ticket} focused={focused} colors={colors} /> }}
      />
      <Tabs.Screen
        name="profile"
        options={{ tabBarIcon: ({ focused }) => <TabIcon icon={User} focused={focused} colors={colors} /> }}
      />
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
});