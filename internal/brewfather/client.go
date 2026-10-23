package brewfather

import "net/http"

// baseURL is the root for all Brewfather v2 API calls.
const baseURL = "https://api.brewfather.app/v2"

// Client wraps credentials and an HTTP client for talking to Brewfather.
type Client struct {
	UserID string
	APIKey string
	HTTP   *http.Client
}

// NewClient constructs a new Brewfather client with a default http.Client.
func NewClient(userID, apiKey string) *Client {
	return &Client{
		UserID: userID,
		APIKey: apiKey,
		HTTP:   &http.Client{},
	}
}
