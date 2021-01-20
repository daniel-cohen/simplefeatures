package geom

import (
	"fmt"
)

type doublyConnectedEdgeList struct {
	faces     []*faceRecord // only populated in the overlay
	halfEdges []*halfEdgeRecord
	vertices  map[XY]*vertexRecord
}

type faceRecord struct {
	cycle *halfEdgeRecord
	label uint8
}

func (f *faceRecord) String() string {
	if f == nil {
		return "nil"
	}
	return "[" + f.cycle.String() + "]"
}

type halfEdgeRecord struct {
	origin       *vertexRecord
	twin         *halfEdgeRecord
	incident     *faceRecord // only populated in the overlay
	next, prev   *halfEdgeRecord
	intermediate []XY
	edgeLabel    uint8
	faceLabel    uint8
}

// secondXY gives the second (1-indexed) XY in the edge. This is either the
// first intermediate XY, or the origin of the next/twin edge in the case where
// there are no intermediates.
func (e *halfEdgeRecord) secondXY() XY {
	if len(e.intermediate) == 0 {
		return e.twin.origin.coords
	}
	return e.intermediate[0]
}

// String shows the origin and destination of the edge (for debugging
// purposes). We can remove this once DCEL active development is completed.
func (e *halfEdgeRecord) String() string {
	if e == nil {
		return "nil"
	}
	return fmt.Sprintf("%v->%v->%v", e.origin.coords, e.intermediate, e.twin.origin.coords)
}

type vertexRecord struct {
	coords    XY
	incidents []*halfEdgeRecord
	label     uint8
	locLabel  uint8
}

func forEachEdge(start *halfEdgeRecord, fn func(*halfEdgeRecord)) {
	e := start
	for {
		fn(e)
		e = e.next
		if e == start {
			break
		}
	}
}

func newDCELFromGeometry(g Geometry, ghosts MultiLineString, mask uint8, interactions map[XY]struct{}) *doublyConnectedEdgeList {
	var dcel *doublyConnectedEdgeList
	switch g.Type() {
	case TypePolygon:
		poly := g.AsPolygon()
		dcel = newDCELFromMultiPolygon(poly.AsMultiPolygon(), mask, interactions)
	case TypeMultiPolygon:
		mp := g.AsMultiPolygon()
		dcel = newDCELFromMultiPolygon(mp, mask, interactions)
	case TypeLineString:
		mls := g.AsLineString().AsMultiLineString()
		dcel = newDCELFromMultiLineString(mls, mask, interactions)
	case TypeMultiLineString:
		mls := g.AsMultiLineString()
		dcel = newDCELFromMultiLineString(mls, mask, interactions)
	case TypePoint:
		mp := NewMultiPointFromPoints([]Point{g.AsPoint()})
		dcel = newDCELFromMultiPoint(mp, mask)
	case TypeMultiPoint:
		mp := g.AsMultiPoint()
		dcel = newDCELFromMultiPoint(mp, mask)
	case TypeGeometryCollection:
		panic("geometry collection not supported")
	default:
		panic(fmt.Sprintf("unknown geometry type: %v", g.Type()))
	}

	dcel.addGhosts(ghosts, mask, interactions)
	return dcel
}

