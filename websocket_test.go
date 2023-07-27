package yves

import (
	"bufio"
	"bytes"
	"testing"
)

var testCasesWrite = []struct {
	name        string
	input       *WebsocketFragment
	expected    []byte
	expectedErr error
}{
	{
		name: "Valid Input",
		input: &WebsocketFragment{
			FinBit:        true,
			Rsv1:          false,
			Rsv2:          false,
			Rsv3:          false,
			OpCode:        0x02,
			MaskBit:       true,
			PayloadLength: 11,
			Key:           []byte{0x1A, 0x2B, 0x3C, 0x4D},
			Data:          []byte{0x68, 0x65, 0x6C, 0x6C, 0x6F, 0x20, 0x77, 0x6F, 0x72, 0x6C, 0x6c},
		},
		expected: []byte{
			0x82,                   // First byte: 10000010
			0x8B,                   // Second byte: 10000101
			0x1A, 0x2B, 0x3C, 0x4D, // Masking key: 26, 43, 60, 77
			0x72, 0x4E, 0x50, 0x21, 0x75, 0x0B, 0x4B, 0x22, 0x68, 0x47, 0x50,
		},
		expectedErr: nil,
	},
	{
		name: "Invalid key length",
		input: &WebsocketFragment{
			FinBit:        true,
			Rsv1:          false,
			Rsv2:          false,
			Rsv3:          false,
			OpCode:        0x02,
			MaskBit:       true,
			PayloadLength: 11,
			Key:           []byte{0x1A, 0x2B, 0x3C, 0x4D, 0x00},
			Data:          []byte{0x68, 0x65, 0x6C, 0x6C, 0x6F, 0x20, 0x77, 0x6F, 0x72, 0x6C, 0x6c},
		},
		expected: []byte{
			0x82,                   // First byte: 10000010
			0x8B,                   // Second byte: 10000101
			0x1A, 0x2B, 0x3C, 0x4D, // Masking key: 26, 43, 60, 77
			0x72, 0x4E, 0x50, 0x21, 0x75, 0x0B, 0x4B, 0x22, 0x68, 0x47, 0x50,
		},
		expectedErr: ErrorMaskKeyLength,
	},
}

var testCasesRead = []struct {
	name        string
	input       []byte
	expected    *WebsocketFragment
	expectedErr error
}{
	{
		name: "Valid Input",
		input: []byte{
			0x82,                   // First byte: 10000010
			0x8B,                   // Second byte: 10000101
			0x1A, 0x2B, 0x3C, 0x4D, // Masking key: 26, 43, 60, 77
			0x72, 0x4E, 0x50, 0x21, 0x75, 0x0B, 0x4B, 0x22, 0x68, 0x47, 0x50,
		},
		expected: &WebsocketFragment{
			FinBit:        true,
			Rsv1:          false,
			Rsv2:          false,
			Rsv3:          false,
			OpCode:        0x02,
			MaskBit:       true,
			PayloadLength: 11,
			Key:           []byte{0x1A, 0x2B, 0x3C, 0x4D},
			Data:          []byte{0x68, 0x65, 0x6C, 0x6C, 0x6F, 0x20, 0x77, 0x6F, 0x72, 0x6C, 0x6c},
		},
		expectedErr: nil,
	},
}

func TestReadWebsocketFragment(t *testing.T) {

	for _, tc := range testCasesRead {
		t.Run(tc.name, func(t *testing.T) {
			b := bufio.NewReader(bytes.NewReader(tc.input))
			result, err := ReadWebsocketFragment(b)

			if err != tc.expectedErr {
				t.Errorf("this was the input: %v", tc.input)
				t.Errorf("Expected error: %v, but got: %v", tc.expectedErr, err)
			}

			if !compareWebsocketFragments(result, tc.expected) {
				t.Errorf("Expected: %v, but got: %v", tc.expected, result)
			}
		})
	}
}

func TestWriteWebsocketFragment(t *testing.T) {
	for _, tc := range testCasesWrite {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			foo := bufio.NewWriter(&b)
			error := tc.input.Write(foo)
			if error != tc.expectedErr {
				t.Errorf("Expected: %s but got %s", tc.expectedErr.Error(), error.Error())
			}
			foo.Flush()

			// bb := b.Bytes()
			// if len(bb) == len(tc.expected) {
			// 	for i, v := range bb {
			// 		if v != tc.expected[i] {
			// 			t.Errorf("Expected: %v, but got: %v", tc.expected, bb)
			// 		}
			// 	}
			// } else {
			// 	t.Errorf("Different length: %d, but got: %d", len(tc.expected), len(bb))
			// }
		})
	}
}

func compareWebsocketFragments(a, b *WebsocketFragment) bool {
	if a == nil || b == nil {
		return a == b
	}

	if a.FinBit != b.FinBit || a.Rsv1 != b.Rsv1 || a.Rsv2 != b.Rsv2 || a.Rsv3 != b.Rsv3 ||
		a.OpCode != b.OpCode || a.MaskBit != b.MaskBit || a.PayloadLength != b.PayloadLength ||
		!bytes.Equal(a.Key, b.Key) || !bytes.Equal(a.Data, b.Data) {
		return false
	}

	return true
}
