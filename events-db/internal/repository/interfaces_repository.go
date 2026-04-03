package repository

import (
	"events-db/internal/models"
	"events-db/internal/utils"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ──────────────────────────────────────────────────────────────────────────────
// Interfaces
//
// Each interface captures exactly what the handlers need from that slice of the
// data layer. Keeping them narrow (interface segregation) makes them easy to
// mock in tests without pulling in unrelated methods.
// ──────────────────────────────────────────────────────────────────────────────

// BookingQuerier is the read/write contract for booking data.
type BookingQuerier interface {
	GetBookingByID(id string) (*models.Booking, error)
	GetUserBookings(userID string, limit int, cursor *utils.Cursor) ([]models.Booking, *utils.Cursor, error)
	CreateBooking(dto *models.CreateBookingDTO, userID string, event *models.Event) (*models.Booking, error)
	RequestRefundWithCascade(bookingID, userID, reason string) ([]CancelledAssignee, error)
	GetUnsentConfirmedBookingIDs() ([]string, error)
}

// EventQuerier is the read/write contract for event data.
type EventQuerier interface {
	GetEventByID(id string, requestingUserID *string) (*models.Event, error)
	GetEventTitlesByIDs(ids []string) (map[string]string, error)
}

// CartQuerier is the read/write contract for cart data.
type CartQuerier interface {
	GetOrCreateCart(userID, sessionID *string) (*models.Cart, error)
	GetCartWithItems(cartID string) (*models.Cart, error)
	AddCartItem(cartID string, dto *models.AddCartItemDTO, price float64) (*models.CartItem, error)
	UpdateCartItemQuantity(cartID, itemID string, quantity int) error
	DeleteCartItem(cartID, itemID string) error
	ClearCart(cartID string) error
	MergeGuestCart(userID, sessionID string) error
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
//
// Thin structs that delegate to the existing package-level functions. Handlers
// receive the interface; tests inject a mock; production wires these structs.
// ──────────────────────────────────────────────────────────────────────────────

// PgxBookingQuerier implements BookingQuerier against a real Postgres pool.
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

// PgxEventQuerier implements EventQuerier against a real Postgres pool.
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

// PgxCartQuerier implements CartQuerier against a real Postgres pool.
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

func (q *PgxCartQuerier) AddCartItem(cartID string, dto *models.AddCartItemDTO, price float64) (*models.CartItem, error) {
	return AddCartItem(q.pool, cartID, dto, price)
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

// PgxReservationQuerier implements ReservationQuerier against a real Postgres pool.
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