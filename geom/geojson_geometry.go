package geom

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

func UnmarshalGeoJSON(input []byte, opts ...ConstructorOption) (GeometryX, error) {
	var firstPass struct {
		Type string `json:"type"`
	}
	if err := json.NewDecoder(bytes.NewReader(input)).Decode(&firstPass); err != nil {
		return nil, err
	}

	switch firstPass.Type {
	case "Point":
		var secondPass struct {
			Coords []float64 `json:"coordinates"`
		}
		if err := json.NewDecoder(bytes.NewReader(input)).Decode(&secondPass); err != nil {
			return nil, err
		}
		if len(secondPass.Coords) == 0 {
			return NewEmptyPoint(opts...), nil
		}
		coords, err := oneDimFloat64sToCoordinates(secondPass.Coords)
		if err != nil {
			return nil, err
		}
		return NewPointC(coords, opts...), nil
	case "LineString", "MultiPoint":
		var secondPass struct {
			Coords [][]float64 `json:"coordinates"`
		}
		if err := json.NewDecoder(bytes.NewReader(input)).Decode(&secondPass); err != nil {
			return nil, err
		}
		coords, err := twoDimFloat64sToCoordinates(secondPass.Coords)
		if err != nil {
			return nil, err
		}
		switch firstPass.Type {
		case "LineString":
			switch len(coords) {
			case 0:
				return NewEmptyLineString(opts...), nil
			case 2:
				return NewLineC(coords[0], coords[1], opts...)
			default:
				return NewLineStringC(coords, opts...)
			}
		case "MultiPoint":
			return NewMultiPointC(coords, opts...), nil
		default:
			panic("switch case bug")
		}
	case "Polygon", "MultiLineString":
		var secondPass struct {
			Coords [][][]float64 `json:"coordinates"`
		}
		if err := json.NewDecoder(bytes.NewReader(input)).Decode(&secondPass); err != nil {
			return nil, err
		}
		coords, err := threeDimFloat64sToCoordinates(secondPass.Coords)
		if err != nil {
			return nil, err
		}
		switch firstPass.Type {
		case "Polygon":
			switch len(coords) {
			case 0:
				return NewEmptyPolygon(opts...), nil
			default:
				return NewPolygonC(coords, opts...)
			}
		case "MultiLineString":
			return NewMultiLineStringC(coords, opts...)
		default:
			panic("switch case bug")
		}
	case "MultiPolygon":
		var secondPass struct {
			Coords [][][][]float64 `json:"coordinates"`
		}
		if err := json.NewDecoder(bytes.NewReader(input)).Decode(&secondPass); err != nil {
			return nil, err
		}
		coords, err := fourDimFloat64sToCoordinates(secondPass.Coords)
		if err != nil {
			return nil, err
		}
		return NewMultiPolygonC(coords, opts...)
	case "GeometryCollection":
		var secondPass struct {
			Geometries []AnyGeometry `json:"geometries"`
		}
		if err := json.NewDecoder(bytes.NewReader(input)).Decode(&secondPass); err != nil {
			return nil, err
		}
		geoms := make([]GeometryX, len(secondPass.Geometries))
		for i := range geoms {
			geoms[i] = secondPass.Geometries[i].Geom
		}
		return NewGeometryCollection(geoms, opts...), nil
	case "":
		return nil, errors.New("type field missing or empty")
	default:
		return nil, fmt.Errorf("unknown geojson type: %s", firstPass.Type)
	}
}

func oneDimFloat64sToCoordinates(fs []float64) (Coordinates, error) {
	if len(fs) < 2 || len(fs) > 4 {
		return Coordinates{}, fmt.Errorf("coordinates have incorrect dimension: %d", len(fs))
	}
	for _, f := range fs {
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return Coordinates{}, errors.New("coordinate is NaN or inf")
		}
	}
	return Coordinates{XY{fs[0], fs[1]}}, nil
}

func twoDimFloat64sToCoordinates(outer [][]float64) ([]Coordinates, error) {
	var coords []Coordinates
	for _, inner := range outer {
		cs, err := oneDimFloat64sToCoordinates(inner)
		if err != nil {
			return nil, err
		}
		coords = append(coords, cs)
	}
	return coords, nil
}

func threeDimFloat64sToCoordinates(outer [][][]float64) ([][]Coordinates, error) {
	var coords [][]Coordinates
	for _, inner := range outer {
		cs, err := twoDimFloat64sToCoordinates(inner)
		if err != nil {
			return nil, err
		}
		coords = append(coords, cs)
	}
	return coords, nil
}

func fourDimFloat64sToCoordinates(outer [][][][]float64) ([][][]Coordinates, error) {
	var coords [][][]Coordinates
	for _, inner := range outer {
		cs, err := threeDimFloat64sToCoordinates(inner)
		if err != nil {
			return nil, err
		}
		coords = append(coords, cs)
	}
	return coords, nil
}

func marshalGeoJSON(geomType string, coordinates interface{}) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(`{"type":"`)
	buf.WriteString(geomType)
	buf.WriteString(`","coordinates":`)
	coordJSON, err := json.Marshal(coordinates)
	if err != nil {
		return nil, err
	}
	buf.Write(coordJSON)
	buf.WriteRune('}')
	return buf.Bytes(), nil
}
