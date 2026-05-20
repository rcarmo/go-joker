package collections

import (
	"io"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

type (
	Box struct {
		Val interface{}
	}
	Node interface {
		assoc(shift uint, hash uint32, key coretypes.Object, val coretypes.Object, addedLeaf *Box) Node
		without(shift uint, hash uint32, key coretypes.Object) Node
		find(shift uint, hash uint32, key coretypes.Object) *coretypes.Pair
		nodeSeq() coretypes.Seq
		iter() coretypes.MapIterator
	}
	HashMap struct {
		coretypes.InfoHolder
		coretypes.MetaHolder
		CountValue int
		Root       Node
	}
	BitmapIndexedNode struct {
		Bitmap int
		Array  []interface{}
	}
	HashCollisionNode struct {
		HashValue  uint32
		CountValue int
		Array      []interface{}
	}
	ArrayNode struct {
		CountValue int
		Array      []Node
	}
	NodeSeq struct {
		coretypes.InfoHolder
		coretypes.MetaHolder
		Array    []interface{}
		I        int
		SeqValue coretypes.Seq
	}
	ArrayNodeSeq struct {
		coretypes.InfoHolder
		coretypes.MetaHolder
		Nodes    []Node
		I        int
		SeqValue coretypes.Seq
	}
	NodeIterator struct {
		Array     []interface{}
		I         int
		NextEntry *coretypes.Pair
		NextIter  coretypes.MapIterator
	}
	ArrayNodeIterator struct {
		Array      []Node
		I          int
		NestedIter coretypes.MapIterator
	}
)

var (
	emptyIndexedNode = &BitmapIndexedNode{}
	EmptyHashMap     = &HashMap{}
)

func (iter *ArrayNodeIterator) HasNext() bool {
	nextI, nextNested, has := ArrayNodeIterHasNext(iter.Array, iter.I, iter.NestedIter, func(n Node) bool { return n == nil }, func(n Node) Iterator[*coretypes.Pair] { return n.iter() })
	iter.I = nextI
	iter.NestedIter = nextNested
	return has
}

func (iter *ArrayNodeIterator) Next() *coretypes.Pair {
	if iter.HasNext() {
		return iter.NestedIter.Next()
	}
	panic(coretypes.NewIteratorError())
}

func (iter *NodeIterator) advance() bool {
	nextI, entry, hasEntry, nested, hasNested := NodeArrayAdvance(iter.Array, iter.I, func(v any) Iterator[*coretypes.Pair] {
		return v.(Node).iter()
	}, func(key any, value any) *coretypes.Pair {
		return &coretypes.Pair{Key: key.(coretypes.Object), Value: value.(coretypes.Object)}
	})
	iter.I = nextI
	if hasEntry {
		iter.NextEntry = entry
		return true
	}
	if hasNested {
		iter.NextIter = nested
		return true
	}
	return false
}

func (iter *NodeIterator) HasNext() bool {
	if iter.NextEntry != nil || iter.NextIter != nil {
		return true
	}
	return iter.advance()
}

func (iter *NodeIterator) Next() *coretypes.Pair {
	ret, nextIter, ok := NodeIteratorNext(iter.NextEntry, iter.NextIter, func() (*coretypes.Pair, Iterator[*coretypes.Pair], bool) {
		if iter.advance() {
			if iter.NextEntry != nil {
				ret := iter.NextEntry
				iter.NextEntry = nil
				return ret, iter.NextIter, true
			}
			if iter.NextIter != nil {
				ret := iter.NextIter.Next()
				if !iter.NextIter.HasNext() {
					iter.NextIter = nil
				}
				return ret, iter.NextIter, true
			}
		}
		return nil, iter.NextIter, false
	})
	if !ok {
		panic(coretypes.NewIteratorError())
	}
	iter.NextEntry = nil
	iter.NextIter = nextIter
	return ret
}

func newArrayNodeSeq(nodes []Node, i int, s coretypes.Seq) coretypes.Seq {
	nextI, nextS, ok := NextArrayNodeSeq(nodes, i, s,
		func(seq coretypes.Seq) bool { return seq != nil },
		func(node Node) coretypes.Seq {
			if node == nil {
				return nil
			}
			return node.nodeSeq()
		})
	if !ok {
		return nil
	}
	return &ArrayNodeSeq{Nodes: nodes, I: nextI, SeqValue: nextS}
}

func (s *ArrayNodeSeq) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	s.Info = info
	return s
}

