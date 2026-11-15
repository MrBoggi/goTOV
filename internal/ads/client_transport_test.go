package ads

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/MrBoggi/goTOV/internal/config"
	"github.com/stretchr/testify/require"
)

const fakeInputHandle uint32 = 0x12345678

type fakeADS struct {
	advanceCycles bool
	size          uint32
	handle        uint32
	metadataError bool
	readErrorAt   int
	shortReadAt   int
	reads         int
	calls         []string
}

func words(values ...uint32) []byte {
	b := make([]byte, 4*len(values))
	for i, v := range values {
		binary.LittleEndian.PutUint32(b[4*i:], v)
	}
	return b
}

// reply is deliberately an allowlist, including port, payload and read length.
// Any unrecognised service (including process writes/WriteControl) fails the test.
func (f *fakeADS) reply(port, cmd uint16, p []byte) ([]byte, error) {
	if cmd == 4 && (port == 851 || port == 10000) && len(p) == 0 {
		f.calls = append(f.calls, fmt.Sprintf("state:%d", port))
		return []byte{0, 0, 0, 0, 5, 0, 0, 0}, nil // RUN
	}
	if port != 851 || len(p) < 12 {
		return nil, fmt.Errorf("forbidden port/service %d/%d", port, cmd)
	}
	group, offset, length := binary.LittleEndian.Uint32(p), binary.LittleEndian.Uint32(p[4:]), binary.LittleEndian.Uint32(p[8:])
	switch {
	case cmd == 2 && group == 0xF008 && offset == 0 && length == 1 && len(p) == 12:
		f.calls = append(f.calls, "version")
		return append(words(0, 1), 1), nil
	case cmd == 9 && offset == 0 && len(p) >= 16:
		name := append([]byte(InputsSymbol), 0)
		if binary.LittleEndian.Uint32(p[12:]) != uint32(len(name)) || !bytes.Equal(p[16:], name) {
			break
		}
		if group == 0xF009 && length == 2048 {
			f.calls = append(f.calls, "metadata")
			if f.metadataError {
				return words(0x710, 0), nil
			}
			// ADS symbol entry header (30 bytes), name, type, comment; no process offset used.
			typ := []byte("ST_BreweryInputs")
			b := append(words(uint32(30+len(name)+len(typ)+2), 0x4020, 99, f.size, 65, 0), make([]byte, 6)...)
			binary.LittleEndian.PutUint16(b[24:], uint16(len(name)-1))
			binary.LittleEndian.PutUint16(b[26:], uint16(len(typ)))
			b = append(b, name...)
			b = append(b, typ...)
			b = append(b, 0, 0)
			return append(words(0, uint32(len(b))), b...), nil
		}
		if group == 0xF003 && length == 4 {
			f.calls = append(f.calls, "handle")
			return words(0, 4, f.handle), nil
		}
	case cmd == 2 && group == 0xF005 && offset == fakeInputHandle && length == InputsSize && len(p) == 12:
		f.reads++
		f.calls = append(f.calls, "snapshot")
		if f.reads == f.readErrorAt {
			return words(0x701, 0), nil
		}
		b, err := hex.DecodeString(validV1Vector)
		if err != nil {
			return nil, err
		}
		if f.advanceCycles {
			binary.LittleEndian.PutUint32(b[4:], uint32(2*((f.reads-1)/3+1)))
		}
		if f.reads == f.shortReadAt {
			b = b[:len(b)-1]
		}
		return append(words(0, uint32(len(b))), b...), nil
	case cmd == 3 && group == 0xF006 && offset == 0 && length == 4 && bytes.Equal(p[12:], words(fakeInputHandle)):
		f.calls = append(f.calls, "release")
		return words(0), nil
	}
	return nil, fmt.Errorf("forbidden ADS service cmd=%d group=%#x offset=%#x payload=%x", cmd, group, offset, p)
}

func (f *fakeADS) serve(listener net.Listener) (retErr error) {
	conn, err := listener.Accept()
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, conn.Close()) }()
	if err = conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	for {
		header := make([]byte, 6)
		if _, err = io.ReadFull(conn, header); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		n := binary.LittleEndian.Uint32(header[2:])
		if n < 32 || n > 4096 || binary.LittleEndian.Uint16(header) != 0 {
			return fmt.Errorf("invalid AMS/TCP frame")
		}
		frame := make([]byte, n)
		if _, err = io.ReadFull(conn, frame); err != nil {
			return err
		}
		if binary.LittleEndian.Uint32(frame[20:]) != n-32 {
			return fmt.Errorf("invalid AMS data length")
		}
		data, err := f.reply(binary.LittleEndian.Uint16(frame[6:]), binary.LittleEndian.Uint16(frame[16:]), frame[32:])
		if err != nil {
			return err
		}
		response := make([]byte, 38+len(data))
		binary.LittleEndian.PutUint32(response[2:], uint32(len(response)-6))
		copy(response[6:14], frame[8:16])
		copy(response[14:22], frame[:8])
		copy(response[22:24], frame[16:18])
		binary.LittleEndian.PutUint16(response[24:], 5)
		binary.LittleEndian.PutUint32(response[26:], uint32(len(data)))
		copy(response[34:38], frame[28:32])
		copy(response[38:], data)
		if _, err = conn.Write(response); err != nil {
			return err
		}
	}
}

