import React from "react";
import { Tabs } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { View, StyleSheet } from "react-native";

export default function Layout() {
  return (
    <Tabs
      screenOptions={({ route }) => ({
        headerShown: false,
        tabBarActiveTintColor: "#ff3c6e",
        tabBarInactiveTintColor: "rgba(255,255,255,0.45)",
        tabBarStyle: styles.tabBar,
        tabBarLabelStyle: styles.tabLabel,
        tabBarItemStyle: styles.tabItem,
        tabBarIcon: ({ color, focused, size }) => {
          let iconName: keyof typeof Ionicons.glyphMap = "home";
          if (route.name === "index") iconName = "home";
          else if (route.name === "merch") iconName = "bag";
          else if (route.name === "feed") iconName = "newspaper";
          else if (route.name === "profile") iconName = "person-circle-outline";
          return (
            <View style={[styles.iconWrapper, focused && styles.iconWrapperFocused]}>
              <Ionicons name={iconName} size={size} color={color} />
            </View>
          );
        },
      })}
    >
      <Tabs.Screen name="index" options={{ title: "Home" }} />
      <Tabs.Screen name="merch" options={{ title: "Merch" }} />
      <Tabs.Screen name="feed" options={{ title: "Feed" }} />
      <Tabs.Screen name="profile" options={{ title: "Profile" }} />
    </Tabs>
  );
}

const styles = StyleSheet.create({
  tabBar: {
    backgroundColor: "#002047",
    borderRadius: 40,
    marginHorizontal: 16,
    marginBottom: 20,
    height: 70,
    position: "absolute",
    left: 0,
    right: 0,
    bottom: 0,
    borderTopWidth: 0,
    shadowColor: "#000",
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.4,
    shadowRadius: 16,
    elevation: 10,
  },
  tabItem: {
    paddingTop: 8,
  },
  tabLabel: {
    fontSize: 11,
    fontWeight: "500",
    marginBottom: 6,
  },
  iconWrapper: {
    padding: 6,
    borderRadius: 30,
  },
  iconWrapperFocused: {
    backgroundColor: "rgba(255,60,110,0.20)",
  },
});