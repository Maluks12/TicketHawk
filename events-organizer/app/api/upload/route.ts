// Image uploads go directly to the Go backend — no Next.js proxy needed.
//
// Organiser dashboard calls:  POST /organiser/upload  (field: "image")
// Backend handler:            handlers.UploadOrganiserImageHandler
// Returns:                    { image_url: "https://..." }
//
// See: events-db/internal/handlers/image_handler.go → UploadOrganiserImageHandler
export {};