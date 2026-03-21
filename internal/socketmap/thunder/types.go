package thunder

import (
	"time"
)

// Auth holds Thunder authentication state
type Auth struct {
	DevelopAppID string
	FlowID       string
	BearerToken  string
	ExpiresAt    time.Time
	LastRefresh  time.Time
}

// FlowStartResponse represents the response from flow start
type FlowStartResponse struct {
	FlowID string `json:"flowId"`
}

// FlowCompleteResponse represents the response from flow completion
type FlowCompleteResponse struct {
	Assertion string `json:"assertion"`
}

// OrgUnitResponse represents an organization unit from Thunder
type OrgUnitResponse struct {
	ID          string  `json:"id"`
	Handle      string  `json:"handle"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Parent      *string `json:"parent"`
}

// UsersResponse represents the response from Thunder Users API
type UsersResponse struct {
	TotalResults int           `json:"totalResults"`
	StartIndex   int           `json:"startIndex"`
	Count        int           `json:"count"`
	Users        []User        `json:"users"`
	Links        []interface{} `json:"links"`
}

// User represents a Thunder user
type User struct {
	ID               string                 `json:"id"`
	OrganizationUnit string                 `json:"organizationUnit"`
	Type             string                 `json:"type"`
	Attributes       map[string]interface{} `json:"attributes"`
}
