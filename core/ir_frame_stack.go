package core

import "sync"

// ir_frame_stack.go — explicit frame stack for irCallSelf to avoid
// per-call Go stack frame + slot allocation overhead.
//
// Instead of recursively calling irExec(prog, args), we maintain a
// frame stack within the same irExec invocation. Each irCallSelf
// pushes the current execution state and restarts from pc=0 with
// new slots. Each irReturn pops a frame and resumes the caller.

// irFrame saves the execution state when entering a self-recursive call.
type irFrame struct {
	pc       int
	slots    []Object
	stackLen int // length of Object stack at frame entry
}

// irFrameStack is a pre-allocated stack of frames for self-recursive IR execution.
type irFrameStack struct {
	frames []irFrame
	depth  int
	// Shared slot pool: contiguous memory for all frame slots
	slotPool []Object
	slotSize int // slots per frame
}

var irFrameStackPool sync.Pool

func newIRFrameStack(slotSize int) *irFrameStack {
	const initialDepth = 32
	if v := irFrameStackPool.Get(); v != nil {
		fs := v.(*irFrameStack)
		if cap(fs.frames) >= initialDepth && cap(fs.slotPool) >= initialDepth*slotSize {
			fs.frames = fs.frames[:initialDepth]
			fs.slotPool = fs.slotPool[:initialDepth*slotSize]
			fs.slotSize = slotSize
			fs.depth = 0
			return fs
		}
	}
	return &irFrameStack{
		frames:   make([]irFrame, initialDepth),
		slotPool: make([]Object, initialDepth*slotSize),
		slotSize: slotSize,
	}
}

func releaseIRFrameStack(fs *irFrameStack) {
	if fs == nil {
		return
	}
	for i := range fs.slotPool {
		fs.slotPool[i] = nil
	}
	fs.depth = 0
	if cap(fs.frames) > 2048 || cap(fs.slotPool) > 16384 {
		return
	}
	irFrameStackPool.Put(fs)
}

func (fs *irFrameStack) push(pc int, slots []Object, stackLen int) {
	if fs.depth >= len(fs.frames) {
		// Grow
		newFrames := make([]irFrame, fs.depth*2)
		copy(newFrames, fs.frames)
		fs.frames = newFrames
		newPool := make([]Object, fs.depth*2*fs.slotSize)
		copy(newPool, fs.slotPool)
		fs.slotPool = newPool
	}
	// Save current slots into pool
	base := fs.depth * fs.slotSize
	copy(fs.slotPool[base:base+fs.slotSize], slots)
	fs.frames[fs.depth] = irFrame{pc: pc, stackLen: stackLen}
	fs.depth++
}

func (fs *irFrameStack) pop(slots []Object) (int, int) {
	fs.depth--
	f := fs.frames[fs.depth]
	// Restore slots from pool
	base := fs.depth * fs.slotSize
	copy(slots, fs.slotPool[base:base+fs.slotSize])
	return f.pc, f.stackLen
}
