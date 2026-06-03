// This local fallback has been removed.
// All image uploads go through the Go API at POST /organiser/upload,
// which stores files in Cloudflare R2.
//
// Ensure NEXT_PUBLIC_API_URL is set in .env.local, e.g.:\
//   NEXT_PUBLIC_API_URL=http://localhost:8080/v1
export {};