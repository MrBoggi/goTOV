package brewfather

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type RecipeListItem struct {
	ID   string `json:"_id"`
	Name string `json:"name"`
}

func (c *Client) ListRecipes() ([]RecipeListItem, error) {
	endpoint := fmt.Sprintf("%s/recipes", baseURL)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Basic auth — nå med riktig felt-navn
	req.SetBasicAuth(c.UserID, c.APIKey)

	// Query params
	q := url.Values{}
	q.Set("include", "name")
	req.URL.RawQuery = q.Encode()

	// Bruk riktig HTTP-klient
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("brewfather returned %d: %s", resp.StatusCode, string(body))
	}

	var list []RecipeListItem
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	return list, nil
}