func (s *ArrayNodeSeq) WithMeta(meta coretypes.Map) coretypes.Object {
	res := *s
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return &res
}

func (s *ArrayNodeSeq) Seq() coretypes.Seq {
	return s
}

func (s *ArrayNodeSeq) Equals(other interface{}) bool {
	return coretypes.IsSeqEqual(s, other)
}

func (s *ArrayNodeSeq) ToString(escape bool) string {
	return SeqToString(s, func(obj coretypes.Object) string { return obj.ToString(escape) })
}

func (seq *ArrayNodeSeq) Pprint(w io.Writer, indent int) int {
	return SeqPprint(seq, w, indent)
}

func (seq *ArrayNodeSeq) Format(w io.Writer, indent int) int {
	return SeqPprint(seq, w, indent)
}

func (s *ArrayNodeSeq) GetType() *coretypes.Type {
	return coretypes.RuntimeTypes.ArrayNodeSeq
}

func (s *ArrayNodeSeq) Hash() uint32 {
	return HashOrdered(s)
}

func (s *ArrayNodeSeq) First() coretypes.Object {
	return s.SeqValue.First()
}

func (s *ArrayNodeSeq) Rest() coretypes.Seq {
	next := s.SeqValue.Rest()
	if next.IsEmpty() {
		next = nil
	}
	res := newArrayNodeSeq(s.Nodes, s.I, next)
	if res == nil {
		return EmptyList
	}
	return res
}

func (s *ArrayNodeSeq) IsEmpty() bool {
	if s.SeqValue != nil {
		return s.SeqValue.IsEmpty()
	}
	return false
}

func (s *ArrayNodeSeq) Cons(obj coretypes.Object) coretypes.Seq {
	return &ConsSeq{FirstValue: obj, RestValue: s}
}

func (s *ArrayNodeSeq) SequentialMarker() {}

func newNodeSeq(array []interface{}, i int, s coretypes.Seq) coretypes.Seq {
	nextI, nextS, ok := NextNodeSeq(array, i, s,
		func(seq coretypes.Seq) bool { return seq != nil },
		func(v any) (coretypes.Seq, bool) {
			node, ok := v.(Node)
			if !ok {
				return nil, false
			}
			return node.nodeSeq(), true
		},
		func(key any) bool { return key != nil })
	if !ok {
		return nil
	}
	return &NodeSeq{Array: array, I: nextI, SeqValue: nextS}
}

func (s *NodeSeq) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	s.Info = info
	return s
}

func (s *NodeSeq) WithMeta(meta coretypes.Map) coretypes.Object {
	res := *s
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return &res
}

func (s *NodeSeq) Seq() coretypes.Seq {
	return s
}

func (s *NodeSeq) Equals(other interface{}) bool {
	return coretypes.IsSeqEqual(s, other)
}

func (s *NodeSeq) ToString(escape bool) string {
	return SeqToString(s, func(obj coretypes.Object) string { return obj.ToString(escape) })
}

func (seq *NodeSeq) Pprint(w io.Writer, indent int) int {
	return SeqPprint(seq, w, indent)
}

func (seq *NodeSeq) Format(w io.Writer, indent int) int {
	return SeqPprint(seq, w, indent)
}

func (s *NodeSeq) GetType() *coretypes.Type {
	return coretypes.RuntimeTypes.NodeSeq
}

func (s *NodeSeq) Hash() uint32 {
	return HashOrdered(s)
}

