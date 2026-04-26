package models

// FIX 6: DefaultCurrency replaces the "ZAR" string literal that was scattered
// across handlers, repositories, and services (12+ occurrences). Using a
// constant means a future multi-currency requirement is a one-line change and
// a global search for "ZAR" will find only legitimate data (e.g. test fixtures,
// SQL seeds) rather than logic that should be driven by config.
//
// Usage — replace every occurrence of "ZAR" in handler/repository code with:
//
//   models.DefaultCurrency
//
// Files to update (search for `"ZAR"` and replace where it is used as a
// computed/default value, NOT in test fixtures or seed SQL):
//
//   internal/handlers/checkout_handler.go         line 91
//   internal/handlers/cart_handler.go             line 226
//   internal/handlers/ad_campaign_handler.go      lines 73, 130
//   internal/repository/organizer_dashboard_repository.go  line 574
//   internal/services/payment_service.go          line 380
const DefaultCurrency = "ZAR"