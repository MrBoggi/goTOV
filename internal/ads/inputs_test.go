package ads

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

// This is the valid-v1-even-sequence vector from
// goTOV-Interfaces/ads/v1/go/test-vectors.json. The contract remains its
// source of truth; the literal keeps the consumer's CI test self-contained.
const validV1Vector = "01003400020000000100feff0300fcff0500faff0700f8ff0900f6ff0b00f4ff0000803f000020c000005040000090c001000100"

func TestDecodeInputsV1ContractVector(t *testing.T) {
	data, err := hex.DecodeString(validV1Vector)
	require.NoError(t, err)

	inputs, err := decodeInputs(data)
	require.NoError(t, err)
	require.Equal(t, uint16(1), inputs.Version)
	require.Equal(t, uint16(52), inputs.StructureSize)
	require.Equal(t, uint32(2), inputs.Sequence)
	require.Equal(t, int16(1), inputs.HltTemp)
	require.Equal(t, int16(-2), inputs.MltTemp)
	require.Equal(t, int16(3), inputs.BkTemp)
	require.Equal(t, int16(-4), inputs.HeatExchWortTemp)
	require.Equal(t, int16(5), inputs.HeatExchWaterTemp)
	require.Equal(t, int16(-6), inputs.PhValue)
	require.Equal(t, int16(7), inputs.SupplyWaterTemp)
	require.Equal(t, int16(-8), inputs.ReturnWaterTemp)
	require.Equal(t, int16(9), inputs.SpecificGravity)
	require.Equal(t, int16(-10), inputs.Fermenter1Temp)
	require.Equal(t, int16(11), inputs.Fermenter2Temp)
	require.Equal(t, int16(-12), inputs.GlycolChillerTemp)
	require.Equal(t, float32(1), inputs.FlowHLT)
	require.Equal(t, float32(-2.5), inputs.FlowMLT)
	require.Equal(t, float32(3.25), inputs.FlowWort)
	require.Equal(t, float32(-4.5), inputs.MltRecircTemp)
	require.True(t, inputs.FlowHLTOutOfRange)
	require.False(t, inputs.FlowMLTOutOfRange)
	require.True(t, inputs.MltRecircTempOutOfRange)
}

func TestDecodeInputsRejectsContractViolations(t *testing.T) {
	data, err := hex.DecodeString(validV1Vector)
	require.NoError(t, err)

	tests := []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"length", func(data []byte) []byte { return data[:51] }},
		{"version", func(data []byte) []byte { data[0] = 2; return data }},
		{"structure size", func(data []byte) []byte { data[2] = 51; return data }},
		{"reserved byte", func(data []byte) []byte { data[51] = 1; return data }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			copyData := append([]byte(nil), data...)
			_, err := decodeInputs(test.mutate(copyData))
			require.Error(t, err)
		})
	}
}

func TestDecodeStableInputsRejectsUnstableAndOddSequences(t *testing.T) {
	data, err := hex.DecodeString(validV1Vector)
	require.NoError(t, err)

	_, err = decodeStableInputs(data, data, data)
	require.NoError(t, err)

	changed := append([]byte(nil), data...)
	changed[4] = 4
	_, err = decodeStableInputs(data, data, changed)
	require.Error(t, err)

	odd := append([]byte(nil), data...)
	odd[4] = 3
	_, err = decodeStableInputs(odd, odd, odd)
	require.Error(t, err)

	mismatchedSnapshot := append([]byte(nil), data...)
	mismatchedSnapshot[4] = 4
	_, err = decodeStableInputs(data, mismatchedSnapshot, data)
	require.Error(t, err)
}
