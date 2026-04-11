import index from "@/app/index"
import feed from "@/app/feed";
import React from "react";
import { createBottomTabNavigator } from "@react-navigation/bottom-tabs";
import merch from "@/app/merch";
import profile from "@/app/profile";
import { Ionicons } from "@expo/vector-icons";

const Tab = createBottomTabNavigator();

const App = () => {
  return (
      <Tab.Navigator
        screenOptions={({ route }) => ({
          headerShown: false,
          tabBarActiveTintColor: "#e91e63",
          tabBarInactiveTintColor: "gray",
          tabBarStyle: {
            backgroundColor: "transparent",
            position: "absolute",
            borderTopWidth: 0,
          },
          tabBarIcon: ({ color, size }) => {
            let iconName: keyof typeof Ionicons.glyphMap = "home";
            if (route.name === "Home") iconName = "home";
            else if (route.name === "Add Menu") iconName = "add-circle";
            else if (route.name === "Courses") iconName = "restaurant";
            return <Ionicons name={iconName} size={size} color={color} />;
          },
        })}
      >
        <Tab.Screen name= "Home" component={index} />
        <Tab.Screen name= "Merch" component={merch} />
        <Tab.Screen name= "Feed" component={feed} />
        <Tab.Screen name= "Profile" component={profile} />
      </Tab.Navigator>
  );
};

export default App;

