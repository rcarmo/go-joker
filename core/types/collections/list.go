package collections

import (
	"io"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

type List struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	FirstValue coretypes.Object
	RestValue  *List
	CountValue int
	Node       *ListNode[coretypes.Object]
}

func NewList(first coretypes.Object, rest *List) *List {
	var restNode *ListNode[coretypes.Object]
	if rest != nil {
		restNode = rest.listNode()
	}
	node := MaterializeListNode(nil, 1, first, restNode)
	return &List{FirstValue: first, RestValue: rest, CountValue: node.Count(), Node: node}
}

func (list *List) listNode() *ListNode[coretypes.Object] {
	if list.Node != nil {
		return list.Node
	}
	if list.CountValue == 0 {
		list.Node = MaterializeListNode(nil, 0, list.FirstValue, nil)
		return list.Node
	}
	var restNode *ListNode[coretypes.Object]
	if list.RestValue != nil {
		restNode = list.RestValue.listNode()
	}
	list.Node = MaterializeListNode(nil, list.CountValue, list.FirstValue, restNode)
	return list.Node
}

func NewListFrom(objs ...coretypes.Object) *List {
	return BuildListFromReverse(objs, EmptyList, func(list *List, obj coretypes.Object) *List {
		return list.conj(obj)
	})
}

func (list *List) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	list.Info = info
	return list
}

func (list *List) WithMeta(meta coretypes.Map) coretypes.Object {
	res := *list
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return &res
}

func (list *List) conj(obj coretypes.Object) *List {
	return NewList(obj, list)
}

func (list *List) Conj(obj coretypes.Object) coretypes.Conjable {
	return list.conj(obj)
}

func (list *List) ToString(escape bool) string {
	return SeqToString(list, func(obj coretypes.Object) string { return obj.ToString(escape) })
}

func (seq *List) Pprint(w io.Writer, indent int) int {
	return pprintSeq(seq, w, indent)
}

func (seq *List) Format(w io.Writer, indent int) int {
	return pprintSeq(seq, w, indent)
}

func (list *List) Equals(other interface{}) bool {
	return coretypes.IsSeqEqual(list, other)
}

func (list *List) GetType() *coretypes.Type {
	return coretypes.RuntimeTypes.List
}

func (list *List) Hash() uint32 {
	return HashOrdered(list)
}

func (list *List) First() coretypes.Object {
	return list.listNode().First()
}

func (list *List) Rest() coretypes.Seq {
	if list.RestValue != nil {
		return list.RestValue
	}
	return &List{Node: list.listNode().Rest()}
}

func (list *List) IsEmpty() bool {
	return list.listNode().IsEmpty()
}

func (list *List) Cons(obj coretypes.Object) coretypes.Seq {
	return list.conj(obj)
}

func (list *List) Seq() coretypes.Seq {
	return list
}

func (list *List) Second() coretypes.Object {
	return ListSecond(list.listNode())
}

func (list *List) Third() coretypes.Object {
	return ListThird(list.listNode())
}

func (list *List) Forth() coretypes.Object {
	return ListFourth(list.listNode())
}

func (list *List) Count() int {
	return list.listNode().Count()
}

func (list *List) Empty() coretypes.Collection {
	return EmptyList
}

func (list *List) Peek() coretypes.Object {
	return list.FirstValue
}

func (list *List) Pop() coretypes.Stack {
	return ListPop(list.CountValue, list.RestValue, func(msg string) any { return coretypes.RuntimeError(msg) })
}

func (list *List) SequentialMarker() {}

var EmptyList = &List{FirstValue: coretypes.RuntimeNil, Node: NewEmptyListNode[coretypes.Object](coretypes.RuntimeNil)}

func init() {
	EmptyList.RestValue = EmptyList
}
