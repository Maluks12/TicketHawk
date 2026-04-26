package models

import "time"

// FulfillmentMode describes what delivery options an organiser makes available
// for a merchandise item.
type FulfillmentMode string

const (
	// FulfillmentModeDelivery — buyer must provide a shipping address.
	FulfillmentModeDelivery FulfillmentMode = "delivery"
	// FulfillmentModePickup — buyer collects at the organiser's stated location.
	FulfillmentModePickup FulfillmentMode = "pickup"
	// FulfillmentModeBoth — buyer chooses delivery or pickup at checkout.
	FulfillmentModeBoth FulfillmentMode = "both"
)

// DeliveryAddress is the buyer's shipping address, stored as JSONB in the DB
// and decoded here for validation and display.
type DeliveryAddress struct {
	Street     string `json:"street"`
	Suburb     string `json:"suburb"`
	City       string `json:"city"`
	Province   string `json:"province"`
	PostalCode string `json:"postal_code"`
}

// MerchFulfillmentInfo carries the fulfillment configuration returned alongside
// a variant lookup. Used by the cart handler to validate buyer choices.
type MerchFulfillmentInfo struct {
	FulfillmentMode FulfillmentMode
	PickupLocation  *string
	DeliveryFee     float64
}

type Merchandise struct {
	ID      string  `json:"id" db:"id"`
	// EventID is optional — nil means standalone merch not tied to any event.
	EventID *string `json:"event_id,omitempty" db:"event_id"`
	Name         string          `json:"name" db:"name"`
	Description  *string         `json:"description,omitempty" db:"description"`
	BasePrice    float64         `json:"base_price" db:"base_price"`
	Category     *string         `json:"category,omitempty" db:"category"`
	DisplayOrder int             `json:"display_order" db:"display_order"`
	IsActive     bool            `json:"is_active" db:"is_active"`
	Images       *string         `json:"images,omitempty" db:"images"` // JSONB stored as string

	// Fulfillment configuration — set by the organiser per item.
	// FulfillmentMode controls what options the buyer can choose at checkout.
	// PickupLocation is shown to the buyer when they choose or are forced to pick up.
	// DeliveryFee is a flat fee charged once per cart line item for delivery orders.
	FulfillmentMode FulfillmentMode `json:"fulfillment_mode" db:"fulfillment_mode"`
	PickupLocation  *string         `json:"pickup_location,omitempty" db:"pickup_location"`
	DeliveryFee     float64         `json:"delivery_fee" db:"delivery_fee"`

	SaleStart *time.Time `json:"sale_start,omitempty" db:"sale_start"`
	SaleEnd   *time.Time `json:"sale_end,omitempty" db:"sale_end"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`

	// Joined — populated by GetMerchandiseByEvent / GetAllMerchandise, not a stored column.
	Variants   []MerchandiseVariant `json:"variants,omitempty" db:"-"`
	// EventTitle is joined from the events table for admin list views. Not stored.
	EventTitle string               `json:"event_title,omitempty" db:"-"`
}

// IsLive returns whether the item is currently within its sale window.
// An item with no sale_start / sale_end boundaries is always considered live
// (existing behaviour is preserved for items created before this feature).
func (m *Merchandise) IsLive() bool {
	now := time.Now()
	if m.SaleStart != nil && now.Before(*m.SaleStart) {
		return false
	}
	if m.SaleEnd != nil && !now.Before(*m.SaleEnd) {
		return false
	}
	return true
}

// SaleStatus returns a display string useful for the frontend countdown timer.
//
//	"scheduled"  — sale_start is in the future
//	"live"       — within the sale window (or no window set)
//	"ended"      — sale_end has passed
func (m *Merchandise) SaleStatus() string {
	now := time.Now()
	if m.SaleStart != nil && now.Before(*m.SaleStart) {
		return "scheduled"
	}
	if m.SaleEnd != nil && !now.Before(*m.SaleEnd) {
		return "ended"
	}
	return "live"
}

// AllowsDelivery returns true when the buyer may choose delivery.
func (m *Merchandise) AllowsDelivery() bool {
	return m.FulfillmentMode == FulfillmentModeDelivery || m.FulfillmentMode == FulfillmentModeBoth
}

// AllowsPickup returns true when the buyer may choose pickup.
func (m *Merchandise) AllowsPickup() bool {
	return m.FulfillmentMode == FulfillmentModePickup || m.FulfillmentMode == FulfillmentModeBoth
}