func (s *NodeSeq) First() coretypes.Object {
	if s.SeqValue != nil {
		return s.SeqValue.First()
	}
	return NewVectorFrom(s.Array[s.I].(coretypes.Object), s.Array[s.I+1].(coretypes.Object))
}

func (s *NodeSeq) Rest() coretypes.Seq {
	var res coretypes.Seq
	if s.SeqValue != nil {
		next := s.SeqValue.Rest()
		if next.IsEmpty() {
			next = nil
		}
		res = newNodeSeq(s.Array, s.I, next)
	} else {
		res = newNodeSeq(s.Array, s.I+2, nil)
	}
	if res == nil {
		return EmptyList
	}
	return res
}

func (s *NodeSeq) IsEmpty() bool {
	if s.SeqValue != nil {
		return s.SeqValue.IsEmpty()
	}
	return false
}

func (s *NodeSeq) Cons(obj coretypes.Object) coretypes.Seq {
	return &ConsSeq{FirstValue: obj, RestValue: s}
}

func (s *NodeSeq) SequentialMarker() {}

func (n *ArrayNode) iter() coretypes.MapIterator {
	return &ArrayNodeIterator{
		Array: n.Array,
	}
}

func (n *ArrayNode) assoc(shift uint, hash uint32, key coretypes.Object, val coretypes.Object, addedLeaf *Box) Node {
	idx := HashMask(hash, shift)
	node := n.Array[idx]
	if node == nil {
		return &ArrayNode{
			CountValue: n.CountValue + 1,
			Array:      AssocCopy[Node](n.Array, int(idx), emptyIndexedNode.assoc(shift+5, hash, key, val, addedLeaf)),
		}
	}
	nn := node.assoc(shift+5, hash, key, val, addedLeaf)
	if nn == node {
		return n
	}
	return &ArrayNode{
		CountValue: n.CountValue,
		Array:      AssocCopy[Node](n.Array, int(idx), nn),
	}
}

func (n *ArrayNode) without(shift uint, hash uint32, key coretypes.Object) Node {
	idx := HashMask(hash, shift)
	node := n.Array[idx]
	if node == nil {
		return n
	}
	nn := node.without(shift+5, hash, key)
	if nn == node {
		return n
	}
	if nn == nil {
		if n.CountValue <= 8 {
			return n.pack(uint(idx))
		}
		return &ArrayNode{
			CountValue: n.CountValue - 1,
			Array:      AssocCopy[Node](n.Array, int(idx), nn),
		}
	} else {
		return &ArrayNode{
			CountValue: n.CountValue,
			Array:      AssocCopy[Node](n.Array, int(idx), nn),
		}
	}
}

func (n *ArrayNode) find(shift uint, hash uint32, key coretypes.Object) *coretypes.Pair {
	idx := HashMask(hash, shift)
	node := n.Array[idx]
	if node == nil {
		return nil
	}
	return node.find(shift+5, hash, key)
}

func (n *ArrayNode) nodeSeq() coretypes.Seq {
	return newArrayNodeSeq(n.Array, 0, nil)
}

func (n *ArrayNode) pack(idx uint) Node {
	bitmap, newArray := PackIndexedNodes(n.Array, idx, func(node Node) bool { return node != nil })
	return &BitmapIndexedNode{
		Bitmap: bitmap,
		Array:  newArray,
	}
}

func (n *HashCollisionNode) findIndex(key coretypes.Object) int {
	for i := 0; i < 2*n.CountValue; i += 2 {
		if key.Equals(n.Array[i]) {
			return i
		}
	}
	return -1
}

func (n *HashCollisionNode) iter() coretypes.MapIterator {
	return &NodeIterator{
		Array: n.Array,
	}
}

