import React from "react";
import { Slot } from "expo-router";
import { View, Text, StyleSheet, TouchableOpacity } from "react-native";
import { useRouter } from "expo-router";

function WebHeader() {
  const router = useRouter();
  return (
    <View style={styles.header}>
      <View style={styles.headerLeft}>
        <Text style={styles.logo}>EVENTS<Text style={styles.logoAccent}>APP</Text></Text>
        <TouchableOpacity onPress={() => router.push("/")}><Text style={styles.navLink}>Home</Text></TouchableOpacity>
        <TouchableOpacity onPress={() => router.push("/merch")}><Text style={styles.navLink}>Merch</Text></TouchableOpacity>
        <TouchableOpacity onPress={() => router.push("/feed")}><Text style={styles.navLink}>Feed</Text></TouchableOpacity>
        <TouchableOpacity onPress={() => router.push("/profile")}><Text style={styles.navLink}>Profile</Text></TouchableOpacity>
      </View>
      <View style={styles.headerRight}>
        <TouchableOpacity style={styles.signInBtn}>
          <Text style={styles.signInText}>Sign in</Text>
        </TouchableOpacity>
      </View>
    </View>
  );
}

export default function WebLayout() {
  return (
    <View style={styles.container}>
      <WebHeader />
      <View style={styles.content}>
        <Slot />
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
  },
  content: {
    flex: 1,
  },
  header: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    backgroundColor: "#000000",
    paddingHorizontal: 32,
    paddingVertical: 16,
    height: 64,
  },
  headerLeft: {
    flexDirection: "row",
    alignItems: "center",
    gap: 32,
  },
  logo: {
    color: "#ffffff",
    fontSize: 18,
    fontWeight: "900",
    letterSpacing: 1,
    marginRight: 16,
  },
  logoAccent: {
    color: "#E8472A",
  },
  navLink: {
    color: "#ffffff",
    fontSize: 15,
    fontWeight: "500",
  },
  headerRight: {
    flexDirection: "row",
    alignItems: "center",
    gap: 20,
  },
  signInBtn: {
    backgroundColor: "#ffffff",
    paddingHorizontal: 20,
    paddingVertical: 8,
    borderRadius: 6,
  },
  signInText: {
    color: "#000000",
    fontWeight: "700",
    fontSize: 14,
  },
});