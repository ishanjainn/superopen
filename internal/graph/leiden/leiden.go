package leiden

// LeidenEdge is an unweighted graph edge. Leiden treats edges as undirected,
// drops self loops, and accumulates duplicate edges as integer weights.
type Edge struct {
	Source int64
	Target int64
	Weight float64
}

type Membership struct {
	NodeID    int64
	Community int
}

const (
	leidenMaxLevels  = 64
	leidenMovePasses = 100
)

type leidenGraph struct {
	offsets   []int
	neighbors []int
	weights   []float64
	degree    []float64
}

// Leiden reproduces the pinned engine's deterministic Leiden-style community
// detection. A non-positive resolution uses the Superopen default of 1.0.
func Detect(nodes []int64, edges []Edge, resolution float64) []Membership {
	result := make([]Membership, len(nodes))
	for index, nodeID := range nodes {
		result[index] = Membership{NodeID: nodeID, Community: index}
	}
	if len(nodes) == 0 {
		return result
	}
	sources, targets, weights := leidenWeights(nodes, edges)
	if len(sources) == 0 {
		return result
	}
	graph := buildLeidenGraph(len(nodes), sources, targets, weights)
	twom := 0.0
	for _, value := range graph.degree {
		twom += value
	}
	if twom <= 0 {
		return result
	}
	gamma := resolution
	if gamma <= 0 {
		gamma = 1
	}
	original := make([]int, len(nodes))
	community := make([]int, len(nodes))
	for index := range nodes {
		original[index] = index
		community[index] = index
	}
	for level := 0; level < leidenMaxLevels; level++ {
		leidenMove(graph, community, gamma, twom)
		communityCount := leidenRelabel(community)
		if communityCount >= len(graph.degree) {
			break
		}
		refined := make([]int, len(graph.degree))
		refinedCount := leidenRefine(graph, community, gamma, twom, refined)
		if refinedCount >= len(graph.degree) {
			break
		}
		for index := range original {
			original[index] = refined[original[index]]
		}
		next, seed := leidenAggregate(graph, refined, refinedCount, community)
		graph = next
		community = seed
	}
	for index := range result {
		result[index].Community = community[original[index]]
	}
	return result
}

func leidenWeights(nodes []int64, edges []Edge) ([]int, []int, []float64) {
	nodeIndex := make(map[int64]int, len(nodes))
	for index, nodeID := range nodes {
		nodeIndex[nodeID] = index
	}
	var sources, targets []int
	var weights []float64
	for _, edge := range edges {
		source, sourceOK := nodeIndex[edge.Source]
		target, targetOK := nodeIndex[edge.Target]
		if !sourceOK || !targetOK || source == target {
			continue
		}
		if source > target {
			source, target = target, source
		}
		found := -1
		for index := range sources {
			if sources[index] == source && targets[index] == target {
				found = index
				break
			}
		}
		w := edge.Weight
		if w <= 0 {
			w = 1
		}
		if found >= 0 {
			weights[found] += w
		} else {
			sources = append(sources, source)
			targets = append(targets, target)
			weights = append(weights, w)
		}
	}
	return sources, targets, weights
}

func buildLeidenGraph(nodeCount int, sources, targets []int, weights []float64) leidenGraph {
	graph := leidenGraph{offsets: make([]int, nodeCount+1), degree: make([]float64, nodeCount)}
	for index := range sources {
		graph.offsets[sources[index]+1]++
		graph.offsets[targets[index]+1]++
	}
	for index := 0; index < nodeCount; index++ {
		graph.offsets[index+1] += graph.offsets[index]
	}
	graph.neighbors = make([]int, graph.offsets[nodeCount])
	graph.weights = make([]float64, graph.offsets[nodeCount])
	fill := append([]int(nil), graph.offsets[:nodeCount]...)
	for index, source := range sources {
		target, weight := targets[index], weights[index]
		graph.neighbors[fill[source]] = target
		graph.weights[fill[source]] = weight
		fill[source]++
		graph.neighbors[fill[target]] = source
		graph.weights[fill[target]] = weight
		fill[target]++
		graph.degree[source] += weight
		graph.degree[target] += weight
	}
	return graph
}

func leidenMove(graph leidenGraph, community []int, gamma, twom float64) {
	nodeCount := len(graph.degree)
	communityDegree := make([]float64, nodeCount)
	accumulated := make([]float64, nodeCount)
	queue := make([]int, nodeCount)
	dirty := make([]int, nodeCount)
	inQueue := make([]bool, nodeCount)
	for node := 0; node < nodeCount; node++ {
		communityDegree[community[node]] += graph.degree[node]
		queue[node] = node
		inQueue[node] = true
	}
	head, count := 0, nodeCount
	remaining := nodeCount*leidenMovePasses + leidenMaxLevels
	for count > 0 && remaining > 0 {
		remaining--
		node := queue[head]
		head = (head + 1) % nodeCount
		count--
		inQueue[node] = false
		current := community[node]
		dirtyCount := 0
		for edge := graph.offsets[node]; edge < graph.offsets[node+1]; edge++ {
			neighbor := graph.neighbors[edge]
			if neighbor == node {
				continue
			}
			candidate := community[neighbor]
			if accumulated[candidate] == 0 {
				dirty[dirtyCount] = candidate
				dirtyCount++
			}
			accumulated[candidate] += graph.weights[edge]
		}
		communityDegree[current] -= graph.degree[node]
		degree := graph.degree[node]
		best := current
		bestGain := accumulated[current] - gamma*degree*communityDegree[current]/twom
		for _, candidate := range dirty[:dirtyCount] {
			gain := accumulated[candidate] - gamma*degree*communityDegree[candidate]/twom
			if gain > bestGain {
				bestGain = gain
				best = candidate
			}
		}
		communityDegree[best] += degree
		community[node] = best
		if best != current {
			for edge := graph.offsets[node]; edge < graph.offsets[node+1]; edge++ {
				neighbor := graph.neighbors[edge]
				if community[neighbor] != best && !inQueue[neighbor] && count < nodeCount {
					queue[(head+count)%nodeCount] = neighbor
					count++
					inQueue[neighbor] = true
				}
			}
		}
		for _, candidate := range dirty[:dirtyCount] {
			accumulated[candidate] = 0
		}
	}
}

