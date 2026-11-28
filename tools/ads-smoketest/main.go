// ads-smoketest performs read-only ADS checks against the brewery PLC.
// It intentionally contains no ADS write, route-registration, or PLC-change code.
package main

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"os"
	"strings"
	"time"

	ads "github.com/siyka-au/go-ads/v3"
	"github.com/siyka-au/go-ads/v3/ams"
)

const (
	plcIP             = "192.168.10.150"
	adsTCPPort        = 48898
	targetNetID       = "5.145.71.174.1.1"
	targetADSPort     = ams.Port(851)
	defaultLocalNetID = "192.168.10.115.1.1"
	localADSPort      = ams.Port(10500) // Logical AMS source port; not a published TCP port.
	fbUAGroup         = ams.Group(0x4040)
	fbUAOffset        = uint32(384664)
	fbUALength        = uint32(120)
)

type bufferField struct {
	name      string
	dataType  string
	offset    uint32
	length    uint32
	reference string
}

var fbUAFields = []bufferField{
	{name: "hltTemp", dataType: "INT", offset: 4, length: 2, reference: "-61"},
	{name: "flowHLT", dataType: "REAL", offset: 24, length: 4, reference: "-6.249523"},
	{name: "flowMLT", dataType: "REAL", offset: 28, length: 4, reference: "-6.249523"},
	{name: "flowWort", dataType: "REAL", offset: 32, length: 4, reference: "0"},
	{name: "mltResirkTemp", dataType: "REAL", offset: 36, length: 4, reference: "ca. 38.12"},
	{name: "pumpeHLT", dataType: "BOOL", offset: 40, length: 1, reference: "FALSE"},
	{name: "pumpeWort", dataType: "BOOL", offset: 41, length: 1, reference: "FALSE"},
}

func main() {
	os.Exit(run())
}

func run() int {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	target, err := ams.NewAddress(targetNetID, targetADSPort)
	if err != nil {
		logger.Error("invalid target AMS address", "error", err)
		return 1
	}
	localNetID := strings.TrimSpace(os.Getenv("ADS_LOCAL_NETID"))
	if localNetID == "" {
		localNetID = defaultLocalNetID
	}
	local, err := ams.NewAddress(localNetID, localADSPort)
	if err != nil {
		logger.Error("invalid local AMS Net ID", "net_id", localNetID, "error", err)
		return 1
	}
	logger.Info("using local AMS Net ID", "net_id", local.NetID, "ams_address", local)

	sess, err := ads.NewSession(ctx, ads.Endpoint{
		Host:   plcIP,
		Port:   adsTCPPort,
		Target: target,
	},
		ads.WithLocalAddress(local),
		ads.WithRequestTimeout(8*time.Second),
		ads.WithAutoReconnect(false),
		ads.WithLogger(logger),
	)
	if err != nil {
		logger.Error("ADS session configuration failed", "error", classifyError(err))
		return 1
	}
	defer func() {
		if err := sess.Close(); err != nil {
			logger.Warn("ADS session cleanup failed", "error", classifyError(err))
		}
	}()

	logger.Info("Test A: connecting over ADS/TCP", "target_ip", plcIP, "tcp_port", adsTCPPort, "target_ams", target, "local_ams", local)
	if err := sess.Connect(ctx); err != nil {
		logger.Error("Test A failed: ADS connection", "error", classifyError(err))
		return 1
	}
	info := sess.Info()
	logger.Info("Test A passed: ADS session connected", "local_ams", info.Local, "local_tcp", info.LocalTCP, "target_ams", info.Target, "session_state", info.State)

	logger.Info("Test B: ADS ReadState", "target_ams", info.Target)
	state, err := sess.Client().ReadState(ctx)
	if err != nil {
		logger.Error("Test B failed: ADS ReadState", "error", classifyError(err))
		return 1
	}
	logger.Info("Test B passed: ADS ReadState", "runtime_state", state.State, "runtime_state_code", uint16(state.State), "device_state", state.DeviceState, "ads_return_code", "0 (0x00000000)")

	logger.Info("Test C: ADS symbol lookup", "symbol", "MAIN.fbUA")
	if _, err := sess.Symbol(ctx, "MAIN.fbUA"); err != nil {
		logger.Error("Test C failed: ADS symbol lookup", "symbol", "MAIN.fbUA", "error", classifyError(err))
		return 1
	}
	logger.Info("Test C passed: ADS symbol lookup", "symbol", "MAIN.fbUA")

	logger.Info("Test D: one raw ADS read of MAIN.fbUA", "group", fmt.Sprintf("0x%04X", uint32(fbUAGroup)), "offset", fbUAOffset, "length", fbUALength)
	data, err := sess.Client().Read(ctx, fbUAGroup, fbUAOffset, fbUALength)
	if err != nil {
		logger.Error("Test D failed: raw ADS read of MAIN.fbUA", "error", classifyError(err))
		return 1
	}
	if len(data) != int(fbUALength) {
		logger.Error("Test D failed: unexpected MAIN.fbUA buffer length", "expected", fbUALength, "received", len(data))
		return 1
	}
	for _, field := range fbUAFields {
		end := field.offset + field.length
		if end > uint32(len(data)) {
			logger.Error("Test D failed: TPY field exceeds MAIN.fbUA buffer", "field", field.name, "offset", field.offset, "length", field.length, "buffer_length", len(data))
			return 1
		}
		raw := data[field.offset:end]
		value, err := decodeRawValue(field.dataType, raw)
		if err != nil {
			logger.Error("Test D failed: MAIN.fbUA field decode", "field", field.name, "error", err)
			return 1
		}
		logger.Info("Test D decoded MAIN.fbUA field", "field", field.name, "data_type", field.dataType, "relative_offset", field.offset, "bytes", len(raw), "raw_hex", "0x"+hex.EncodeToString(raw), "decoded_value", value, "reference_value", field.reference)
	}
	logger.Info("Test D passed: raw ADS read of MAIN.fbUA", "bytes", len(data), "ads_return_code", "0 (0x00000000)")
	logger.Info("ADS structure-read smoke test passed", "mandatory_tests", "A,B,C,D")
	return 0
}

