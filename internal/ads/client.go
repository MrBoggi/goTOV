package ads

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MrBoggi/goTOV/internal/config"
	adsclient "github.com/siyka-au/go-ads/v3"
	"github.com/siyka-au/go-ads/v3/ams"
)

// Client is a read-only client for the V1 GVL_ADS.stInputs contract.
// It has no methods that write to the PLC.
type Client struct {
	session *adsclient.Session

	inputsHandle uint32
	pollInterval time.Duration
}

// NewClient validates the ADS configuration and creates a session without
// connecting to the PLC. Route registration is deliberately not configured;
// the PLC route must already exist.
func NewClient(cfg config.ADSConfig) (*Client, error) {
	if strings.TrimSpace(cfg.Host) == "" {
		return nil, fmt.Errorf("ADS host is required when ADS is enabled")
	}
	if strings.TrimSpace(cfg.TargetNetID) == "" {
		return nil, fmt.Errorf("ADS target_net_id is required when ADS is enabled")
	}
	if strings.TrimSpace(cfg.LocalNetID) == "" {
		return nil, fmt.Errorf("ADS local_net_id is required when ADS is enabled")
	}
	if cfg.Port <= 0 {
		return nil, fmt.Errorf("ADS port must be positive")
	}

	target, err := ams.NewAddress(cfg.TargetNetID, ams.Port(cfg.TargetPort))
	if err != nil {
		return nil, fmt.Errorf("invalid ADS target address: %w", err)
	}
	local, err := ams.NewAddress(cfg.LocalNetID, ams.Port(cfg.LocalPort))
	if err != nil {
		return nil, fmt.Errorf("invalid ADS local address: %w", err)
	}
	requestTimeout, err := time.ParseDuration(cfg.RequestTimeout)
	if err != nil || requestTimeout <= 0 {
		return nil, fmt.Errorf("invalid ADS request_timeout %q", cfg.RequestTimeout)
	}
	pollInterval, err := time.ParseDuration(cfg.PollInterval)
	if err != nil || pollInterval <= 0 {
		return nil, fmt.Errorf("invalid ADS poll_interval %q", cfg.PollInterval)
	}

	session, err := adsclient.NewSession(context.Background(), adsclient.Endpoint{
		Host:   cfg.Host,
		Port:   cfg.Port,
		Target: target,
	}, adsclient.WithLocalAddress(local), adsclient.WithRequestTimeout(requestTimeout), adsclient.WithAutoReconnect(false), adsclient.WithSkipRouteRegistration())
	if err != nil {
		return nil, fmt.Errorf("create ADS session: %w", err)
	}

	return &Client{session: session, pollInterval: pollInterval}, nil
}

func (c *Client) Connect(ctx context.Context) error {
	if err := c.session.Connect(ctx); err != nil {
		return fmt.Errorf("connect ADS: %w", err)
	}
	return nil
}

func (c *Client) Close() error { return c.session.Close() }

func (c *Client) PollInterval() time.Duration { return c.pollInterval }

// ReadState checks the connected target state without modifying it.
func (c *Client) ReadState(ctx context.Context) error {
	if _, err := c.session.Client().ReadState(ctx); err != nil {
		return fmt.Errorf("read ADS state: %w", err)
	}
	return nil
}

// LookupInputs resolves the V1 input symbol through the ADS symbol service.
// Only its PLC-reported size is retained; no PLC memory offset is used.
func (c *Client) LookupInputs(ctx context.Context) error {
	symbol, err := c.session.Symbol(ctx, InputsSymbol)
	if err != nil {
		return fmt.Errorf("lookup ADS symbol %q: %w", InputsSymbol, err)
	}
	if symbol.Length != InputsSize {
		return fmt.Errorf("ADS symbol %q size = %d, want %d", InputsSymbol, symbol.Length, InputsSize)
	}
	if symbol.Handle == 0 {
		return fmt.Errorf("ADS symbol %q has no handle", InputsSymbol)
	}
	c.inputsHandle = symbol.Handle
	return nil
}

// ReadInputsSnapshot reads the whole symbol by its ADS symbol handle three
// times. The outer reads prove that the middle V1 snapshot was not observed
// while the PLC publisher held an odd sequence number.
func (c *Client) ReadInputsSnapshot(ctx context.Context) (Inputs, error) {
	if c.inputsHandle == 0 {
		return Inputs{}, fmt.Errorf("ADS inputs symbol has not been looked up")
	}
	read := func() ([]byte, error) {
		data, err := c.session.Client().Read(ctx, ams.GroupSymbolValueByHandle, c.inputsHandle, InputsSize)
		if err != nil {
			return nil, fmt.Errorf("read ADS symbol %q: %w", InputsSymbol, err)
		}
		return data, nil
	}
	before, err := read()
	if err != nil {
		return Inputs{}, err
	}
	snapshot, err := read()
	if err != nil {
		return Inputs{}, err
	}
	after, err := read()
	if err != nil {
		return Inputs{}, err
	}
	return decodeStableInputs(before, snapshot, after)
}
