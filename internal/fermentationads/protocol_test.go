package fermentationads

import (
	"encoding/binary"
	"encoding/hex"
	"testing"
)

func TestCommandContractVectorAndInterlocks(t *testing.T) {
	got, err := (Command{Session: 0x11223344, Sequence: 1, Owner: ADS}).MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got) != "010018004433221101000000020000000000000000000000" {
		t.Fatalf("unexpected vector: %x", got)
	}
	for _, c := range []Command{{Sequence: 1, Owner: ADS}, {Session: 1, Owner: ADS}, {Session: 1, Sequence: 1, Owner: 3}, {Session: 1, Sequence: 1, Owner: ADS, Outputs: Outputs{Pump: true}}, {Session: 1, Sequence: 1, Owner: ADS, Outputs: Outputs{F1Cooling: true, F1Heating: true}}, {Session: 1, Sequence: 1, Owner: ADS, Outputs: Outputs{F2Cooling: true, F2Heating: true}}} {
		if _, err := c.MarshalBinary(); err == nil {
			t.Fatalf("accepted invalid command %+v", c)
		}
	}
}
func snapshot(size int) []byte {
	b := make([]byte, size)
	binary.LittleEndian.PutUint16(b, Version)
	binary.LittleEndian.PutUint16(b[2:], uint16(size))
	binary.LittleEndian.PutUint32(b[4:], 2)
	return b
}
func TestSnapshotsFailClosed(t *testing.T) {
	in := snapshot(InputSize)
	binary.LittleEndian.PutUint16(in[8:], uint16(0xff9c))
	v, err := DecodeInputs(in, in, in)
	if err != nil || v.F1 != -100 {
		t.Fatalf("input %v %v", v, err)
	}
	bad := append([]byte(nil), in...)
	bad[4] = 3
	if _, err = DecodeInputs(in, bad, in); err == nil {
		t.Fatal("accepted odd sequence")
	}
	bad = append([]byte(nil), in...)
	bad[14] = 1
	if _, err = DecodeInputs(in, bad, in); err == nil {
		t.Fatal("accepted reserved byte")
	}
	status := snapshot(StatusSize)
	status[24] = 1
	status[26] = 1
	status[27] = byte(ADS)
	s, err := DecodeStatus(status, status, status)
	if err != nil || !s.Valid || !s.WatchdogHealthy || s.Owner != ADS {
		t.Fatalf("status %v %v", s, err)
	}
	status[25] = 2
	if _, err = DecodeStatus(status, status, status); err == nil {
		t.Fatal("accepted invalid bool")
	}
	applied := snapshot(AppliedSize)
	applied[16] = 1
	a, err := DecodeApplied(applied, applied, applied)
	if err != nil || !a.Outputs.F1Cooling {
		t.Fatalf("applied %v %v", a, err)
	}
	applied[23] = 1
	if _, err = DecodeApplied(applied, applied, applied); err == nil {
		t.Fatal("accepted reserved byte")
	}
	if _, err = DecodeInputs(nil, nil, nil); err == nil {
		t.Fatal("accepted short input")
	}
}