func newDCELFromMultiPolygon(mp MultiPolygon, mask uint8, interactions map[XY]struct{}) *doublyConnectedEdgeList {
	mp = mp.ForceCCW()

	dcel := &doublyConnectedEdgeList{vertices: make(map[XY]*vertexRecord)}

	for polyIdx := 0; polyIdx < mp.NumPolygons(); polyIdx++ {
		poly := mp.PolygonN(polyIdx)

		// Extract rings.
		rings := make([]Sequence, 1+poly.NumInteriorRings())
		rings[0] = poly.ExteriorRing().Coordinates()
		for i := 0; i < poly.NumInteriorRings(); i++ {
			rings[i+1] = poly.InteriorRingN(i).Coordinates()
		}

		// Populate vertices.
		for _, ring := range rings {
			for i := 0; i < ring.Length(); i++ {
				xy := ring.GetXY(i)
				if _, ok := interactions[xy]; !ok {
					continue
				}
				if _, ok := dcel.vertices[xy]; !ok {
					dcel.vertices[xy] = &vertexRecord{xy, nil /* populated later */, mask, mask & locBoundary}
				}
			}
		}

		for _, ring := range rings {
			var newEdges []*halfEdgeRecord
			forEachNonInteractingSegment(ring, interactions, func(segment []XY) {
				// Construct the internal points slices.
				intermediateFwd := segment[1 : len(segment)-1]
				intermediateRev := reverseXYs(intermediateFwd)

				// Build the edges (fwd and rev).
				vertA := dcel.vertices[segment[0]]
				vertB := dcel.vertices[segment[len(segment)-1]]
				internalEdge := &halfEdgeRecord{
					origin:       vertA,
					twin:         nil, // populated later
					incident:     nil, // only populated in the overlay
					next:         nil, // populated later
					prev:         nil, // populated later
					intermediate: intermediateFwd,
					edgeLabel:    mask,
					faceLabel:    mask,
				}
				externalEdge := &halfEdgeRecord{
					origin:       vertB,
					twin:         internalEdge,
					incident:     nil, // only populated in the overlay
					next:         nil, // populated later
					prev:         nil, // populated later
					intermediate: intermediateRev,
					edgeLabel:    mask,
					faceLabel:    mask & populatedMask,
				}
				internalEdge.twin = externalEdge
				vertA.incidents = append(vertA.incidents, internalEdge)
				vertB.incidents = append(vertB.incidents, externalEdge)
				newEdges = append(newEdges, internalEdge, externalEdge)
			})

			// Link together next/prev pointers.
			numEdges := len(newEdges)
			for i := 0; i < numEdges/2; i++ {
				newEdges[i*2+0].next = newEdges[(2*i+2)%numEdges]
				newEdges[i*2+1].next = newEdges[(2*i-1+numEdges)%numEdges]
				newEdges[i*2+0].prev = newEdges[(2*i-2+numEdges)%numEdges]
				newEdges[i*2+1].prev = newEdges[(2*i+3)%numEdges]
			}
			dcel.halfEdges = append(dcel.halfEdges, newEdges...)
		}
	}
	return dcel
}

func newDCELFromMultiLineString(mls MultiLineString, mask uint8, interactions map[XY]struct{}) *doublyConnectedEdgeList {
	dcel := &doublyConnectedEdgeList{
		vertices: make(map[XY]*vertexRecord),
	}

	// Add vertices.
	for i := 0; i < mls.NumLineStrings(); i++ {
		ls := mls.LineStringN(i)
		seq := ls.Coordinates()
		n := seq.Length()
		for j := 0; j < n; j++ {
			xy := seq.GetXY(j)
			if _, ok := interactions[xy]; !ok {
				continue
			}
			loc := locInterior
			if (j == 0 || j == n-1) && !ls.IsClosed() {
				loc = locBoundary
			}
			if v, ok := dcel.vertices[xy]; ok {
				// TODO: We need to flip between interior and boundary using the mod-2 rule.
				v.locLabel |= mask & loc
			} else {
				dcel.vertices[xy] = &vertexRecord{xy, nil /* populated later */, mask, mask & loc}
			}
		}
	}

	edges := make(edgeSet)

	// Add edges.
	for i := 0; i < mls.NumLineStrings(); i++ {
		seq := mls.LineStringN(i).Coordinates()
		forEachNonInteractingSegment(seq, interactions, func(segment []XY) {
			startXY := segment[0]
			endXY := segment[len(segment)-1]

			intermediateFwd := segment[1 : len(segment)-1]
			intermediateRev := reverseXYs(intermediateFwd)

			if edges.containsStartIntermediateEnd(startXY, intermediateFwd, endXY) {
				return
			}
			edges.insertStartIntermediateEnd(startXY, intermediateFwd, endXY)
			edges.insertStartIntermediateEnd(endXY, intermediateRev, startXY)

			vOrigin := dcel.vertices[startXY]
			vDestin := dcel.vertices[endXY]

			fwd := &halfEdgeRecord{
				origin:       vOrigin,
				twin:         nil, // set later
				incident:     nil, // only populated in overlay
				next:         nil, // set later
				prev:         nil, // set later
				intermediate: intermediateFwd,
				edgeLabel:    mask,
				faceLabel:    mask & populatedMask,
			}
			rev := &halfEdgeRecord{
				origin:       vDestin,
				twin:         fwd,
				incident:     nil, // only populated in overlay
				next:         fwd,
				prev:         fwd,
				intermediate: intermediateRev,
				edgeLabel:    mask,
				faceLabel:    mask & populatedMask,
			}
			fwd.twin = rev
			fwd.next = rev
			fwd.prev = rev

			vOrigin.incidents = append(vOrigin.incidents, fwd)
			vDestin.incidents = append(vDestin.incidents, rev)

			dcel.halfEdges = append(dcel.halfEdges, fwd, rev)
		})
	}
	return dcel
}

