package brewfather

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
)

// FetchRecipe fetches a single recipe by ID.
func (c *Client) FetchRecipe(recipeID string) (*BrewfatherRecipe, error) {
    endpoint := fmt.Sprintf("%s/recipes/%s", baseURL, recipeID)

    req, err := http.NewRequest("GET", endpoint, nil)
    if err != nil {
        return nil, fmt.Errorf("create request: %w", err)
    }

    // Brewfather v2 uses basic auth (userId/API key)
    req.SetBasicAuth(c.UserID, c.APIKey)

    resp, err := c.HTTP.Do(req)
    if err != nil {
        return nil, fmt.Errorf("http error: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("brewfather returned %d: %s", resp.StatusCode, string(body))
    }

    var recipe BrewfatherRecipe
    if err := json.NewDecoder(resp.Body).Decode(&recipe); err != nil {
        return nil, fmt.Errorf("decode recipe: %w", err)
    }

    return &recipe, nil
}
