package repository

import (
	"events-db/internal/models"
	"events-db/internal/utils"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ──────────────────────────────────────────────────────────────────────────────
// Interfaces
// ──────────────────────────────────────────────────────────────────────────────

// BookingQuerier is the read/write contract for booking data.
// FIX 3: Added SumBookingTotalsByIDs so CheckoutConfirmHandler can use the
// interface instead of calling the package function with a raw pool.
type BookingQuerier interface {
	GetBookingByID(id string) (*models.Booking, error)
	GetUserBookings(userID string, limit int, cursor *utils.Cursor) ([]models.Booking, *utils.Cursor, error)
	CreateBooking(dto *models.CreateBookingDTO, userID string, event *models.Event) (*models.Booking, error)
	RequestRefundWithCascade(bookingID, userID, reason string) ([]CancelledAssignee, error)
	GetUnsentConfirmedBookingIDs() ([]string, error)
	SumBookingTotalsByIDs(ids []string) (float64, error) // FIX 3
	// CreateMerchOnlyBooking creates a placeholder booking row for a cart that
	// contains only merchandise (no ticket items). Needed so the payment record
	// has a booking to anchor to.
	CreateMerchOnlyBooking(cartID string, userID, sessionID *string, total float64, currency string) (*models.Booking, error)
	// ConfirmBooking marks a booking as confirmed with payment completed.
	// Used for free-ticket bookings that require no payment step.
	ConfirmBooking(bookingID string) error
}

// EventQuerier is the read/write contract for event data.
type EventQuerier interface {
	GetEventByID(id string, requestingUserID *string) (*models.Event, error)
	GetEventTitlesByIDs(ids []string) (map[string]string, error)
}

// CartQuerier is the read/write contract for cart data.
// FIX 3: Added SumCartTotal so CheckoutBeginHandler and CheckoutConfirmHandler
// can use the interface instead of calling the package function with a raw pool.
type CartQuerier interface {
	GetOrCreateCart(userID, sessionID *string) (*models.Cart, error)
	GetCartWithItems(cartID string) (*models.Cart, error)
	AddCartItem(cartID string, dto *models.AddCartItemDTO, price float64, deliveryFee float64) (*models.CartItem, error)
	UpdateCartItemQuantity(cartID, itemID string, quantity int) error
	DeleteCartItem(cartID, itemID string) error
	ClearCart(cartID string) error
	MergeGuestCart(userID, sessionID string) error
	SumCartTotal(cartID string) (float64, error) // FIX 3
}

// ReservationQuerier is the read/write contract for ticket reservation data.
type ReservationQuerier interface {
	CreateReservations(cartID string, userID, sessionID *string) ([]models.TicketReservation, error)
	RefreshReservations(cartID string) (int, error)
	AbandonReservations(cartID string) error
	ConvertReservationsToBookings(cartID string, userID, sessionID *string) ([]string, error)
	GetReservationsByCart(cartID string) ([]models.TicketReservation, error)
}

// ──────────────────────────────────────────────────────────────────────────────
// Concrete pgx adapters
// ──────────────────────────────────────────────────────────────────────────────

type PgxBookingQuerier struct{ pool *pgxpool.Pool }

func NewPgxBookingQuerier(pool *pgxpool.Pool) *PgxBookingQuerier {
	return &PgxBookingQuerier{pool: pool}
}

func (q *PgxBookingQuerier) GetBookingByID(id string) (*models.Booking, error) {
	return GetBookingByID(q.pool, id)
}
func (q *PgxBookingQuerier) GetUserBookings(userID string, limit int, cursor *utils.Cursor) ([]models.Booking, *utils.Cursor, error) {
	return GetUserBookings(q.pool, userID, limit, cursor)
}
func (q *PgxBookingQuerier) CreateBooking(dto *models.CreateBookingDTO, userID string, event *models.Event) (*models.Booking, error) {
	return CreateBooking(q.pool, dto, userID, event)
}
func (q *PgxBookingQuerier) RequestRefundWithCascade(bookingID, userID, reason string) ([]CancelledAssignee, error) {
	return RequestRefundWithCascade(q.pool, bookingID, userID, reason)
}
func (q *PgxBookingQuerier) GetUnsentConfirmedBookingIDs() ([]string, error) {
	return GetUnsentConfirmedBookingIDs(q.pool)
}
// FIX 3: new delegate
func (q *PgxBookingQuerier) SumBookingTotalsByIDs(ids []string) (float64, error) {
	return SumBookingTotalsByIDs(q.pool, ids)
}
func (q *PgxBookingQuerier) CreateMerchOnlyBooking(cartID string, userID, sessionID *string, total float64, currency string) (*models.Booking, error) {
	return CreateMerchOnlyBooking(q.pool, cartID, userID, sessionID, total, currency)
}
func (q *PgxBookingQuerier) ConfirmBooking(bookingID string) error {
	return ConfirmBooking(q.pool, bookingID)
}

type PgxEventQuerier struct{ pool *pgxpool.Pool }

func NewPgxEventQuerier(pool *pgxpool.Pool) *PgxEventQuerier {
	return &PgxEventQuerier{pool: pool}
}
func (q *PgxEventQuerier) GetEventByID(id string, requestingUserID *string) (*models.Event, error) {
	return GetEventByID(q.pool, id, requestingUserID)
}
func (q *PgxEventQuerier) GetEventTitlesByIDs(ids []string) (map[string]string, error) {
	return GetEventTitlesByIDs(q.pool, ids)
}

type PgxCartQuerier struct{ pool *pgxpool.Pool }

func NewPgxCartQuerier(pool *pgxpool.Pool) *PgxCartQuerier {
	return &PgxCartQuerier{pool: pool}
}
func (q *PgxCartQuerier) GetOrCreateCart(userID, sessionID *string) (*models.Cart, error) {
	return GetOrCreateCart(q.pool, userID, sessionID)
}
func (q *PgxCartQuerier) GetCartWithItems(cartID string) (*models.Cart, error) {
	return GetCartWithItems(q.pool, cartID)
}
func (q *PgxCartQuerier) AddCartItem(cartID string, dto *models.AddCartItemDTO, price float64, deliveryFee float64) (*models.CartItem, error) {
	return AddCartItem(q.pool, cartID, dto, price, deliveryFee)
}
func (q *PgxCartQuerier) UpdateCartItemQuantity(cartID, itemID string, quantity int) error {
	return UpdateCartItemQuantity(q.pool, cartID, itemID, quantity)
}
func (q *PgxCartQuerier) DeleteCartItem(cartID, itemID string) error {
	return DeleteCartItem(q.pool, cartID, itemID)
}
func (q *PgxCartQuerier) ClearCart(cartID string) error {
	return ClearCart(q.pool, cartID)
}
func (q *PgxCartQuerier) MergeGuestCart(userID, sessionID string) error {
	return MergeGuestCart(q.pool, userID, sessionID)
}
// FIX 3: new delegate
func (q *PgxCartQuerier) SumCartTotal(cartID string) (float64, error) {
	return SumCartTotal(q.pool, cartID)
}

type PgxReservationQuerier struct{ pool *pgxpool.Pool }

func NewPgxReservationQuerier(pool *pgxpool.Pool) *PgxReservationQuerier {
	return &PgxReservationQuerier{pool: pool}
}
func (q *PgxReservationQuerier) CreateReservations(cartID string, userID, sessionID *string) ([]models.TicketReservation, error) {
	return CreateReservations(q.pool, cartID, userID, sessionID)
}
func (q *PgxReservationQuerier) RefreshReservations(cartID string) (int, error) {
	return RefreshReservations(q.pool, cartID)
}
func (q *PgxReservationQuerier) AbandonReservations(cartID string) error {
	return AbandonReservations(q.pool, cartID)
}
func (q *PgxReservationQuerier) ConvertReservationsToBookings(cartID string, userID, sessionID *string) ([]string, error) {
	return ConvertReservationsToBookings(q.pool, cartID, userID, sessionID)
}
func (q *PgxReservationQuerier) GetReservationsByCart(cartID string) ([]models.TicketReservation, error) {
	return GetReservationsByCart(q.pool, cartID)
}