type MerchandiseVariant struct {
	ID            string    `json:"id" db:"id"`
	MerchandiseID string    `json:"merchandise_id" db:"merchandise_id"`
	SKU           string    `json:"sku" db:"sku"`
	Attributes    string    `json:"attributes" db:"attributes"` // JSONB as string e.g. {"size":"L","colour":"blue"}
	StockQuantity int       `json:"stock_quantity" db:"stock_quantity"`
	PriceOverride *float64  `json:"price_override,omitempty" db:"price_override"`
	IsActive      bool      `json:"is_active" db:"is_active"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
}

// EffectivePrice returns price_override if set, otherwise the parent base_price.
func (v *MerchandiseVariant) EffectivePrice(basePrice float64) float64 {
	if v.PriceOverride != nil {
		return *v.PriceOverride
	}
	return basePrice
}

// DTOs — json binding only, no db tags needed

type CreateMerchandiseDTO struct {
	Name         string     `json:"name" binding:"required,min=1,max=255"`
	Description  *string    `json:"description"`
	BasePrice    float64    `json:"base_price" binding:"required,gt=0"`
	Category     *string    `json:"category"`
	DisplayOrder int        `json:"display_order"`
	Images       *string    `json:"images"`

	// Fulfillment — defaults to "delivery" if omitted.
	// Set fulfillment_mode to "pickup" or "both" to enable collection.
	// pickup_location is required when mode is "pickup" or "both".
	FulfillmentMode string  `json:"fulfillment_mode" binding:"omitempty,oneof=delivery pickup both"`
	PickupLocation  *string `json:"pickup_location"`
	DeliveryFee     float64 `json:"delivery_fee" binding:"min=0"`

	// Optional sale window — omit both for always-available items.
	SaleStart *time.Time `json:"sale_start"`
	SaleEnd   *time.Time `json:"sale_end"`
}

type UpdateMerchandiseDTO struct {
	Name         *string  `json:"name" binding:"omitempty,min=1,max=255"`
	Description  *string  `json:"description"`
	BasePrice    *float64 `json:"base_price" binding:"omitempty,gt=0"`
	Category     *string  `json:"category"`
	DisplayOrder *int     `json:"display_order"`
	IsActive     *bool    `json:"is_active"`
	Images       *string  `json:"images"`

	// Fulfillment updates — all optional.
	FulfillmentMode *string  `json:"fulfillment_mode" binding:"omitempty,oneof=delivery pickup both"`
	PickupLocation  *string  `json:"pickup_location"`
	DeliveryFee     *float64 `json:"delivery_fee" binding:"omitempty,min=0"`

	// Set sale_end to a past time to immediately end a sale without deactivating.
	SaleStart *time.Time `json:"sale_start"`
	SaleEnd   *time.Time `json:"sale_end"`
}

type CreateVariantDTO struct {
	SKU           string   `json:"sku" binding:"required,min=1,max=100"`
	Attributes    string   `json:"attributes" binding:"required"` // e.g. {"size":"L"} or {"size":"M","colour":"home"}
	StockQuantity int      `json:"stock_quantity" binding:"min=0"`
	PriceOverride *float64 `json:"price_override" binding:"omitempty,gt=0"`
}

type UpdateVariantDTO struct {
	StockQuantity *int     `json:"stock_quantity" binding:"omitempty,min=0"`
	PriceOverride *float64 `json:"price_override" binding:"omitempty,gt=0"`
	IsActive      *bool    `json:"is_active"`
}

// StockStatus returns a user-friendly stock level string.
// Call this after scanning a variant — used by the dedicated merch page.
func (v *MerchandiseVariant) StockStatus() string {
	switch {
	case v.StockQuantity == 0:
		return "sold_out"
	case v.StockQuantity <= 5:
		return "low_stock"
	default:
		return "available"
	}
}

// MerchandiseDetail is the full response for a single merch item page.
// It embeds the item and adds computed fields the list view doesn't need.
type MerchandiseDetail struct {
	Merchandise
	StockSummary MerchandiseStockSummary `json:"stock_summary"`
	SaleStatus   string                  `json:"sale_status"` // live | scheduled | ended
}

type MerchandiseStockSummary struct {
	TotalStock     int    `json:"total_stock"`
	AvailableCount int    `json:"available_variants"` // variants with stock > 0
	Status         string `json:"status"`             // available | low_stock | sold_out
}

// ComputeStockSummary fills in the StockSummary field from the attached variants.
func (d *MerchandiseDetail) ComputeStockSummary() {
	total := 0
	available := 0
	for _, v := range d.Variants {
		total += v.StockQuantity
		if v.StockQuantity > 0 {
			available++
		}
	}
	d.StockSummary.TotalStock = total
	d.StockSummary.AvailableCount = available
	switch {
	case total == 0:
		d.StockSummary.Status = "sold_out"
	case total <= 10:
		d.StockSummary.Status = "low_stock"
	default:
		d.StockSummary.Status = "available"
	}
	d.SaleStatus = d.Merchandise.SaleStatus()
}