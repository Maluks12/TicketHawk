// Mirrors: EventsApp/app/(tabs)/index.tsx
// Port the component logic from that file here.
// Key changes when porting:
//   - Replace ScrollView / FlatList / View / Text / TouchableOpacity with <div> / <p> / <button>
//   - Replace expo-router useRouter/Link with next/navigation useRouter and next/link Link
//   - Replace expo-image <Image> with next/image <Image>
//   - StyleSheet.create → Tailwind classes or inline styles using useAppTheme() colors
//   - Remove SafeAreaView — NavBar handles top spacing
'use client';

export { default } from '../website/page';

export default function HomePage() {
  return (
    <div>
      {/* TODO: port EventsApp/app/(tabs)/index.tsx here */}
    </div>
  );
}