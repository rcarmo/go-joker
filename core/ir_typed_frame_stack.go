package core

import "sync"

// ir_typed_frame_stack.go — frame stack for irCallSelf in the typed executor.
//
// Pre-allocates contiguous irValue slot memory to avoid per-call allocation.
// Each irCallSelf pushes the current state; irReturn pops it.

type irTypedFrame struct {
	pc       int
	stackLen int
}

type irTypedFrameStack struct {
	frames   []irTypedFrame
	depth    int
	slotPool []irValue
	slotSize int
}

var irTypedFrameStackPool sync.Pool

func newIRTypedFrameStack(slotSize int) *irTypedFrameStack {
	const initialDepth = 32
	if v := irTypedFrameStackPool.Get(); v != nil {
		fs := v.(*irTypedFrameStack)
		if cap(fs.frames) >= initialDepth && cap(fs.slotPool) >= initialDepth*slotSize {
			fs.frames = fs.frames[:initialDepth]
			fs.slotPool = fs.slotPool[:initialDepth*slotSize]
			fs.slotSize = slotSize
			fs.depth = 0
			return fs
		}
	}
	return &irTypedFrameStack{
		frames:   make([]irTypedFrame, initialDepth),
		slotPool: make([]irValue, initialDepth*slotSize),
		slotSize: slotSize,
	}
}

func releaseIRTypedFrameStack(fs *irTypedFrameStack) {
	if fs == nil {
		return
	}
	for i := range fs.slotPool {
		fs.slotPool[i] = irValue{}
	}
	fs.depth = 0
	if cap(fs.frames) > 2048 || cap(fs.slotPool) > 16384 {
		return
	}
	irTypedFrameStackPool.Put(fs)
}

func (fs *irTypedFrameStack) push(pc int, slots []irValue, stackLen int) {
	if fs.depth >= len(fs.frames) {
		newFrames := make([]irTypedFrame, fs.depth*2)
		copy(newFrames, fs.frames)
		fs.frames = newFrames
		newPool := make([]irValue, fs.depth*2*fs.slotSize)
		copy(newPool, fs.slotPool)
		fs.slotPool = newPool
	}
	base := fs.depth * fs.slotSize
	copy(fs.slotPool[base:base+fs.slotSize], slots)
	fs.frames[fs.depth] = irTypedFrame{pc: pc, stackLen: stackLen}
	fs.depth++
}

func (fs *irTypedFrameStack) pop(slots []irValue) (int, int) {
	fs.depth--
	f := fs.frames[fs.depth]
	base := fs.depth * fs.slotSize
	copy(slots, fs.slotPool[base:base+fs.slotSize])
	return f.pc, f.stackLen
}
