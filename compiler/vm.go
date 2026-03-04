package compiler

const (
	vmStackSize = 512
	vmFrameSize = 64
)

type vmFrame struct {
	fn      *CompiledFunction
	ip      int
	basePtr int
}

// arrayIter is a lightweight iterator for each loops
type arrayIter struct {
	arr []Value
	idx int
}

type mapIter struct {
	keys []string
	vals map[string]Value
	idx  int
}

// VM executes bytecode
type VM struct {
	stack   [vmStackSize]Value
	sp      int
	frames  [vmFrameSize]vmFrame
	fp      int
	globals *Environment
}

func NewVM(globals *Environment) *VM {
	return &VM{globals: globals}
}

func (vm *VM) push(v Value) {
	vm.stack[vm.sp] = v
	vm.sp++
}

func (vm *VM) pop() Value {
	vm.sp--
	return vm.stack[vm.sp]
}

// Execute runs a compiled function with arguments, returns the result
func (vm *VM) Execute(fn *CompiledFunction, args []Value) Value {
	// Set up initial frame
	vm.sp = 0
	vm.fp = 0

	// Reserve space for locals
	for i := 0; i < fn.NumLocals; i++ {
		vm.stack[i] = NULL
	}
	// Copy args to local slots
	for i, a := range args {
		vm.stack[i] = a
	}
	vm.sp = fn.NumLocals

	vm.frames[0] = vmFrame{fn: fn, ip: 0, basePtr: 0}

	return vm.run()
}

