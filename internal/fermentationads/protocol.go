// Package fermentationads implements the dedicated fermentation ADS V1 wire contract.
// It does not connect to a PLC or activate outputs.
package fermentationads

import (
	"encoding/binary"
	"fmt"
)

const (
	Version     = 1
	InputSize   = 16
	CommandSize = 24
	StatusSize  = 28
	AppliedSize = 24
)

type Owner byte

const (
	Safe Owner = iota
	UA
	ADS
)

type Outputs struct{ F1Cooling, F2Cooling, F1Heating, F2Heating, Pump bool }

func (o Outputs) interlocked() bool {
	return !(o.F1Cooling && o.F1Heating || o.F2Cooling && o.F2Heating || o.Pump && !o.F1Cooling && !o.F2Cooling)
}

type Command struct {
	Session, Sequence uint32
	Owner             Owner
	Outputs           Outputs
	Enable            bool
}
type Inputs struct {
	Sequence       uint32
	F1, F2, Glycol int16
}
type Status struct {
	Sequence, Session, AppliedSequence, Faults, AgeMS uint32
	WatchdogHealthy, Enabled, Valid                   bool
	Owner                                             Owner
}
type Applied struct {
	Sequence, Session, AppliedSequence uint32
	Outputs                            Outputs
	Enabled                            bool
}

func (c Command) MarshalBinary() ([]byte, error) {
	if c.Session == 0 || c.Sequence == 0 || c.Owner > ADS {
		return nil, fmt.Errorf("invalid command identity or owner")
	}
	if !c.Outputs.interlocked() {
		return nil, fmt.Errorf("command interlock")
	}
	b := make([]byte, CommandSize)
	binary.LittleEndian.PutUint16(b, Version)
	binary.LittleEndian.PutUint16(b[2:], CommandSize)
	binary.LittleEndian.PutUint32(b[4:], c.Session)
	binary.LittleEndian.PutUint32(b[8:], c.Sequence)
	b[12] = byte(c.Owner)
	for i, v := range []bool{c.Outputs.F1Cooling, c.Outputs.F2Cooling, c.Outputs.F1Heating, c.Outputs.F2Heating, c.Outputs.Pump, c.Enable} {
		if v {
			b[13+i] = 1
		}
	}
	return b, nil
}
func header(b []byte, size int) error {
	if len(b) != size || binary.LittleEndian.Uint16(b) != Version || int(binary.LittleEndian.Uint16(b[2:])) != size {
		return fmt.Errorf("invalid layout")
	}
	return nil
}
func stable(before, middle, after []byte, size int) error {
	for _, b := range [][]byte{before, middle, after} {
		if err := header(b, size); err != nil {
			return err
		}
	}
	seq := binary.LittleEndian.Uint32(middle[4:])
	if seq%2 != 0 || seq != binary.LittleEndian.Uint32(before[4:]) || seq != binary.LittleEndian.Uint32(after[4:]) {
		return fmt.Errorf("unstable snapshot")
	}
	return nil
}
func bools(b []byte) error {
	for _, v := range b {
		if v > 1 {
			return fmt.Errorf("invalid BOOL")
		}
	}
	return nil
}
func DecodeInputs(before, middle, after []byte) (Inputs, error) {
	if err := stable(before, middle, after, InputSize); err != nil {
		return Inputs{}, err
	}
	if middle[14] != 0 || middle[15] != 0 {
		return Inputs{}, fmt.Errorf("reserved input bytes")
	}
	return Inputs{binary.LittleEndian.Uint32(middle[4:]), int16(binary.LittleEndian.Uint16(middle[8:])), int16(binary.LittleEndian.Uint16(middle[10:])), int16(binary.LittleEndian.Uint16(middle[12:]))}, nil
}
func DecodeStatus(before, middle, after []byte) (Status, error) {
	if err := stable(before, middle, after, StatusSize); err != nil {
		return Status{}, err
	}
	if err := bools(middle[24:27]); err != nil {
		return Status{}, err
	}
	if middle[27] > byte(ADS) {
		return Status{}, fmt.Errorf("invalid owner")
	}
	return Status{binary.LittleEndian.Uint32(middle[4:]), binary.LittleEndian.Uint32(middle[8:]), binary.LittleEndian.Uint32(middle[12:]), binary.LittleEndian.Uint32(middle[16:]), binary.LittleEndian.Uint32(middle[20:]), middle[24] == 1, middle[25] == 1, middle[26] == 1, Owner(middle[27])}, nil
}
func DecodeApplied(before, middle, after []byte) (Applied, error) {
	if err := stable(before, middle, after, AppliedSize); err != nil {
		return Applied{}, err
	}
	if err := bools(middle[16:22]); err != nil {
		return Applied{}, err
	}
	if middle[22] != 0 || middle[23] != 0 {
		return Applied{}, fmt.Errorf("reserved applied bytes")
	}
	return Applied{binary.LittleEndian.Uint32(middle[4:]), binary.LittleEndian.Uint32(middle[8:]), binary.LittleEndian.Uint32(middle[12:]), Outputs{middle[16] == 1, middle[17] == 1, middle[18] == 1, middle[19] == 1, middle[20] == 1}, middle[21] == 1}, nil
}