func (n *HashCollisionNode) assoc(shift uint, hash uint32, key coretypes.Object, val coretypes.Object, addedLeaf *Box) Node {
	if hash == n.HashValue {
		idx := n.findIndex(key)
		if idx != -1 {
			if n.Array[idx+1] == val {
				return n
			}
			return &HashCollisionNode{
				HashValue:  hash,
				CountValue: n.CountValue,
				Array:      AssocCopy[interface{}](n.Array, idx+1, val),
			}
		}
		newArray := AppendPair[interface{}](n.Array, key, val)
		addedLeaf.Val = addedLeaf
		return &HashCollisionNode{
			HashValue:  hash,
			CountValue: n.CountValue + 1,
			Array:      newArray,
		}
	}
	return (&BitmapIndexedNode{
		Bitmap: Bitpos(n.HashValue, shift),
		Array:  []interface{}{nil, n},
	}).assoc(shift, hash, key, val, addedLeaf)
}

func (n *HashCollisionNode) without(shift uint, hash uint32, key coretypes.Object) Node {
	idx := n.findIndex(key)
	if idx == -1 {
		return n
	}
	if n.CountValue == 1 {
		return nil
	}
	return &HashCollisionNode{
		HashValue:  hash,
		CountValue: n.CountValue - 1,
		Array:      RemovePair(n.Array, idx/2),
	}
}

func (n *HashCollisionNode) find(shift uint, hash uint32, key coretypes.Object) *coretypes.Pair {
	idx := n.findIndex(key)
	if idx == -1 {
		return nil
	}
	return &coretypes.Pair{
		Key:   n.Array[idx].(coretypes.Object),
		Value: n.Array[idx+1].(coretypes.Object),
	}
}

func (n *HashCollisionNode) nodeSeq() coretypes.Seq {
	return newNodeSeq(n.Array, 0, nil)
}

func createNode(shift uint, key1 coretypes.Object, val1 coretypes.Object, key2hash uint32, key2 coretypes.Object, val2 coretypes.Object) Node {
	key1hash := key1.Hash()
	if key1hash == key2hash {
		return &HashCollisionNode{
			HashValue:  key1hash,
			CountValue: 2,
			Array:      []interface{}{key1, val1, key2, val2},
		}
	}
	addedLeaf := &Box{}
	return emptyIndexedNode.assoc(shift, key1hash, key1, val1, addedLeaf).assoc(shift, key2hash, key2, val2, addedLeaf)
}

func (b *BitmapIndexedNode) index(bit int) int {
	return BitCount(b.Bitmap & (bit - 1))
}

func (b *BitmapIndexedNode) iter() coretypes.MapIterator {
	return &NodeIterator{
		Array: b.Array,
	}
}

func (b *BitmapIndexedNode) assoc(shift uint, hash uint32, key coretypes.Object, val coretypes.Object, addedLeaf *Box) Node {
	bit := Bitpos(hash, shift)
	idx := b.index(bit)
	if b.Bitmap&bit != 0 {
		keyOrNull := b.Array[2*idx]
		valOrNode := b.Array[2*idx+1]
		if keyOrNull == nil {
			n := valOrNode.(Node).assoc(shift+5, hash, key, val, addedLeaf)
			if n == valOrNode {
				return b
			}
			return &BitmapIndexedNode{
				Bitmap: b.Bitmap,
				Array:  AssocCopy[interface{}](b.Array, 2*idx+1, n),
			}
		}
		if key.Equals(keyOrNull) {
			if val == valOrNode {
				return b
			}
			return &BitmapIndexedNode{
				Bitmap: b.Bitmap,
				Array:  AssocCopy[interface{}](b.Array, 2*idx+1, val),
			}
		}
		addedLeaf.Val = addedLeaf
		return &BitmapIndexedNode{
			Bitmap: b.Bitmap,
			Array:  Assoc2Copy[interface{}](b.Array, 2*idx, nil, 2*idx+1, createNode(shift+5, keyOrNull.(coretypes.Object), valOrNode.(coretypes.Object), hash, key, val)),
		}
	} else {
		n := BitCount(b.Bitmap)
		if n >= 16 {
			nodes := make([]Node, 32)
			jdx := HashMask(hash, shift)
			nodes[jdx] = emptyIndexedNode.assoc(shift+5, hash, key, val, addedLeaf)
			j := 0
			var i uint
			for i = 0; i < 32; i++ {
				if (b.Bitmap>>i)&1 != 0 {
					if b.Array[j] == nil {
						nodes[i] = b.Array[j+1].(Node)
					} else {
						nodes[i] = emptyIndexedNode.assoc(shift+5, b.Array[j].(coretypes.Object).Hash(), b.Array[j].(coretypes.Object), b.Array[j+1].(coretypes.Object), addedLeaf)
					}
					j += 2
				}
			}
			return &ArrayNode{
				CountValue: n + 1,
				Array:      nodes,
			}
		} else {
			newArray := InsertPair[interface{}](b.Array, idx, key, val)
			addedLeaf.Val = addedLeaf
			return &BitmapIndexedNode{
				Bitmap: b.Bitmap | bit,
				Array:  newArray,
			}
		}
	}
}

