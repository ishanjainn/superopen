package engine

import "testing"

func TestLeidenPinnedBasicAndDegenerateGraphs(t *testing.T) {
	if got := Leiden(nil, nil, 1); len(got) != 0 {
		t.Fatalf("empty=%+v", got)
	}
	single := Leiden([]int64{42}, nil, 1)
	if len(single) != 1 || single[0] != (LeidenMembership{NodeID: 42, Community: 0}) {
		t.Fatalf("single=%+v", single)
	}
	nodes := []int64{1, 2, 3, 4, 5}
	edges := []LeidenEdge{{1, 2}, {2, 3}, {1, 3}, {4, 5}}
	got := Leiden(nodes, edges, 1)
	communities := membershipMap(got)
	if communities[1] != communities[2] || communities[2] != communities[3] || communities[4] != communities[5] || communities[1] == communities[4] {
		t.Fatalf("memberships=%+v", got)
	}
}

func TestLeidenMultilevelCommunitiesAreConnected(t *testing.T) {
	const clusterCount, clusterSize = 4, 8
	nodes := make([]int64, clusterCount*clusterSize)
	for index := range nodes {
		nodes[index] = int64(index + 1)
	}
	var edges []LeidenEdge
	for cluster := 0; cluster < clusterCount; cluster++ {
		base := cluster*clusterSize + 1
		for left := 0; left < clusterSize; left++ {
			for right := left + 1; right < clusterSize; right++ {
				edges = append(edges, LeidenEdge{int64(base + left), int64(base + right)})
			}
		}
	}
	for cluster := 0; cluster+1 < clusterCount; cluster++ {
		edges = append(edges, LeidenEdge{int64(cluster*clusterSize + 1), int64((cluster+1)*clusterSize + 1)})
	}
	got := Leiden(nodes, edges, 1)
	count := distinctMemberships(got)
	if count < 2 || count > clusterCount+1 {
		t.Fatalf("community count=%d, memberships=%+v", count, got)
	}
	if !membershipsConnected(got, edges) {
		t.Fatalf("disconnected membership=%+v", got)
	}
	for cluster := 0; cluster < clusterCount; cluster++ {
		base := cluster * clusterSize
		same := 0
		for index := 0; index < clusterSize; index++ {
			if got[base+index].Community == got[base].Community {
				same++
			}
		}
		if same < clusterSize-1 {
			t.Fatalf("cluster %d retained only %d members: %+v", cluster, same, got)
		}
	}
}

func TestLeidenResolutionControlsGranularity(t *testing.T) {
	nodes := make([]int64, 30)
	var edges []LeidenEdge
	for index := range nodes {
		nodes[index] = int64(index + 1)
		if index > 0 {
			edges = append(edges, LeidenEdge{int64(index), int64(index + 1)})
		}
	}
	low := Leiden(nodes, edges, .1)
	high := Leiden(nodes, edges, 5)
	if distinctMemberships(high) <= distinctMemberships(low) {
		t.Fatalf("low=%d high=%d", distinctMemberships(low), distinctMemberships(high))
	}
	if !membershipsConnected(low, edges) || !membershipsConnected(high, edges) {
		t.Fatal("resolution result contains a disconnected community")
	}
}

func membershipMap(memberships []LeidenMembership) map[int64]int {
	result := map[int64]int{}
	for _, membership := range memberships {
		result[membership.NodeID] = membership.Community
	}
	return result
}

func distinctMemberships(memberships []LeidenMembership) int {
	seen := map[int]bool{}
	for _, membership := range memberships {
		seen[membership.Community] = true
	}
	return len(seen)
}

func membershipsConnected(memberships []LeidenMembership, edges []LeidenEdge) bool {
	index := map[int64]int{}
	for position, membership := range memberships {
		index[membership.NodeID] = position
	}
	checked := map[int]bool{}
	for seed, membership := range memberships {
		if checked[membership.Community] {
			continue
		}
		checked[membership.Community] = true
		seen := map[int]bool{seed: true}
		queue := []int{seed}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			for _, edge := range edges {
				other := int64(0)
				switch memberships[current].NodeID {
				case edge.Source:
					other = edge.Target
				case edge.Target:
					other = edge.Source
				default:
					continue
				}
				otherIndex, ok := index[other]
				if ok && !seen[otherIndex] && memberships[otherIndex].Community == membership.Community {
					seen[otherIndex] = true
					queue = append(queue, otherIndex)
				}
			}
		}
		for candidate, item := range memberships {
			if item.Community == membership.Community && !seen[candidate] {
				return false
			}
		}
	}
	return true
}
