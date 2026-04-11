import { View, Text, StyleSheet } from "react-native";

export default function Merch() {
  return (
    <View style={styles.container}>
      <Text>Merch</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
  },
});