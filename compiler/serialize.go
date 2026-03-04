package compiler

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
)

// Magic bytes appended to end of binary to find the program data
var programMagic = []byte("HTTPDSL\x00")

func init() {
	// Register all AST types with gob
	gob.Register(&RouteStatement{})
	gob.Register(&FnStatement{})
	gob.Register(&ReturnStatement{})
	gob.Register(&HTTPReturnStatement{})
	gob.Register(&AssignStatement{})
	gob.Register(&IndexAssignStatement{})
	gob.Register(&CompoundAssignStatement{})
	gob.Register(&IfStatement{})
	gob.Register(&WhileStatement{})
	gob.Register(&EachStatement{})
	gob.Register(&ServerStatement{})
	gob.Register(&ExpressionStatement{})
	gob.Register(&BlockStatement{})
	gob.Register(&BreakStatement{})
	gob.Register(&ContinueStatement{})

	gob.Register(&Identifier{})
	gob.Register(&IntegerLiteral{})
	gob.Register(&FloatLiteral{})
	gob.Register(&StringLiteral{})
	gob.Register(&BooleanLiteral{})
	gob.Register(&NullLiteral{})
	gob.Register(&ArrayLiteral{})
	gob.Register(&HashLiteral{})
	gob.Register(&PrefixExpression{})
	gob.Register(&InfixExpression{})
	gob.Register(&CallExpression{})
	gob.Register(&IndexExpression{})
	gob.Register(&DotExpression{})
	gob.Register(&FunctionLiteral{})
}

// SerializeProgram encodes a program to bytes
func SerializeProgram(program *Program) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(program); err != nil {
		return nil, fmt.Errorf("serialize: %w", err)
	}
	return buf.Bytes(), nil
}

// DeserializeProgram decodes a program from bytes
func DeserializeProgram(data []byte) (*Program, error) {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	var program Program
	if err := dec.Decode(&program); err != nil {
		return nil, fmt.Errorf("deserialize: %w", err)
	}
	return &program, nil
}

// PackBinary appends serialized program data + metadata to a runtime binary.
// Format: [runtime binary][program data][8 bytes: data length][8 bytes: magic]
func PackBinary(runtimeBin []byte, programData []byte) []byte {
	result := make([]byte, 0, len(runtimeBin)+len(programData)+16)
	result = append(result, runtimeBin...)
	result = append(result, programData...)
	// Write length of program data as uint64 LE
	lenBuf := make([]byte, 8)
	binary.LittleEndian.PutUint64(lenBuf, uint64(len(programData)))
	result = append(result, lenBuf...)
	result = append(result, programMagic...)
	return result
}

// UnpackProgramData reads program data from the end of a binary.
func UnpackProgramData(bin []byte) ([]byte, error) {
	if len(bin) < 16 {
		return nil, fmt.Errorf("binary too small")
	}
	// Check magic
	magic := bin[len(bin)-8:]
	if !bytes.Equal(magic, programMagic) {
		return nil, fmt.Errorf("no embedded program data found")
	}
	// Read length
	lenBytes := bin[len(bin)-16 : len(bin)-8]
	dataLen := binary.LittleEndian.Uint64(lenBytes)
	if uint64(len(bin)) < dataLen+16 {
		return nil, fmt.Errorf("invalid program data length")
	}
	start := uint64(len(bin)) - 16 - dataLen
	return bin[start : start+dataLen], nil
}
