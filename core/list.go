package core

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"

	corecollections "github.com/rcarmo/go-joker/core/collections"
)

type List struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	first coretypes.Object
	rest  *List
	count int
	node  *corecollections.ListNode[coretypes.Object]
}

func NewList(first coretypes.Object, rest *List) *List {
	var restNode *corecollections.ListNode[coretypes.Object]
	if rest != nil {
		restNode = rest.listNode()
	}
	node := corecollections.NewListNode(first, restNode)
	return &List{first: first, rest: rest, count: node.Count(), node: node}
}

func (list *List) listNode() *corecollections.ListNode[coretypes.Object] {
	if list.node != nil {
		return list.node
	}
	if list.count == 0 {
		list.node = corecollections.NewEmptyListNode[coretypes.Object](list.first)
		return list.node
	}
	var restNode *corecollections.ListNode[coretypes.Object]
	if list.rest != nil {
		restNode = list.rest.listNode()
	}
	list.node = corecollections.NewListNode(list.first, restNode)
	return list.node
}

func NewListFrom(objs ...coretypes.Object) *List {
	res := EmptyList
	for i := len(objs) - 1; i >= 0; i-- {
		res = res.conj(objs[i])
	}
	return res
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
	return SeqToString(list, escape)
}

func (seq *List) Pprint(w io.Writer, indent int) int {
	return pprintSeq(seq, w, indent)
}

func (seq *List) Format(w io.Writer, indent int) int {
	return formatSeq(seq, w, indent)
}

func (list *List) Equals(other interface{}) bool {
	return IsSeqEqual(list, other)
}

func (list *List) GetType() *coretypes.Type {
	return TYPE.List
}

func (list *List) Hash() uint32 {
	return hashOrdered(list)
}

func (list *List) First() coretypes.Object {
	return list.listNode().First()
}

func (list *List) Rest() coretypes.Seq {
	if list.rest != nil {
		return list.rest
	}
	return &List{node: list.listNode().Rest()}
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
	return list.listNode().Rest().First()
}

func (list *List) Third() coretypes.Object {
	return list.listNode().Rest().Rest().First()
}

func (list *List) Forth() coretypes.Object {
	return list.listNode().Rest().Rest().Rest().First()
}

func (list *List) Count() int {
	return list.listNode().Count()
}

func (list *List) Empty() coretypes.Collection {
	return EmptyList
}

func (list *List) Peek() coretypes.Object {
	return list.first
}

func (list *List) Pop() coretypes.Stack {
	if list.count == 0 {
		panic(RT.NewError("Can't pop empty list"))
	}
	return list.rest
}

func (list *List) SequentialMarker() {}

var EmptyList = &List{first: Nil{}, node: corecollections.NewEmptyListNode[coretypes.Object](Nil{})}

func init() {
	EmptyList.rest = EmptyList
}
