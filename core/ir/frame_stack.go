package ir

import "sync"

type frame struct {
	PC       int
	StackLen int
}

type FrameStack[T any] struct {
	frames   []frame
	depth    int
	slotPool []T
	slotSize int
	zero     T
}

var frameStackPools sync.Map // map[int]*sync.Pool keyed by slot size

func poolFor[T any](slotSize int) *sync.Pool {
	pool, _ := frameStackPools.LoadOrStore(slotSize, &sync.Pool{})
	return pool.(*sync.Pool)
}

func NewFrameStack[T any](slotSize int) *FrameStack[T] {
	const initialDepth = 32
	pool := poolFor[T](slotSize)
	if v := pool.Get(); v != nil {
		fs := v.(*FrameStack[T])
		if cap(fs.frames) >= initialDepth && cap(fs.slotPool) >= initialDepth*slotSize {
			fs.frames = fs.frames[:initialDepth]
			fs.slotPool = fs.slotPool[:initialDepth*slotSize]
			fs.slotSize = slotSize
			fs.depth = 0
			var zero T
			fs.zero = zero
			return fs
		}
	}
	var zero T
	return &FrameStack[T]{
		frames:   make([]frame, initialDepth),
		slotPool: make([]T, initialDepth*slotSize),
		slotSize: slotSize,
		zero:     zero,
	}
}

func ReleaseFrameStack[T any](fs *FrameStack[T]) {
	if fs == nil {
		return
	}
	for i := range fs.slotPool {
		fs.slotPool[i] = fs.zero
	}
	fs.depth = 0
	if cap(fs.frames) > 2048 || cap(fs.slotPool) > 16384 {
		return
	}
	poolFor[T](fs.slotSize).Put(fs)
}

func (fs *FrameStack[T]) Depth() int { return fs.depth }

func (fs *FrameStack[T]) Push(pc int, slots []T, stackLen int) {
	if fs.depth >= len(fs.frames) {
		newFrames := make([]frame, fs.depth*2)
		copy(newFrames, fs.frames)
		fs.frames = newFrames
		newPool := make([]T, fs.depth*2*fs.slotSize)
		copy(newPool, fs.slotPool)
		fs.slotPool = newPool
	}
	base := fs.depth * fs.slotSize
	copy(fs.slotPool[base:base+fs.slotSize], slots)
	fs.frames[fs.depth] = frame{PC: pc, StackLen: stackLen}
	fs.depth++
}

func (fs *FrameStack[T]) Pop(slots []T) (int, int) {
	fs.depth--
	f := fs.frames[fs.depth]
	base := fs.depth * fs.slotSize
	copy(slots, fs.slotPool[base:base+fs.slotSize])
	return f.PC, f.StackLen
}
