package engine

import (
	"context"
	"math"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// These values intentionally mirror the renderer we are matching. The
// initial directory-ring structure does most of the visual work; the short
// ForceAtlas-style pass only separates overlaps while anchor springs preserve
// the galaxy shape.
const (
	layoutDefaultMaxNodes = 5000
	layoutMaxMaxNodes     = 10_000_000
	layoutBHTheta         = 1.2
	layoutOctreeMaxDepth  = 26
	layoutOctreeMinHalf   = 1e-4
	layoutRepulsion       = 8.0
	layoutAttraction      = 1.0
	layoutAnchorK         = 0.25
	layoutIterations      = 40
	layoutDepthSpacing    = 50.0
)

type layoutBody struct {
	x, y, z    float64
	ax, ay, az float64
	fx, fy, fz float64
	mass       float64
}

type layoutOctree struct {
	cx, cy, cz float64
	totalMass  float64
	half       float64
	ox, oy, oz float64
	bodyIndex  int
	bodyMass   float64
	children   [8]*layoutOctree
}

func newLayoutOctree(x, y, z, half float64) *layoutOctree {
	return &layoutOctree{ox: x, oy: y, oz: z, half: half, bodyIndex: -1}
}

func (o *layoutOctree) octant(x, y, z float64) int {
	result := 0
	if x >= o.ox {
		result |= 1
	}
	if y >= o.oy {
		result |= 2
	}
	if z >= o.oz {
		result |= 4
	}
	return result
}

func (o *layoutOctree) child(index int) *layoutOctree {
	if o.children[index] != nil {
		return o.children[index]
	}
	quarter := o.half * 0.5
	x, y, z := o.ox-quarter, o.oy-quarter, o.oz-quarter
	if index&1 != 0 {
		x = o.ox + quarter
	}
	if index&2 != 0 {
		y = o.oy + quarter
	}
	if index&4 != 0 {
		z = o.oz + quarter
	}
	o.children[index] = newLayoutOctree(x, y, z, quarter)
	return o.children[index]
}

func (o *layoutOctree) insert(index int, x, y, z, mass float64, depth int) {
	if o.totalMass == 0 && o.bodyIndex == -1 {
		o.bodyIndex, o.bodyMass = index, mass
		o.cx, o.cy, o.cz, o.totalMass = x, y, z, mass
		return
	}
	// Coincident points otherwise subdivide forever.
	if depth >= layoutOctreeMaxDepth || o.half < layoutOctreeMinHalf {
		nextMass := o.totalMass + mass
		o.cx = (o.cx*o.totalMass + x*mass) / nextMass
		o.cy = (o.cy*o.totalMass + y*mass) / nextMass
		o.cz = (o.cz*o.totalMass + z*mass) / nextMass
		o.totalMass = nextMass
		o.bodyIndex = -1
		return
	}
	if o.bodyIndex >= 0 {
		oldIndex, oldMass := o.bodyIndex, o.bodyMass
		oldX, oldY, oldZ := o.cx, o.cy, o.cz
		o.bodyIndex = -1
		o.child(o.octant(oldX, oldY, oldZ)).insert(
			oldIndex, oldX, oldY, oldZ, oldMass, depth+1,
		)
	}
	nextMass := o.totalMass + mass
	o.cx = (o.cx*o.totalMass + x*mass) / nextMass
	o.cy = (o.cy*o.totalMass + y*mass) / nextMass
	o.cz = (o.cz*o.totalMass + z*mass) / nextMass
	o.totalMass = nextMass
	o.child(o.octant(x, y, z)).insert(index, x, y, z, mass, depth+1)
}

func (o *layoutOctree) repulse(
	x, y, z, mass float64,
	self int,
	fx, fy, fz *float64,
) {
	if o == nil || o.totalMass == 0 || o.bodyIndex == self {
		return
	}
	dx, dy, dz := x-o.cx, y-o.cy, z-o.cz
	distance := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if o.bodyIndex >= 0 || (o.half*2/(distance+0.001)) < layoutBHTheta {
		distance = math.Max(distance, 0.01)
		force := layoutRepulsion * mass * o.totalMass / distance
		*fx += force * dx / distance
		*fy += force * dy / distance
		*fz += force * dz / distance
		return
	}
	for _, child := range o.children {
		if child != nil {
			child.repulse(x, y, z, mass, self, fx, fy, fz)
		}
	}
}

// Layout produces the same render contract as the established 3D graph UI:
// directory-anchored galaxies, call-depth Z layers, stellar degree colors,
// and every edge whose endpoints are in the selected node budget.
func (s *Store) Layout(ctx context.Context, req api.LayoutRequest) (api.LayoutResult, error) {
	if req.Project == "" {
		req.Project, _ = s.defaultProject(ctx)
	}
	maxNodes := req.MaxNodes
	if maxNodes <= 0 {
		maxNodes = layoutDefaultMaxNodes
	}
	if maxNodes > layoutMaxMaxNodes {
		maxNodes = layoutMaxMaxNodes
	}

	result := api.LayoutResult{Project: req.Project}
	if err := s.db.QueryRowContext(
		ctx, `SELECT count(*) FROM nodes WHERE project=?`, req.Project,
	).Scan(&result.TotalNodes); err != nil {
		return result, err
	}
	if err := s.db.QueryRowContext(
		ctx, `SELECT count(*) FROM edges WHERE project=?`, req.Project,
	).Scan(&result.TotalEdges); err != nil {
		return result, err
	}

	nodes, err := s.layoutNodes(ctx, req.Project, maxNodes)
	if err != nil || len(nodes) == 0 {
		return result, err
	}
	edges, err := s.layoutEdges(ctx, req.Project, nodes)
	if err != nil {
		return result, err
	}
	assignLayoutMetadata(nodes, edges)
	positionLayout3D(nodes, edges)
	if communities, communityErr := s.communityLabelByNodeID(ctx, req.Project); communityErr == nil {
		for i := range nodes {
			nodes[i].Community = communities[nodes[i].ID]
		}
	}
	result.Nodes, result.Edges = nodes, edges
	return result, nil
}

func (s *Store) layoutNodes(
	ctx context.Context,
	project string,
	maxNodes int,
) ([]api.LayoutNode, error) {
	// Degree ordering preserves the connected overview when a budget is lower
	// than the graph. Ties remain stable across rebuilds.
	rows, err := s.db.QueryContext(ctx, `
SELECT n.id,n.label,n.name,n.qualified_name,n.file_path,
       n.start_line,n.end_line,COALESCE(d.degree,0)
FROM nodes n
LEFT JOIN (
  SELECT node_id,count(*) degree FROM (
    SELECT source_id node_id FROM edges WHERE project=?
    UNION ALL
    SELECT target_id node_id FROM edges WHERE project=?
  ) GROUP BY node_id
) d ON d.node_id=n.id
WHERE n.project=?
ORDER BY COALESCE(d.degree,0) DESC,n.id ASC
LIMIT ?`, project, project, project, maxNodes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nodes := make([]api.LayoutNode, 0, maxNodes)
	for rows.Next() {
		var node api.LayoutNode
		if err := rows.Scan(
			&node.ID, &node.Label, &node.Name, &node.QualifiedName,
			&node.FilePath, &node.StartLine, &node.EndLine, &node.Degree,
		); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func (s *Store) layoutEdges(
	ctx context.Context,
	project string,
	nodes []api.LayoutNode,
) ([]api.LayoutEdge, error) {
	keep := make(map[int64]struct{}, len(nodes))
	for _, node := range nodes {
		keep[node.ID] = struct{}{}
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT source_id,target_id,type FROM edges WHERE project=? ORDER BY id`,
		project,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	edges := make([]api.LayoutEdge, 0)
	for rows.Next() {
		var edge api.LayoutEdge
		if err := rows.Scan(&edge.Source, &edge.Target, &edge.Type); err != nil {
			return nil, err
		}
		if _, ok := keep[edge.Source]; !ok {
			continue
		}
		if _, ok := keep[edge.Target]; !ok {
			continue
		}
		edges = append(edges, edge)
	}
	return edges, rows.Err()
}

func assignLayoutMetadata(nodes []api.LayoutNode, edges []api.LayoutEdge) {
	index := make(map[int64]int, len(nodes))
	degree := make([]int, len(nodes))
	for i := range nodes {
		index[nodes[i].ID] = i
	}
	for _, edge := range edges {
		source, sourceOK := index[edge.Source]
		target, targetOK := index[edge.Target]
		if sourceOK {
			degree[source]++
		}
		if targetOK {
			degree[target]++
		}
	}
	for i := range nodes {
		nodes[i].Degree = degree[i]
		nodes[i].Color = stellarColor(degree[i])
		nodes[i].Size = sizeForLabel(nodes[i].Label)
		if degree[i] > 5 {
			nodes[i].Size += math.Min(float64(degree[i])*0.3, 10)
		}
	}
}

func stellarColor(degree int) string {
	switch {
	case degree <= 1:
		return "#ff6050"
	case degree <= 3:
		return "#ff8855"
	case degree <= 5:
		return "#ffa060"
	case degree <= 8:
		return "#ffc070"
	case degree <= 12:
		return "#ffe080"
	case degree <= 18:
		return "#fff0c0"
	case degree <= 25:
		return "#fff8e8"
	case degree <= 35:
		return "#e8e8ff"
	case degree <= 50:
		return "#c0d0ff"
	default:
		return "#80a0ff"
	}
}

func sizeForLabel(label string) float64 {
	switch label {
	case "Project":
		return 20
	case "Package", "Module":
		return 15
	case "Folder":
		return 12
	case "File":
		return 8
	case "Class", "Struct", "Interface":
		return 6
	default:
		return 4
	}
}

func positionLayout3D(nodes []api.LayoutNode, edges []api.LayoutEdge) {
	index := make(map[int64]int, len(nodes))
	labels := make([]string, len(nodes))
	for i := range nodes {
		index[nodes[i].ID] = i
		labels[i] = nodes[i].Label
	}
	sources := make([]int, 0, len(edges))
	targets := make([]int, 0, len(edges))
	for _, edge := range edges {
		source, sourceOK := index[edge.Source]
		target, targetOK := index[edge.Target]
		if sourceOK && targetOK {
			sources = append(sources, source)
			targets = append(targets, target)
		}
	}
	depth := layoutCallDepth(len(nodes), sources, targets, labels)
	bodies := make([]layoutBody, len(nodes))
	for i := range nodes {
		cluster := layoutClusterKey(nodes[i].FilePath)
		hash := fnv1a32(cluster)
		angle := (float64(hash&0xffff) / 65535) * 6.2832
		radius := 500 + (float64((hash>>16)&0xff)/255)*250
		seed := fnv1a32(nodes[i].QualifiedName)
		x := radius*math.Cos(angle) + layoutRandom(&seed)*40
		y := radius*math.Sin(angle) + layoutRandom(&seed)*40
		z := -float64(depth[i]) * layoutDepthSpacing
		mass := float64(nodes[i].Degree + 1)
		bodies[i] = layoutBody{
			x: x, y: y, z: z,
			ax: x, ay: y, az: z,
			mass: mass,
		}
	}
	localOptimize3D(bodies, sources, targets)
	for i := range nodes {
		nodes[i].X = finiteRounded(bodies[i].x)
		nodes[i].Y = finiteRounded(bodies[i].y)
		nodes[i].Z = finiteRounded(bodies[i].z)
	}
}

func layoutCallDepth(
	nodeCount int,
	sources, targets []int,
	labels []string,
) []int {
	depth := make([]int, nodeCount)
	for i := range depth {
		depth[i] = -1
	}
	queue := make([]int, 0, nodeCount)
	for i, label := range labels {
		switch label {
		case "Route", "File", "Module", "Package":
			depth[i] = 0
			queue = append(queue, i)
		}
	}
	if len(queue) == 0 {
		inDegree := make([]int, nodeCount)
		for _, target := range targets {
			inDegree[target]++
		}
		for i, count := range inDegree {
			if count == 0 {
				depth[i] = 0
				queue = append(queue, i)
			}
		}
	}
	outgoing := make([][]int, nodeCount)
	for i, source := range sources {
		outgoing[source] = append(outgoing[source], targets[i])
	}
	for head := 0; head < len(queue); head++ {
		current := queue[head]
		for _, target := range outgoing[current] {
			if depth[target] == -1 {
				depth[target] = depth[current] + 1
				queue = append(queue, target)
			}
		}
	}
	for i := range depth {
		if depth[i] < 0 {
			depth[i] = 0
		}
	}
	return depth
}

func localOptimize3D(bodies []layoutBody, sources, targets []int) {
	iterations := layoutIterations
	if len(bodies) > 500_000 {
		iterations = 10
	} else if len(bodies) > 100_000 {
		iterations = 20
	}
	for iteration := 0; iteration < iterations; iteration++ {
		minX, minY, minZ := math.Inf(1), math.Inf(1), math.Inf(1)
		maxX, maxY, maxZ := math.Inf(-1), math.Inf(-1), math.Inf(-1)
		for i := range bodies {
			bodies[i].fx, bodies[i].fy, bodies[i].fz = 0, 0, 0
			minX, minY, minZ = math.Min(minX, bodies[i].x), math.Min(minY, bodies[i].y), math.Min(minZ, bodies[i].z)
			maxX, maxY, maxZ = math.Max(maxX, bodies[i].x), math.Max(maxY, bodies[i].y), math.Max(maxZ, bodies[i].z)
		}
		half := math.Max(maxX-minX, math.Max(maxY-minY, maxZ-minZ))*0.5 + 1
		root := newLayoutOctree((minX+maxX)*0.5, (minY+maxY)*0.5, (minZ+maxZ)*0.5, half)
		for i := range bodies {
			root.insert(i, bodies[i].x, bodies[i].y, bodies[i].z, bodies[i].mass, 0)
		}
		for i := range bodies {
			root.repulse(
				bodies[i].x, bodies[i].y, bodies[i].z, bodies[i].mass, i,
				&bodies[i].fx, &bodies[i].fy, &bodies[i].fz,
			)
		}
		for i, source := range sources {
			target := targets[i]
			dx := bodies[target].x - bodies[source].x
			dy := bodies[target].y - bodies[source].y
			dz := bodies[target].z - bodies[source].z
			bodies[source].fx += dx * layoutAttraction
			bodies[source].fy += dy * layoutAttraction
			bodies[source].fz += dz * layoutAttraction
			bodies[target].fx -= dx * layoutAttraction
			bodies[target].fy -= dy * layoutAttraction
			bodies[target].fz -= dz * layoutAttraction
		}
		for i := range bodies {
			bodies[i].fx += (bodies[i].ax - bodies[i].x) * layoutAnchorK * bodies[i].mass
			bodies[i].fy += (bodies[i].ay - bodies[i].y) * layoutAnchorK * bodies[i].mass
			bodies[i].fz += (bodies[i].az - bodies[i].z) * layoutAnchorK * bodies[i].mass
			force := math.Sqrt(
				bodies[i].fx*bodies[i].fx +
					bodies[i].fy*bodies[i].fy +
					bodies[i].fz*bodies[i].fz,
			)
			speed := 1.0
			if force > 8 {
				speed = 8 / (force + 0.001)
			}
			bodies[i].x += bodies[i].fx * speed
			bodies[i].y += bodies[i].fy * speed
			bodies[i].z += bodies[i].fz * speed
		}
	}
}

func layoutClusterKey(filePath string) string {
	parts := strings.Split(filePath, "/")
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return strings.Join(parts, "/")
}

func fnv1a32(value string) uint32 {
	hash := uint32(2166136261)
	for i := 0; i < len(value); i++ {
		hash ^= uint32(value[i])
		hash *= 16777619
	}
	return hash
}

func layoutRandom(seed *uint32) float64 {
	*seed = *seed*1103515245 + 12345
	return float64((*seed>>16)&0x7fff)/32768 - 0.5
}

func finiteRounded(value float64) float64 {
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return 0
	}
	return math.Round(value*1000) / 1000
}
