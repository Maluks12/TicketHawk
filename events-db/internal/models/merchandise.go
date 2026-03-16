package models

import "time"

type Merchandise struct {
	ID           string    `json:"id" db:"id"`
	EventID      string    `json:"event_id" db:"event_id"`
	Name         string    `json:"name" db:"name"`
	Description  *string   `json:"description,omitempty" db:"description"`
	BasePrice    float64   `json:"base_price" db:"base_price"`
	Category     *string   `json:"category,omitempty" db:"category"`
	DisplayOrder int       `json:"display_order" db:"display_order"`
	IsActive     bool      `json:"is_active" db:"is_active"`
	Images       *string   `json:"images,omitempty" db:"images"` // JSONB stored as string
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`

	// Joined — populated by GetMerchandiseByEvent, not a stored column
	Variants []MerchandiseVariant `json:"variants,omitempty" db:"-"`
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
	Name         string  `json:"name" binding:"required,min=1,max=255"`
	Description  *string `json:"description"`
	BasePrice    float64 `json:"base_price" binding:"required,gt=0"`
	Category     *string `json:"category"`
	DisplayOrder int     `json:"display_order"`
	Images       *string `json:"images"`
}

type UpdateMerchandiseDTO struct {
	Name         *string  `json:"name" binding:"omitempty,min=1,max=255"`
	Description  *string  `json:"description"`
	BasePrice    *float64 `json:"base_price" binding:"omitempty,gt=0"`
	Category     *string  `json:"category"`
	DisplayOrder *int     `json:"display_order"`
	IsActive     *bool    `json:"is_active"`
	Images       *string  `json:"images"`
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