func decodeRawValue(dataType string, data []byte) (any, error) {
	switch dataType {
	case "INT":
		if len(data) != 2 {
			return nil, fmt.Errorf("INT requires 2 bytes, got %d", len(data))
		}
		return int16(binary.LittleEndian.Uint16(data)), nil
	case "REAL":
		if len(data) != 4 {
			return nil, fmt.Errorf("REAL requires 4 bytes, got %d", len(data))
		}
		return math.Float32frombits(binary.LittleEndian.Uint32(data)), nil
	case "BOOL":
		if len(data) != 1 {
			return nil, fmt.Errorf("BOOL requires 1 byte, got %d", len(data))
		}
		return data[0] != 0, nil
	default:
		return nil, fmt.Errorf("unsupported TPY datatype %q", dataType)
	}
}

func classifyError(err error) string {
	var routerErr ams.RouterError
	if errors.As(err, &routerErr) {
		return fmt.Sprintf("AMS router rejection (request did not reach the PLC runtime): code %d (0x%08X): %s", uint32(routerErr.Code), uint32(routerErr.Code), routerErr.Code)
	}

	var code ams.ReturnCode
	if errors.As(err, &code) {
		return fmt.Sprintf("PLC runtime ADS response: code %d (0x%08X): %s", uint32(code), uint32(code), code)
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return "TCP/ADS timeout (not alone proof of an ADS-route failure): " + err.Error()
		}
	}

	message := err.Error()
	switch {
	case strings.Contains(message, "connection refused"):
		return "TCP connection refused: " + message
	case strings.Contains(message, "symbol") && strings.Contains(message, "not found"):
		return "ADS symbol not found: " + message
	case strings.Contains(message, "access") || strings.Contains(message, "permission"):
		return "ADS permission/access error: " + message
	case strings.Contains(message, "decode") || strings.Contains(message, "datatype"):
		return "ADS datatype/decode error: " + message
	case strings.Contains(message, "route") || strings.Contains(message, "NetID"):
		return "possible ADS routing/addressing error: " + message
	default:
		return "ADS/transport error: " + message
	}
}