func (b *BitmapIndexedNode) without(shift uint, hash uint32, key coretypes.Object) Node {
	bit := Bitpos(hash, shift)
	if (b.Bitmap & bit) == 0 {
		return b
	}
	idx := b.index(bit)
	keyOrNull := b.Array[2*idx]
	valOrNode := b.Array[2*idx+1]
	if keyOrNull == nil {
		n := valOrNode.(Node).without(shift+5, hash, key)
		if n == valOrNode {
			return b
		}
		if n != nil {
			return &BitmapIndexedNode{
				Bitmap: b.Bitmap,
				Array:  AssocCopy[interface{}](b.Array, 2*idx+1, n),
			}
		}
		if b.Bitmap == bit {
			return nil
		}
		return &BitmapIndexedNode{
			Bitmap: b.Bitmap ^ bit,
			Array:  RemovePair(b.Array, idx),
		}
	}
	if key.Equals(keyOrNull) {
		return &BitmapIndexedNode{
			Bitmap: b.Bitmap ^ bit,
			Array:  RemovePair(b.Array, idx),
		}
	}
	return b
}

func (b *BitmapIndexedNode) find(shift uint, hash uint32, key coretypes.Object) *coretypes.Pair {
	bit := Bitpos(hash, shift)
	if (b.Bitmap & bit) == 0 {
		return nil
	}
	idx := b.index(bit)
	keyOrNull := b.Array[2*idx]
	valOrNode := b.Array[2*idx+1]
	if keyOrNull == nil {
		return valOrNode.(Node).find(shift+5, hash, key)
	}
	if key.Equals(keyOrNull) {
		return &coretypes.Pair{
			Key:   keyOrNull.(coretypes.Object),
			Value: valOrNode.(coretypes.Object),
		}
	}
	return nil
}

func (b *BitmapIndexedNode) nodeSeq() coretypes.Seq {
	return newNodeSeq(b.Array, 0, nil)
}

func (m *HashMap) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	m.Info = info
	return m
}

func (m *HashMap) WithMeta(meta coretypes.Map) coretypes.Object {
	res := *m
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return &res
}

func (m *HashMap) ToString(escape bool) string {
	return MapToString(m, escape)
}

func (m *HashMap) Equals(other interface{}) bool {
	return MapEquals(m, other)
}

func (m *HashMap) GetType() *coretypes.Type {
	return coretypes.RuntimeTypes.HashMap
}

func (m *HashMap) Hash() uint32 {
	return HashUnordered(m.Seq(), 1)
}

func (m *HashMap) Seq() coretypes.Seq {
	if m.Root != nil {
		s := m.Root.nodeSeq()
		if s != nil {
			return s
		}
	}
	return EmptyList
}

func (m *HashMap) Count() int {
	return m.CountValue
}

func (m *HashMap) ContainsKey(key coretypes.Object) bool {
	if m.Root != nil {
		return m.Root.find(0, key.Hash(), key) != nil
	} else {
		return false
	}
}

