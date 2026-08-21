package engine

import "sync"

const internShardCount = 32

type internShard struct {
	mu sync.Mutex
	m  map[string]string
}

var internShards [internShardCount]internShard

type internBuf struct {
	m map[string]string
}

var internBufPool = sync.Pool{New: func() any {
	return &internBuf{m: make(map[string]string, 1024)}
}}

func bindWorkerIntern() *internBuf {
	buf := internBufPool.Get().(*internBuf)
	if buf.m == nil {
		buf.m = make(map[string]string, 1024)
	}
	return buf
}

func releaseWorkerIntern(buf *internBuf) {
	if buf == nil {
		return
	}
	clear(buf.m)
	internBufPool.Put(buf)
}

func internString(value string) string {
	if value == "" {
		return ""
	}
	shard := &internShards[internShardIndex(value)]
	shard.mu.Lock()
	if shard.m == nil {
		shard.m = map[string]string{}
	}
	if existing, ok := shard.m[value]; ok {
		shard.mu.Unlock()
		return existing
	}
	shard.m[value] = value
	shard.mu.Unlock()
	return value
}

func internLocal(buf *internBuf, value string) string {
	if value == "" {
		return ""
	}
	if buf != nil {
		if existing, ok := buf.m[value]; ok {
			return existing
		}
	}
	interned := internString(value)
	if buf != nil {
		buf.m[value] = interned
	}
	return interned
}

func internShardIndex(value string) int {
	h := uint32(2166136261)
	for i := 0; i < len(value); i++ {
		h ^= uint32(value[i])
		h *= 16777619
	}
	return int(h % internShardCount)
}

// graphIntern assigns dense integer IDs to interned strings for compact edges
// (shared_ids table; edge identity is a 4-tuple of int32s).
type graphIntern struct {
	ids   map[string]int32
	names []string
}

func newGraphIntern(capacity int) *graphIntern {
	if capacity < 8 {
		capacity = 8
	}
	return &graphIntern{ids: make(map[string]int32, capacity), names: make([]string, 1, capacity+1)}
}

func (g *graphIntern) id(value string) int32 {
	if g == nil {
		return 0
	}
	if value == "" {
		return 0
	}
	if existing, ok := g.ids[value]; ok {
		return existing
	}
	interned := internString(value)
	if existing, ok := g.ids[interned]; ok {
		g.ids[value] = existing
		return existing
	}
	id := int32(len(g.names))
	g.ids[interned] = id
	if value != interned {
		g.ids[value] = id
	}
	g.names = append(g.names, interned)
	return id
}

type edgeKey struct {
	source, kind, target, extra int32
}

func edgeIdentityKey(intern *graphIntern, edge pendingEdge) edgeKey {
	extra := int32(0)
	if edge.kind == "IMPORTS" {
		localName, _ := edge.properties["local_name"].(string)
		extra = intern.id(localName)
	}
	return edgeKey{intern.id(edge.source), intern.id(edge.kind), intern.id(edge.target), extra}
}