func leidenRelabel(community []int) int {
	labels := make([]int, len(community))
	for index := range labels {
		labels[index] = -1
	}
	next := 0
	for index, label := range community {
		if labels[label] < 0 {
			labels[label] = next
			next++
		}
		community[index] = labels[label]
	}
	return next
}

func leidenRefine(graph leidenGraph, community []int, gamma, twom float64, refined []int) int {
	nodeCount := len(graph.degree)
	communityDegree := make([]float64, nodeCount)
	accumulated := make([]float64, nodeCount)
	sizes := make([]int, nodeCount)
	dirty := make([]int, nodeCount)
	for node := 0; node < nodeCount; node++ {
		refined[node] = node
		communityDegree[node] = graph.degree[node]
		sizes[node] = 1
	}
	for node := 0; node < nodeCount; node++ {
		if sizes[refined[node]] != 1 {
			continue
		}
		moveCommunity := community[node]
		dirtyCount := 0
		for edge := graph.offsets[node]; edge < graph.offsets[node+1]; edge++ {
			neighbor := graph.neighbors[edge]
			if neighbor == node || community[neighbor] != moveCommunity {
				continue
			}
			candidate := refined[neighbor]
			if accumulated[candidate] == 0 {
				dirty[dirtyCount] = candidate
				dirtyCount++
			}
			accumulated[candidate] += graph.weights[edge]
		}
		current := refined[node]
		degree := graph.degree[node]
		communityDegree[current] -= degree
		best, bestGain := current, 0.0
		for _, candidate := range dirty[:dirtyCount] {
			if candidate == current {
				continue
			}
			gain := accumulated[candidate] - gamma*degree*communityDegree[candidate]/twom
			if gain > bestGain {
				bestGain = gain
				best = candidate
			}
		}
		if best != current {
			refined[node] = best
			communityDegree[best] += degree
			sizes[best]++
			sizes[current]--
		} else {
			communityDegree[current] += degree
		}
		for _, candidate := range dirty[:dirtyCount] {
			accumulated[candidate] = 0
		}
	}
	return leidenRelabel(refined)
}

func leidenAggregate(graph leidenGraph, refined []int, refinedCount int, community []int) (leidenGraph, []int) {
	nodeCount := len(graph.degree)
	degree := make([]float64, refinedCount)
	memberCount := make([]int, refinedCount)
	starts := make([]int, refinedCount+1)
	members := make([]int, nodeCount)
	fill := make([]int, refinedCount)
	seed := make([]int, refinedCount)
	for index := range seed {
		seed[index] = -1
	}
	for node := 0; node < nodeCount; node++ {
		group := refined[node]
		degree[group] += graph.degree[node]
		memberCount[group]++
		if seed[group] < 0 {
			seed[group] = community[node]
		}
	}
	for group := 0; group < refinedCount; group++ {
		starts[group+1] = starts[group] + memberCount[group]
		fill[group] = starts[group]
	}
	for node := 0; node < nodeCount; node++ {
		group := refined[node]
		members[fill[group]] = node
		fill[group]++
	}
	offsets := make([]int, refinedCount+1)
	accumulated := make([]float64, refinedCount)
	dirty := make([]int, refinedCount)
	for group := 0; group < refinedCount; group++ {
		dirtyCount := 0
		for member := starts[group]; member < starts[group+1]; member++ {
			node := members[member]
			for edge := graph.offsets[node]; edge < graph.offsets[node+1]; edge++ {
				other := refined[graph.neighbors[edge]]
				if other == group || accumulated[other] != 0 {
					continue
				}
				accumulated[other] = 1
				dirty[dirtyCount] = other
				dirtyCount++
			}
		}
		offsets[group+1] = offsets[group] + dirtyCount
		for _, other := range dirty[:dirtyCount] {
			accumulated[other] = 0
		}
	}
	neighbors := make([]int, offsets[refinedCount])
	weights := make([]float64, offsets[refinedCount])
	for group := 0; group < refinedCount; group++ {
		dirtyCount := 0
		for member := starts[group]; member < starts[group+1]; member++ {
			node := members[member]
			for edge := graph.offsets[node]; edge < graph.offsets[node+1]; edge++ {
				other := refined[graph.neighbors[edge]]
				if other == group {
					continue
				}
				if accumulated[other] == 0 {
					dirty[dirtyCount] = other
					dirtyCount++
				}
				accumulated[other] += graph.weights[edge]
			}
		}
		for index, other := range dirty[:dirtyCount] {
			neighbors[offsets[group]+index] = other
			weights[offsets[group]+index] = accumulated[other]
			accumulated[other] = 0
		}
	}
	return leidenGraph{offsets: offsets, neighbors: neighbors, weights: weights, degree: degree}, seed
}
