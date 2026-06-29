package ams

import (
	"bytes"
	"testing"
)

func TestHeaderEncodeDecode(t *testing.T) {
	// Arrange
	original := &Header{
		TargetNetId: NetId{1, 2, 3, 4, 5, 6},
		TargetPort:  851,

		SourceNetId: NetId{6, 5, 4, 3, 2, 1},
		SourcePort:  852,

		CommandID:  2,
		StateFlags: 4,
		DataLength: 12345,
		ErrorCode:  0,
		InvokeID:   99,
	}

	// Act
	encoded := original.Encode()

	// basic length check
	if len(encoded) != 32 {
		t.Fatalf("expected encoded length 32, got %d", len(encoded))
	}

	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	// Assert
	if !bytes.Equal(original.TargetNetId[:], decoded.TargetNetId[:]) {
		t.Errorf("TargetNetId mismatch")
	}
	if original.TargetPort != decoded.TargetPort {
		t.Errorf("TargetPort mismatch: %d != %d", original.TargetPort, decoded.TargetPort)
	}

	if !bytes.Equal(original.SourceNetId[:], decoded.SourceNetId[:]) {
		t.Errorf("SourceNetId mismatch")
	}
	if original.SourcePort != decoded.SourcePort {
		t.Errorf("SourcePort mismatch: %d != %d", original.SourcePort, decoded.SourcePort)
	}

	if original.CommandID != decoded.CommandID {
		t.Errorf("CommandID mismatch: %d != %d", original.CommandID, decoded.CommandID)
	}
	if original.StateFlags != decoded.StateFlags {
		t.Errorf("StateFlags mismatch: %d != %d", original.StateFlags, decoded.StateFlags)
	}
	if original.DataLength != decoded.DataLength {
		t.Errorf("DataLength mismatch: %d != %d", original.DataLength, decoded.DataLength)
	}
	if original.ErrorCode != decoded.ErrorCode {
		t.Errorf("ErrorCode mismatch: %d != %d", original.ErrorCode, decoded.ErrorCode)
	}
	if original.InvokeID != decoded.InvokeID {
		t.Errorf("InvokeID mismatch: %d != %d", original.InvokeID, decoded.InvokeID)
	}
}