func (m *HashMap) Assoc(key, val coretypes.Object) coretypes.Associative {
	addedLeaf := &Box{}
	var newroot, t Node
	if m.Root == nil {
		t = emptyIndexedNode
	} else {
		t = m.Root
	}
	newroot = t.assoc(0, key.Hash(), key, val, addedLeaf)
	if newroot == m.Root {
		return m
	}
	newcount := m.CountValue
	if addedLeaf.Val != nil {
		newcount = m.CountValue + 1
	}
	res := &HashMap{
		CountValue: newcount,
		Root:       newroot,
	}
	res.Meta = m.Meta
	return res
}

func (m *HashMap) EntryAt(key coretypes.Object) coretypes.Object {
	if m.Root != nil {
		p := m.Root.find(0, key.Hash(), key)
		if p != nil {
			return NewArrayVectorFrom(p.Key, p.Value)
		}
	}
	return nil
}

func (m *HashMap) Get(key coretypes.Object) (bool, coretypes.Object) {
	if m.Root != nil {
		if res := m.Root.find(0, key.Hash(), key); res != nil {
			return true, res.Value
		}
	}
	return false, nil
}

func (m *HashMap) Conj(obj coretypes.Object) coretypes.Conjable {
	return MapConj(m, obj, func(msg string) any { return coretypes.RuntimeError(msg) })
}

func (m *HashMap) Iter() coretypes.MapIterator {
	if m.Root == nil {
		return coretypes.EmptyMapIteratorInstance
	}
	return m.Root.iter()
}

func (m *HashMap) Keys() coretypes.Seq {
	return &MappingSeq{
		SeqValue: m.Seq(),
		Fn: func(obj coretypes.Object) coretypes.Object {
			return obj.(coretypes.Vec).Nth(0)
		},
	}
}

func (m *HashMap) Vals() coretypes.Seq {
	return &MappingSeq{
		SeqValue: m.Seq(),
		Fn: func(obj coretypes.Object) coretypes.Object {
			return obj.(coretypes.Vec).Nth(1)
		},
	}
}

func (m *HashMap) Merge(other coretypes.Map) coretypes.Map {
	if other.Count() == 0 {
		return m
	}
	if m.Count() == 0 {
		return other
	}
	var res coretypes.Associative = m
	for iter := other.Iter(); iter.HasNext(); {
		p := iter.Next()
		res = res.Assoc(p.Key, p.Value)
	}
	return res.(coretypes.Map)
}

func (m *HashMap) Without(key coretypes.Object) coretypes.Map {
	if m.Root == nil {
		return m
	}
	newroot := m.Root.without(0, key.Hash(), key)
	if newroot == m.Root {
		return m
	}
	res := &HashMap{
		CountValue: m.CountValue - 1,
		Root:       newroot,
	}
	res.Meta = m.Meta
	return res
}

func (m *HashMap) Call(args []coretypes.Object) coretypes.Object {
	return CallMap(m, args, func(args []coretypes.Object, min int, max int) {
		if len(args) < min || len(args) > max {
			coretypes.RuntimePanicArityMinMax(len(args), min, max)
		}
	}, coretypes.RuntimeNil)
}

func NewHashMap(keyvals ...coretypes.Object) *HashMap {
	var res coretypes.Associative = EmptyHashMap
	for i := 0; i < len(keyvals); i += 2 {
		res = res.Assoc(keyvals[i], keyvals[i+1])
	}
	return res.(*HashMap)
}

func (m *HashMap) Empty() coretypes.Collection {
	return EmptyHashMap
}

func (m *HashMap) Pprint(w io.Writer, indent int) int {
	return PprintMap(m, w, indent, coretypes.RuntimePprintObject, coretypes.RuntimeWriteIndent)
}

func (m *HashMap) KVReduce(c coretypes.Callable, init coretypes.Object) coretypes.Object {
	res := init
	iter := m.Iter()
	for iter.HasNext() {
		kv := iter.Next()
		res = c.Call([]coretypes.Object{res, kv.Key, kv.Value})
	}
	return res
}
