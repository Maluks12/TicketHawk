package utils

import (
	"encoding/base64"
	"encoding/json"
	"time"
)

type Cursor struct {
	FeaturedUntil time.Time `json:"featured_until"`
	StartTime time.Time `json:"start_time"`
	ID        string    `json:"id"`
	CreatedAt     time.Time  `json:"created_at,omitempty"`
	Views         int        `json:"views,omitempty"`
	// Relevance carries the trigram score of the last row on the previous
	// page. Only SearchEvents sets it; omitempty keeps every other cursor
	// payload byte-identical to before.
	Relevance     float64    `json:"relevance,omitempty"`
}

func EncodeCursor(c *Cursor) string {
	if c == nil {
		return ""
	}
	b, _ := json.Marshal(c)
	return base64.StdEncoding.EncodeToString(b)
}

func DecodeCursor(s string) (*Cursor, error) {
	if s == "" {
		return nil, nil
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	var c Cursor
	err = json.Unmarshal(b, &c)
	return &c, err
}