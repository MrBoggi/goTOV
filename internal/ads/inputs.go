// Package ads implements the optional, read-only brewery ADS V1 input reader.
package ads

import (
	"encoding/binary"
	"fmt"
	"math"
)

const (
	InputsSymbol  = "GVL_ADS.stInputs"
	InputsSize    = 52
	InputsVersion = 1
)

// Inputs is one validated V1 snapshot of GVL_ADS.stInputs. The integer values
// deliberately retain their PLC representation; their physical units are not
// part of the ADS V1 contract.
type Inputs struct {
	Version       uint16
	StructureSize uint16
	Sequence      uint32

	HltTemp           int16
	MltTemp           int16
	BkTemp            int16
	HeatExchWortTemp  int16
	HeatExchWaterTemp int16
	PhValue           int16
	SupplyWaterTemp   int16
	ReturnWaterTemp   int16
	SpecificGravity   int16
	Fermenter1Temp    int16
	Fermenter2Temp    int16
	GlycolChillerTemp int16

	FlowHLT                 float32
	FlowMLT                 float32
	FlowWort                float32
	MltRecircTemp           float32
	FlowHLTOutOfRange       bool
	FlowMLTOutOfRange       bool
	MltRecircTempOutOfRange bool
}

func decodeInputs(data []byte) (Inputs, error) {
	if len(data) != InputsSize {
		return Inputs{}, fmt.Errorf("ADS inputs length = %d, want %d", len(data), InputsSize)
	}

	inputs := Inputs{
		Version:                 binary.LittleEndian.Uint16(data[0:2]),
		StructureSize:           binary.LittleEndian.Uint16(data[2:4]),
		Sequence:                binary.LittleEndian.Uint32(data[4:8]),
		HltTemp:                 int16(binary.LittleEndian.Uint16(data[8:10])),
		MltTemp:                 int16(binary.LittleEndian.Uint16(data[10:12])),
		BkTemp:                  int16(binary.LittleEndian.Uint16(data[12:14])),
		HeatExchWortTemp:        int16(binary.LittleEndian.Uint16(data[14:16])),
		HeatExchWaterTemp:       int16(binary.LittleEndian.Uint16(data[16:18])),
		PhValue:                 int16(binary.LittleEndian.Uint16(data[18:20])),
		SupplyWaterTemp:         int16(binary.LittleEndian.Uint16(data[20:22])),
		ReturnWaterTemp:         int16(binary.LittleEndian.Uint16(data[22:24])),
		SpecificGravity:         int16(binary.LittleEndian.Uint16(data[24:26])),
		Fermenter1Temp:          int16(binary.LittleEndian.Uint16(data[26:28])),
		Fermenter2Temp:          int16(binary.LittleEndian.Uint16(data[28:30])),
		GlycolChillerTemp:       int16(binary.LittleEndian.Uint16(data[30:32])),
		FlowHLT:                 math.Float32frombits(binary.LittleEndian.Uint32(data[32:36])),
		FlowMLT:                 math.Float32frombits(binary.LittleEndian.Uint32(data[36:40])),
		FlowWort:                math.Float32frombits(binary.LittleEndian.Uint32(data[40:44])),
		MltRecircTemp:           math.Float32frombits(binary.LittleEndian.Uint32(data[44:48])),
		FlowHLTOutOfRange:       data[48] != 0,
		FlowMLTOutOfRange:       data[49] != 0,
		MltRecircTempOutOfRange: data[50] != 0,
	}

	if inputs.Version != InputsVersion {
		return Inputs{}, fmt.Errorf("ADS inputs version = %d, want %d", inputs.Version, InputsVersion)
	}
	if inputs.StructureSize != InputsSize {
		return Inputs{}, fmt.Errorf("ADS inputs structure size = %d, want %d", inputs.StructureSize, InputsSize)
	}
	if data[51] != 0 {
		return Inputs{}, fmt.Errorf("ADS inputs reserved byte = %d, want 0", data[51])
	}

	return inputs, nil
}

func decodeStableInputs(before, snapshot, after []byte) (Inputs, error) {
	if len(before) != InputsSize || len(after) != InputsSize {
		return Inputs{}, fmt.Errorf("ADS inputs sequence read has invalid length")
	}
	beforeSequence := binary.LittleEndian.Uint32(before[4:8])
	afterSequence := binary.LittleEndian.Uint32(after[4:8])
	if beforeSequence != afterSequence {
		return Inputs{}, fmt.Errorf("ADS inputs sequence changed during read: %d -> %d", beforeSequence, afterSequence)
	}
	if beforeSequence%2 != 0 {
		return Inputs{}, fmt.Errorf("ADS inputs sequence %d is odd", beforeSequence)
	}

	inputs, err := decodeInputs(snapshot)
	if err != nil {
		return Inputs{}, err
	}
	if inputs.Sequence != beforeSequence {
		return Inputs{}, fmt.Errorf("ADS inputs snapshot sequence = %d, want %d", inputs.Sequence, beforeSequence)
	}
	return inputs, nil
}
