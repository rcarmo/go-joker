package collections

func CloneInterfaces(s []interface{}) []interface{} {
	return CloneSlice(s)
}

func VectorTailOffset(count int) int {
	if count < TrieBranching {
		return 0
	}
	return ((count - 1) >> 5) << 5
}

func VectorArrayFor(i int, count int, shift uint, root []interface{}, tail []interface{}) []interface{} {
	if i >= VectorTailOffset(count) {
		return tail
	}
	node := root
	for level := shift; level > 0; level -= 5 {
		node = node[(i>>level)&0x01F].([]interface{})
	}
	return node
}

func VectorNewPath(level uint, node []interface{}) []interface{} {
	if level == 0 {
		return node
	}
	result := make([]interface{}, TrieBranching)
	result[0] = VectorNewPath(level-5, node)
	return result
}

func VectorPushTail(level uint, count int, parent []interface{}, tailNode []interface{}) []interface{} {
	subidx := ((count - 1) >> level) & 0x01F
	result := CloneInterfaces(parent)
	var nodeToInsert []interface{}
	if level == 5 {
		nodeToInsert = tailNode
	} else {
		if parent[subidx] != nil {
			nodeToInsert = VectorPushTail(level-5, count, parent[subidx].([]interface{}), tailNode)
		} else {
			nodeToInsert = VectorNewPath(level-5, tailNode)
		}
	}
	result[subidx] = nodeToInsert
	return result
}

func VectorPopTail(level uint, count int, node []interface{}) []interface{} {
	subidx := ((count - 2) >> level) & 0x01F
	if level > 5 {
		newChild := VectorPopTail(level-5, count, node[subidx].([]interface{}))
		if newChild == nil && subidx == 0 {
			return nil
		}
		ret := CloneInterfaces(node)
		ret[subidx] = newChild
		return ret
	}
	if subidx == 0 {
		return nil
	}
	ret := CloneInterfaces(node)
	ret[subidx] = nil
	return ret
}

func VectorAssocNode(level uint, node []interface{}, i int, val interface{}) []interface{} {
	ret := CloneInterfaces(node)
	if level == 0 {
		ret[i&0x01f] = val
	} else {
		subidx := (i >> level) & 0x01f
		ret[subidx] = VectorAssocNode(level-5, node[subidx].([]interface{}), i, val)
	}
	return ret
}