func (vm *VM) run() Value {
	for {
		frame := &vm.frames[vm.fp]
		fn := frame.fn
		code := fn.Code
		ip := frame.ip

		if ip >= len(code) {
			return NULL
		}

		op := code[ip]
		ip++

		switch op {
		case OP_CONST:
			idx := int(code[ip])<<8 | int(code[ip+1])
			ip += 2
			vm.stack[vm.sp] = fn.Constants[idx]
			vm.sp++

		case OP_NULL:
			vm.stack[vm.sp] = NULL
			vm.sp++

		case OP_TRUE:
			vm.stack[vm.sp] = true
			vm.sp++

		case OP_FALSE:
			vm.stack[vm.sp] = false
			vm.sp++

		case OP_POP:
			vm.sp--

		case OP_GET_LOCAL:
			slot := int(code[ip])
			ip++
			vm.stack[vm.sp] = vm.stack[frame.basePtr+slot]
			vm.sp++

		case OP_SET_LOCAL:
			slot := int(code[ip])
			ip++
			vm.sp--
			vm.stack[frame.basePtr+slot] = vm.stack[vm.sp]

		case OP_GET_GLOBAL:
			nameIdx := int(code[ip])<<8 | int(code[ip+1])
			ip += 2
			name := fn.Names[nameIdx]
			val, _ := vm.globals.Get(name)
			vm.stack[vm.sp] = val
			vm.sp++

		case OP_SET_GLOBAL:
			nameIdx := int(code[ip])<<8 | int(code[ip+1])
			ip += 2
			vm.sp--
			vm.globals.Set(fn.Names[nameIdx], vm.stack[vm.sp])

		// --- Arithmetic with int64 fast paths ---
		case OP_ADD:
			vm.sp--
			b := vm.stack[vm.sp]
			a := vm.stack[vm.sp-1]
			if ai, ok := a.(int64); ok {
				if bi, ok := b.(int64); ok {
					vm.stack[vm.sp-1] = ai + bi
					goto done
				}
			}
			vm.stack[vm.sp-1] = addValues(a, b)

		case OP_SUB:
			vm.sp--
			b := vm.stack[vm.sp]
			a := vm.stack[vm.sp-1]
			if ai, ok := a.(int64); ok {
				if bi, ok := b.(int64); ok {
					vm.stack[vm.sp-1] = ai - bi
					goto done
				}
			}
			vm.stack[vm.sp-1] = subtractValues(a, b)

		case OP_MUL:
			vm.sp--
			b := vm.stack[vm.sp]
			a := vm.stack[vm.sp-1]
			if ai, ok := a.(int64); ok {
				if bi, ok := b.(int64); ok {
					vm.stack[vm.sp-1] = ai * bi
					goto done
				}
			}
			vm.stack[vm.sp-1] = multiplyValues(a, b)

		case OP_DIV:
			vm.sp--
			b := vm.stack[vm.sp]
			a := vm.stack[vm.sp-1]
			vm.stack[vm.sp-1] = divideValues(a, b)

		case OP_MOD:
			vm.sp--
			b := vm.stack[vm.sp]
			a := vm.stack[vm.sp-1]
			vm.stack[vm.sp-1] = modValues(a, b)

		case OP_NEG:
			a := vm.stack[vm.sp-1]
			if ai, ok := a.(int64); ok {
				vm.stack[vm.sp-1] = -ai
			} else if af, ok := a.(float64); ok {
				vm.stack[vm.sp-1] = -af
			}

		// --- Comparison with int64 fast paths ---
		case OP_EQ:
			vm.sp--
			b := vm.stack[vm.sp]
			a := vm.stack[vm.sp-1]
			vm.stack[vm.sp-1] = valuesEqual(a, b)

		case OP_NEQ:
			vm.sp--
			b := vm.stack[vm.sp]
			a := vm.stack[vm.sp-1]
			eq := valuesEqual(a, b)
			if v, ok := eq.(bool); ok {
				vm.stack[vm.sp-1] = !v
			} else {
				vm.stack[vm.sp-1] = true
			}

		case OP_LT:
			vm.sp--
			b := vm.stack[vm.sp]
			a := vm.stack[vm.sp-1]
			if ai, ok := a.(int64); ok {
				if bi, ok := b.(int64); ok {
					vm.stack[vm.sp-1] = ai < bi
					goto done
				}
			}
			vm.stack[vm.sp-1] = compareLess(a, b)

		case OP_GT:
			vm.sp--
			b := vm.stack[vm.sp]
			a := vm.stack[vm.sp-1]
			if ai, ok := a.(int64); ok {
				if bi, ok := b.(int64); ok {
					vm.stack[vm.sp-1] = ai > bi
					goto done
				}
			}
			vm.stack[vm.sp-1] = compareLess(b, a)

		case OP_LTE:
			vm.sp--
			b := vm.stack[vm.sp]
			a := vm.stack[vm.sp-1]
			if ai, ok := a.(int64); ok {
				if bi, ok := b.(int64); ok {
					vm.stack[vm.sp-1] = ai <= bi
					goto done
				}
			}
			l := compareLess(a, b)
			eq := valuesEqual(a, b)
			lb, _ := l.(bool)
			eb, _ := eq.(bool)
			vm.stack[vm.sp-1] = lb || eb

		case OP_GTE:
			vm.sp--
			b := vm.stack[vm.sp]
			a := vm.stack[vm.sp-1]
			if ai, ok := a.(int64); ok {
				if bi, ok := b.(int64); ok {
					vm.stack[vm.sp-1] = ai >= bi
					goto done
				}
			}
			l := compareLess(b, a)
			eq := valuesEqual(a, b)
			lb, _ := l.(bool)
			eb, _ := eq.(bool)
			vm.stack[vm.sp-1] = lb || eb

		case OP_NOT:
			vm.stack[vm.sp-1] = !isTruthy(vm.stack[vm.sp-1])

		case OP_AND:
			vm.sp--
			b := vm.stack[vm.sp]
			a := vm.stack[vm.sp-1]
			vm.stack[vm.sp-1] = isTruthy(a) && isTruthy(b)

		case OP_OR:
			vm.sp--
			b := vm.stack[vm.sp]
			a := vm.stack[vm.sp-1]
			vm.stack[vm.sp-1] = isTruthy(a) || isTruthy(b)

		// --- Control flow ---
		case OP_JMP:
			target := int(code[ip])<<8 | int(code[ip+1])
			ip = target

		case OP_JMP_FALSE:
			target := int(code[ip])<<8 | int(code[ip+1])
			ip += 2
			vm.sp--
			if !isTruthy(vm.stack[vm.sp]) {
				ip = target
			}

		// --- Function calls ---
		case OP_CALL:
			nArgs := int(code[ip])
			ip++
			frame.ip = ip // save current position

			fnVal := vm.stack[vm.sp-1-nArgs]

			switch f := fnVal.(type) {
			case *BuiltinFunction:
				args := make([]Value, nArgs)
				copy(args, vm.stack[vm.sp-nArgs:vm.sp])
				vm.sp -= nArgs + 1 // pop args + function
				result := f.Fn(args...)
				vm.push(result)

			case *CompiledFunction:
				// Push a new frame
				vm.fp++
				newBase := vm.sp - nArgs
				// Move args into position and reserve locals
				// Args are already at newBase..newBase+nArgs
				// We need to clear remaining locals
				for i := nArgs; i < f.NumLocals; i++ {
					vm.stack[newBase+i] = NULL
				}
				// Overwrite the function value slot (it's below the args)
				// Actually, function value is at newBase-1, args at newBase..newBase+nArgs
				vm.sp = newBase + f.NumLocals
				vm.frames[vm.fp] = vmFrame{fn: f, ip: 0, basePtr: newBase}
				continue // restart loop with new frame

			case *FunctionValue:
				// AST-based function — compile it on the fly
				compiled := CompileFunction(f.Params, f.Body)
				vm.fp++
				newBase := vm.sp - nArgs
				for i := nArgs; i < compiled.NumLocals; i++ {
					vm.stack[newBase+i] = NULL
				}
				vm.sp = newBase + compiled.NumLocals
				vm.frames[vm.fp] = vmFrame{fn: compiled, ip: 0, basePtr: newBase}
				continue

			default:
				// Unknown callable
				vm.sp -= nArgs + 1
				vm.push(NULL)
			}

		case OP_RETURN:
			nVals := int(code[ip])
			// Collect return values
			if vm.fp == 0 {
				// Top-level return
				if nVals == 0 {
					return NULL
				}
				if nVals == 1 {
					return vm.stack[vm.sp-1]
				}
				vals := make([]Value, nVals)
				copy(vals, vm.stack[vm.sp-nVals:vm.sp])
				return &ReturnValue{Values: vals}
			}
			// Return from function call
			var retVal Value
			if nVals == 0 {
				retVal = NULL
			} else if nVals == 1 {
				retVal = vm.stack[vm.sp-1]
			} else {
				vals := make([]Value, nVals)
				copy(vals, vm.stack[vm.sp-nVals:vm.sp])
				retVal = &ReturnValue{Values: vals}
			}
			// Pop frame
			base := frame.basePtr
			vm.fp--
			vm.sp = base - 1 // pop the function value too
			vm.push(retVal)
			frame = &vm.frames[vm.fp]
			fn = frame.fn
			code = fn.Code
			ip = frame.ip
			continue

		case OP_RETURN_HTTP:
			// Stack: body, statusCode, responseType
			respType := valueToString(vm.pop())
			statusCode := int(vm.pop().(int64))
			body := vm.pop()
			return &HTTPReturnValue{
				ResponseType: respType,
				StatusCode:   statusCode,
				Body:         body,
			}

		// --- Data structures ---
		case OP_ARRAY:
			size := int(code[ip])<<8 | int(code[ip+1])
			ip += 2
			arr := make([]Value, size)
			copy(arr, vm.stack[vm.sp-size:vm.sp])
			vm.sp -= size
			vm.push(arr)

		case OP_HASH:
			size := int(code[ip])<<8 | int(code[ip+1])
			ip += 2
			hash := make(map[string]Value, size)
			base := vm.sp - size*2
			for i := 0; i < size; i++ {
				key := valueToString(vm.stack[base+i*2])
				val := vm.stack[base+i*2+1]
				hash[key] = val
			}
			vm.sp = base
			vm.push(hash)

		case OP_INDEX:
			vm.sp--
			idx := vm.stack[vm.sp]
			obj := vm.stack[vm.sp-1]
			switch o := obj.(type) {
			case []Value:
				if i, ok := idx.(int64); ok && i >= 0 && int(i) < len(o) {
					vm.stack[vm.sp-1] = o[i]
				} else {
					vm.stack[vm.sp-1] = NULL
				}
			case map[string]Value:
				key := valueToString(idx)
				if v, ok := o[key]; ok {
					vm.stack[vm.sp-1] = v
				} else {
					vm.stack[vm.sp-1] = NULL
				}
			case string:
				if i, ok := idx.(int64); ok && i >= 0 && int(i) < len(o) {
					vm.stack[vm.sp-1] = string(o[i])
				} else {
					vm.stack[vm.sp-1] = NULL
				}
			default:
				vm.stack[vm.sp-1] = NULL
			}

		case OP_SET_INDEX:
			vm.sp -= 3
			obj := vm.stack[vm.sp]
			idx := vm.stack[vm.sp+1]
			val := vm.stack[vm.sp+2]
			switch o := obj.(type) {
			case map[string]Value:
				o[valueToString(idx)] = val
			case []Value:
				if i, ok := idx.(int64); ok && i >= 0 && int(i) < len(o) {
					o[i] = val
				}
			}

		case OP_DOT:
			nameIdx := int(code[ip])<<8 | int(code[ip+1])
			ip += 2
			field := fn.Names[nameIdx]
			obj := vm.stack[vm.sp-1]
			if m, ok := obj.(map[string]Value); ok {
				if v, ok := m[field]; ok {
					vm.stack[vm.sp-1] = v
				} else {
					vm.stack[vm.sp-1] = NULL
				}
			} else {
				vm.stack[vm.sp-1] = NULL
			}

		case OP_APPEND:
			vm.sp--
			val := vm.stack[vm.sp]
			if arr, ok := vm.stack[vm.sp-1].([]Value); ok {
				vm.stack[vm.sp-1] = append(arr, val)
			}

		// --- Optimized local operations ---
		case OP_INC_LOCAL:
			slot := int(code[ip])
			ip++
			vm.sp--
			inc := vm.stack[vm.sp]
			cur := vm.stack[frame.basePtr+slot]
			if ci, ok := cur.(int64); ok {
				if ii, ok := inc.(int64); ok {
					vm.stack[frame.basePtr+slot] = ci + ii
					goto done
				}
			}
			vm.stack[frame.basePtr+slot] = addValues(cur, inc)

		case OP_DEC_LOCAL:
			slot := int(code[ip])
			ip++
			vm.sp--
			dec := vm.stack[vm.sp]
			cur := vm.stack[frame.basePtr+slot]
			if ci, ok := cur.(int64); ok {
				if di, ok := dec.(int64); ok {
					vm.stack[frame.basePtr+slot] = ci - di
					goto done
				}
			}
			vm.stack[frame.basePtr+slot] = subtractValues(cur, dec)

		// --- Iterators ---
		case OP_ITER_ARRAY:
			val := vm.stack[vm.sp-1]
			switch v := val.(type) {
			case []Value:
				vm.stack[vm.sp-1] = &arrayIter{arr: v, idx: 0}
			case map[string]Value:
				keys := make([]string, 0, len(v))
				for k := range v {
					keys = append(keys, k)
				}
				vm.stack[vm.sp-1] = &mapIter{keys: keys, vals: v, idx: 0}
			}

		case OP_ITER_NEXT:
			iter := vm.stack[vm.sp-1]
			switch it := iter.(type) {
			case *arrayIter:
				if it.idx < len(it.arr) {
					// Push: index, value, true (hasMore)
					vm.stack[vm.sp] = int64(it.idx)
					vm.stack[vm.sp+1] = it.arr[it.idx]
					vm.stack[vm.sp+2] = true
					vm.sp += 3
					it.idx++
				} else {
					vm.stack[vm.sp] = false
					vm.sp++
				}
			case *mapIter:
				if it.idx < len(it.keys) {
					key := it.keys[it.idx]
					vm.stack[vm.sp] = it.vals[key]
					vm.stack[vm.sp+1] = key
					vm.stack[vm.sp+2] = true
					vm.sp += 3
					it.idx++
				} else {
					vm.stack[vm.sp] = false
					vm.sp++
				}
			default:
				vm.stack[vm.sp] = false
				vm.sp++
			}

		default:
			// Unknown opcode, skip
		}

	done:
		frame.ip = ip
	}
}
