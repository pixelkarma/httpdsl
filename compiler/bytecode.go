package compiler

// Opcodes
const (
	OP_NOP byte = iota
	OP_CONST        // push constants[u16]
	OP_NULL         // push null
	OP_TRUE         // push true
	OP_FALSE        // push false
	OP_POP          // discard top of stack

	// Local variable access (slot-based, no map lookup)
	OP_GET_LOCAL    // push locals[u8]
	OP_SET_LOCAL    // locals[u8] = pop

	// Global/outer access (name-based, for builtins)
	OP_GET_GLOBAL   // push globals[names[u16]]
	OP_SET_GLOBAL   // globals[names[u16]] = pop

	// Arithmetic
	OP_ADD
	OP_SUB
	OP_MUL
	OP_DIV
	OP_MOD
	OP_NEG

	// Comparison
	OP_EQ
	OP_NEQ
	OP_LT
	OP_GT
	OP_LTE
	OP_GTE
	OP_NOT

	// Logical (short-circuit handled by jumps)
	OP_AND
	OP_OR

	// Control flow
	OP_JMP          // ip = u16
	OP_JMP_FALSE    // if !pop(), ip = u16

	// Functions
	OP_CALL         // call with u8 args
	OP_RETURN       // return u8 values
	OP_RETURN_HTTP  // return HTTP response

	// Data structures
	OP_ARRAY        // create array from u16 elements on stack
	OP_HASH         // create hash from u16 key/value pairs
	OP_INDEX        // pop index, pop obj, push obj[index]
	OP_SET_INDEX    // pop val, pop index, pop obj, obj[index]=val
	OP_DOT          // pop obj, push obj.field (field=names[u16])

	// Optimized operations
	OP_APPEND       // pop val, pop arr, push append(arr, val)
	OP_INC_LOCAL    // locals[u8] += pop (optimized i += n)
	OP_DEC_LOCAL    // locals[u8] -= pop

	// Loop helpers
	OP_ITER_ARRAY   // pop array, push iterator state
	OP_ITER_NEXT    // advance iterator: push (value, index, hasMore)
	OP_ITER_MAP     // pop map, push map iterator
	OP_ITER_MNEXT   // advance map iterator: push (key, value, hasMore)

	// Concatenation fast path
	OP_CONCAT       // pop b, pop a, push str(a)+str(b)
)

// CompiledFunction holds bytecode for a function or route body
type CompiledFunction struct {
	Code      []byte
	Constants []Value
	Names     []string // for global lookups
	NumLocals int
	NumParams int
}

// Bytecode builder helpers
type CodeBuilder struct {
	code      []byte
	constants []Value
	names     []string
	constMap  map[interface{}]int // dedup constants
	nameMap   map[string]int      // dedup names
}

func NewCodeBuilder() *CodeBuilder {
	return &CodeBuilder{
		constMap: make(map[interface{}]int),
		nameMap:  make(map[string]int),
	}
}

func (b *CodeBuilder) Emit(op byte) int {
	pos := len(b.code)
	b.code = append(b.code, op)
	return pos
}

func (b *CodeBuilder) EmitU8(op byte, operand byte) int {
	pos := len(b.code)
	b.code = append(b.code, op, operand)
	return pos
}

func (b *CodeBuilder) EmitU16(op byte, operand int) int {
	pos := len(b.code)
	b.code = append(b.code, op, byte(operand>>8), byte(operand&0xFF))
	return pos
}

func (b *CodeBuilder) AddConstant(val Value) int {
	// Dedup simple constants
	var key interface{}
	switch v := val.(type) {
	case int64:
		key = v
	case float64:
		key = v
	case string:
		key = v
	case bool:
		key = v
	}
	if key != nil {
		if idx, ok := b.constMap[key]; ok {
			return idx
		}
	}
	idx := len(b.constants)
	b.constants = append(b.constants, val)
	if key != nil {
		b.constMap[key] = idx
	}
	return idx
}

func (b *CodeBuilder) AddName(name string) int {
	if idx, ok := b.nameMap[name]; ok {
		return idx
	}
	idx := len(b.names)
	b.names = append(b.names, name)
	b.nameMap[name] = idx
	return idx
}

func (b *CodeBuilder) EmitJump(op byte) int {
	pos := len(b.code)
	b.code = append(b.code, op, 0, 0) // placeholder
	return pos
}

func (b *CodeBuilder) PatchJump(pos int) {
	target := len(b.code)
	b.code[pos+1] = byte(target >> 8)
	b.code[pos+2] = byte(target & 0xFF)
}

func (b *CodeBuilder) PatchJumpTo(pos int, target int) {
	b.code[pos+1] = byte(target >> 8)
	b.code[pos+2] = byte(target & 0xFF)
}

func (b *CodeBuilder) CurrentPos() int {
	return len(b.code)
}
