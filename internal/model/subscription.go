package model

import (
	"encoding/json"
	"time"
)

// Subscription sources.
const (
	SourceRemote = "remote"
	SourceLocal  = "local"
)

// Subscription is a stored subscription entry.
type Subscription struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	DisplayName string          `json:"displayName"`
	Source      string          `json:"source"`
	URL         string          `json:"url"`
	Content     string          `json:"-"`
	Enabled     bool            `json:"enabled"`
	Sort        int             `json:"sort"`
	Process     json.RawMessage `json:"process"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// ValidateSource reports whether s is a known subscription source.
func ValidateSource(s string) bool {
	return s == SourceRemote || s == SourceLocal
}