func newDCELFromMultiPoint(mp MultiPoint, mask uint8) *doublyConnectedEdgeList {
	dcel := &doublyConnectedEdgeList{vertices: make(map[XY]*vertexRecord)}
	n := mp.NumPoints()
	for i := 0; i < n; i++ {
		xy, ok := mp.PointN(i).XY()
		if !ok {
			continue
		}
		record, ok := dcel.vertices[xy]
		if !ok {
			record = &vertexRecord{
				coords:    xy,
				incidents: nil,

				// TODO: why not just set label here and remove label
				// adjustment below? The current way seems a bit weird...
				label: 0,

				locLabel: mask & locInterior,
			}
			dcel.vertices[xy] = record
		}
		record.label |= mask
	}
	return dcel
}

func (d *doublyConnectedEdgeList) addGhosts(mls MultiLineString, mask uint8, interactions map[XY]struct{}) {
	edges := make(edgeSet)
	for _, e := range d.halfEdges {
		edges.insertEdge(e)
	}

	for i := 0; i < mls.NumLineStrings(); i++ {
		seq := mls.LineStringN(i).Coordinates()
		forEachNonInteractingSegment(seq, interactions, func(segment []XY) {
			startXY := segment[0]
			endXY := segment[len(segment)-1]
			intermediateFwd := segment[1 : len(segment)-1]
			intermediateRev := reverseXYs(intermediateFwd)

			if _, ok := d.vertices[startXY]; !ok {
				d.vertices[startXY] = &vertexRecord{coords: startXY, incidents: nil, label: 0}
			}
			if _, ok := d.vertices[endXY]; !ok {
				d.vertices[endXY] = &vertexRecord{coords: endXY, incidents: nil, label: 0}
			}

			if edges.containsStartIntermediateEnd(startXY, intermediateFwd, endXY) {
				// Already exists, so shouldn't add.
				return
			}
			edges.insertStartIntermediateEnd(startXY, intermediateFwd, endXY)
			edges.insertStartIntermediateEnd(endXY, intermediateRev, startXY)

			d.addGhostLine(startXY, intermediateFwd, intermediateRev, endXY, mask)
		})
	}
}

func (d *doublyConnectedEdgeList) addGhostLine(startXY XY, intermediateFwd, intermediateRev []XY, endXY XY, mask uint8) {
	vertA := d.vertices[startXY]
	vertB := d.vertices[endXY]

	e1 := &halfEdgeRecord{
		origin:       vertA,
		twin:         nil, // populated later
		incident:     nil, // only populated in the overlay
		next:         nil, // popluated later
		prev:         nil, // populated later
		intermediate: intermediateFwd,
		edgeLabel:    mask & populatedMask,
		faceLabel:    0,
	}
	e2 := &halfEdgeRecord{
		origin:       vertB,
		twin:         e1,
		incident:     nil, // only populated in the overlay
		next:         e1,
		prev:         e1,
		intermediate: intermediateRev,
		edgeLabel:    mask & populatedMask,
		faceLabel:    0,
	}
	e1.twin = e2
	e1.next = e2
	e1.prev = e2

	vertA.incidents = append(vertA.incidents, e1)
	vertB.incidents = append(vertB.incidents, e2)

	d.halfEdges = append(d.halfEdges, e1, e2)

	d.fixVertex(vertA)
	d.fixVertex(vertB)
}

func forEachNonInteractingSegment(seq Sequence, interactions map[XY]struct{}, fn func([]XY)) {
	n := seq.Length()
	i := 0
	for i < n-1 {
		// Find the next interaction point after i. This will be the
		// end of the next non-interacting segment.
		start := i
		var end int
		for j := i + 1; j < n; j++ {
			if _, ok := interactions[seq.GetXY(j)]; ok {
				end = j
				break
			}
		}

		// Construct the segment.
		segment := make([]XY, end-start+1)
		for j := range segment {
			segment[j] = seq.GetXY(start + j)
		}

		// Execute the callback with the segment.
		fn(segment)

		// On the next iteration, start the next edge at the end of
		// this one.
		i = end
	}
}
