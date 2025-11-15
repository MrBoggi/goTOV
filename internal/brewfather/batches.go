package brewfather

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
)

// FetchBatch fetches a single batch (including recipe + fermentation snapshot).
func (c *Client) FetchBatch(batchID string) (*BrewfatherBatch, error) {
    endpoint := fmt.Sprintf("%s/batches/%s", baseURL, batchID)

    req, err := http.NewRequest("GET", endpoint, nil)
    if err != nil {
        return nil, fmt.Errorf("create request: %w", err)
    }

    req.SetBasicAuth(c.UserID, c.APIKey)

    // include fermentation + recipe snapshot
    q := req.URL.Query()
    q.Set("include", "recipe,fermentation")
    req.URL.RawQuery = q.Encode()

    resp, err := c.HTTP.Do(req)
    if err != nil {
        return nil, fmt.Errorf("http error: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("brewfather returned %d: %s", resp.StatusCode, string(body))
    }

    var batch BrewfatherBatch
    if err := json.NewDecoder(resp.Body).Decode(&batch); err != nil {
        return nil, fmt.Errorf("decode batch: %w", err)
    }

    return &batch, nil
}

// FetchBatches fetches a list of batches (with embedded recipe + fermentation snapshots).
func (c *Client) FetchBatches() ([]BrewfatherBatch, error) {
    endpoint := fmt.Sprintf("%s/batches", baseURL)

    req, err := http.NewRequest("GET", endpoint, nil)
    if err != nil {
        return nil, fmt.Errorf("create request: %w", err)
    }

    // BasicAuth = Brewfather API v2
    req.SetBasicAuth(c.UserID, c.APIKey)

    // include snapshots of recipe + fermentation profile
    q := req.URL.Query()
    q.Set("include", "recipe,fermentation")
    q.Set("limit", "200")
    req.URL.RawQuery = q.Encode()

    resp, err := c.HTTP.Do(req)
    if err != nil {
        return nil, fmt.Errorf("http error: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("brewfather returned %d: %s", resp.StatusCode, string(body))
    }

    var batches []BrewfatherBatch
    if err := json.NewDecoder(resp.Body).Decode(&batches); err != nil {
        return nil, fmt.Errorf("decode batches: %w", err)
    }

    return batches, nil
}