func TestClientTransport(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		size, handle             uint32
		metadataError            bool
		readErrorAt, shortReadAt int
		lookupError              string
	}{
		{name: "lifecycle", size: 52, handle: fakeInputHandle},
		{name: "wrong symbol size", size: 51, handle: fakeInputHandle, lookupError: "size = 51"},
		{name: "zero handle", size: 52, lookupError: "has no handle"},
		{name: "metadata service error", metadataError: true, lookupError: "lookup ADS symbol"},
		{name: "first read error", size: 52, handle: fakeInputHandle, readErrorAt: 1},
		{name: "middle read error", size: 52, handle: fakeInputHandle, readErrorAt: 2},
		{name: "last read error", size: 52, handle: fakeInputHandle, readErrorAt: 3},
		{name: "short snapshot", size: 52, handle: fakeInputHandle, shortReadAt: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			require.NoError(t, err)
			defer func() { require.NoError(t, listener.Close()) }()
			f := &fakeADS{size: tc.size, handle: tc.handle, metadataError: tc.metadataError, readErrorAt: tc.readErrorAt, shortReadAt: tc.shortReadAt}
			done := make(chan error, 1)
			go func() { done <- f.serve(listener) }()
			client, err := NewClient(config.ADSConfig{Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, TargetNetID: "1.2.3.4.1.1", LocalNetID: "1.2.3.5.1.1", TargetPort: 851, LocalPort: 10500, RequestTimeout: "1s", PollInterval: "5s"})
			require.NoError(t, err)
			// Close still runs if an assertion fails, and retains normal SDK cleanup.
			defer func() { require.NoError(t, client.Close()) }()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			require.NoError(t, client.Connect(ctx))
			require.NoError(t, client.ReadState(ctx))
			err = client.LookupInputs(ctx)
			expected := []string{"version", "state:10000", "state:851", "metadata"}
			if !tc.metadataError {
				expected = append(expected, "handle")
			}
			if tc.lookupError != "" {
				require.ErrorContains(t, err, tc.lookupError)
			} else {
				require.NoError(t, err)
				sample, readErr := client.ReadInputsSnapshot(ctx)
				count := 3
				if tc.readErrorAt != 0 {
					count = tc.readErrorAt
					require.ErrorContains(t, readErr, "read ADS symbol")
				} else if tc.shortReadAt != 0 {
					require.ErrorContains(t, readErr, "length")
				} else {
					require.NoError(t, readErr)
					require.Equal(t, uint32(2), sample.Sequence)
				}
				for i := 0; i < count; i++ {
					expected = append(expected, "snapshot")
				}
			}
			require.NoError(t, client.Close())
			select {
			case err = <-done:
				require.NoError(t, err)
			case <-time.After(2 * time.Second):
				t.Fatal("fake server did not observe connection close")
			}
			if tc.handle != 0 {
				expected = append(expected, "release")
			}
			require.Equal(t, expected, f.calls)
		})
	}
}

func TestClientTransportRejectsWrites(t *testing.T) {
	for _, p := range [][]byte{
		append(words(0xF005, fakeInputHandle, 4), words(1)...), // process handle write
		append(words(0x4020, 0, 4), words(1)...),               // raw process memory write
		append(words(0xF006, 0, 4), words(999)...),             // wrong release handle

	} {
		for _, cmd := range []uint16{3, 5, 9} {
			_, err := (&fakeADS{}).reply(851, cmd, p)
			require.Error(t, err)
		}
	}
}

func TestClientTransportRejectsOutputLookup(t *testing.T) {
	for _, name := range []string{"GVL_ADS.stOutputs", "GVL_ADS.udOutputCommitSequence"} {
		payload := append([]byte(name), 0)
		for _, group := range []uint32{0xF003, 0xF009} {
			length := uint32(4)
			if group == 0xF009 {
				length = 2048
			}
			_, err := (&fakeADS{}).reply(851, 9, append(words(group, 0, length, uint32(len(payload))), payload...))
			require.Error(t, err)
		}
	}
}
