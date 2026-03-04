package gamemap

import (
	"errors"
	"fmt"
	"sort"

	"prison-break/internal/shared/model"
)

var (
	ErrRoomNotFound    = errors.New("map: room not found")
	ErrNoTilePathFound = errors.New("map: no tile path found")
)

const (
	RoomCorridorMain model.RoomID = "corridor_main"
	RoomWardenHQ     model.RoomID = "warden_hq"
	RoomCameraRoom   model.RoomID = "camera_room"
	RoomPowerRoom    model.RoomID = "power_room"
	RoomAmmoRoom     model.RoomID = "ammunition_room"
	RoomMailRoom     model.RoomID = "mail_room"
	RoomCellBlockA   model.RoomID = "cell_block_a"
	RoomCafeteria    model.RoomID = "cafeteria"
	RoomCourtyard    model.RoomID = "courtyard"
	RoomBlackMarket  model.RoomID = "black_market"
	RoomRoofLookout  model.RoomID = "roof_lookout"
)

type TileKind uint8

const (
	TileWall TileKind = iota
	TileFloor
	TileDoor
)

type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type Tile struct {
	Kind   TileKind     `json:"kind"`
	RoomID model.RoomID `json:"room_id,omitempty"`
}

func (t Tile) Walkable() bool {
	return t.Kind == TileFloor || t.Kind == TileDoor
}

type Room struct {
	ID         model.RoomID `json:"id"`
	Name       string       `json:"name"`
	Min        Point        `json:"min"`
	Max        Point        `json:"max"`
	IsCorridor bool         `json:"is_corridor"`
}

func (r Room) Contains(point Point) bool {
	return point.X >= r.Min.X &&
		point.X <= r.Max.X &&
		point.Y >= r.Min.Y &&
		point.Y <= r.Max.Y
}

type RestrictedZone struct {
	ID     model.ZoneID `json:"id"`
	Name   string       `json:"name"`
	RoomID model.RoomID `json:"room_id"`
}

type DoorLink struct {
	ID       model.DoorID `json:"id"`
	RoomA    model.RoomID `json:"room_a"`
	RoomB    model.RoomID `json:"room_b"`
	Position Point        `json:"position"`
	Locked   bool         `json:"locked"`
}

type Cell struct {
	ID     model.CellID `json:"id"`
	RoomID model.RoomID `json:"room_id"`
	DoorID model.DoorID `json:"door_id"`
}

type RoomAccessCheck struct {
	FromRoom         model.RoomID   `json:"from_room"`
	ToRoom           model.RoomID   `json:"to_room"`
	Reachable        bool           `json:"reachable"`
	TargetRestricted bool           `json:"target_restricted"`
	RoomPath         []model.RoomID `json:"room_path,omitempty"`
}

type Layout struct {
	width  int
	height int

	tiles []Tile

	rooms         map[model.RoomID]Room
	roomOrder     []model.RoomID
	corridorRooms map[model.RoomID]struct{}

	doors       []DoorLink
	doorByPoint map[Point]DoorLink
	cells       []Cell

	restrictedZones  []RestrictedZone
	restrictedByRoom map[model.RoomID]RestrictedZone

	roomGraph map[model.RoomID][]model.RoomID

	blackMarketRoom model.RoomID
}

func DefaultPrisonLayout() Layout {
	// Expanded from 38x22 to 76x44 (4x total area) while preserving room graph and door IDs.
	builder := newLayoutBuilder(76, 44)

	builder.carveRoom(RoomCorridorMain, "Main Corridor", 1, 19, 74, 24, true)

	builder.carveRoom(RoomWardenHQ, "Warden HQ", 1, 2, 13, 17, false)
	builder.carveRoom(RoomCameraRoom, "Camera Room", 16, 2, 28, 17, false)
	builder.carveRoom(RoomPowerRoom, "Power Room", 31, 2, 43, 17, false)
	builder.carveRoom(RoomAmmoRoom, "Ammunition Room", 46, 2, 58, 17, false)
	builder.carveRoom(RoomMailRoom, "Mail Room", 61, 2, 73, 17, false)

	builder.carveRoom(RoomCellBlockA, "Cell Block A", 1, 26, 13, 41, false)
	builder.carveRoom(RoomCafeteria, "Cafeteria", 16, 26, 28, 41, false)
	builder.carveRoom(RoomCourtyard, "Courtyard", 31, 26, 43, 41, false)
	builder.carveRoom(RoomBlackMarket, "Black Market", 46, 26, 58, 41, false)
	builder.carveRoom(RoomRoofLookout, "Roof Lookout", 61, 26, 73, 41, false)

	builder.addDoor(1, RoomWardenHQ, RoomCorridorMain, Point{X: 7, Y: 18}, false)
	builder.addDoor(2, RoomCameraRoom, RoomCorridorMain, Point{X: 22, Y: 18}, false)
	builder.addDoor(3, RoomPowerRoom, RoomCorridorMain, Point{X: 37, Y: 18}, false)
	builder.addDoor(4, RoomAmmoRoom, RoomCorridorMain, Point{X: 52, Y: 18}, false)
	builder.addDoor(5, RoomMailRoom, RoomCorridorMain, Point{X: 67, Y: 18}, false)

	builder.addDoor(6, RoomCellBlockA, RoomCorridorMain, Point{X: 7, Y: 25}, false)
	builder.addDoor(7, RoomCafeteria, RoomCorridorMain, Point{X: 22, Y: 25}, false)
	builder.addDoor(8, RoomCourtyard, RoomCorridorMain, Point{X: 37, Y: 25}, false)
	builder.addDoor(9, RoomBlackMarket, RoomCorridorMain, Point{X: 52, Y: 25}, false)
	builder.addDoor(10, RoomRoofLookout, RoomCorridorMain, Point{X: 67, Y: 25}, false)

	builder.addRestrictedZone(1, RoomWardenHQ, "Warden HQ")
	builder.addRestrictedZone(2, RoomCameraRoom, "Camera Room")
	builder.addRestrictedZone(3, RoomAmmoRoom, "Ammunition Room")
	builder.addRestrictedZone(4, RoomPowerRoom, "Power Room")

	builder.addCell(1, RoomCellBlockA, 201)
	builder.addCell(2, RoomCellBlockA, 202)
	builder.addCell(3, RoomCellBlockA, 203)
	builder.addCell(4, RoomCellBlockA, 204)
	builder.addCell(5, RoomCellBlockA, 205)
	builder.addCell(6, RoomCellBlockA, 206)
	builder.addCell(7, RoomCellBlockA, 207)
	builder.addCell(8, RoomCellBlockA, 208)
	builder.addCell(9, RoomCellBlockA, 209)
	builder.addCell(10, RoomCellBlockA, 210)
	builder.addCell(11, RoomCellBlockA, 211)
	builder.addCell(12, RoomCellBlockA, 212)

	builder.blackMarketRoom = RoomBlackMarket

	return builder.build()
}

func (l Layout) Width() int {
	return l.width
}

func (l Layout) Height() int {
	return l.height
}

func (l Layout) BlackMarketRoomID() model.RoomID {
	return l.blackMarketRoom
}

func (l Layout) InBounds(point Point) bool {
	return point.X >= 0 && point.Y >= 0 && point.X < l.width && point.Y < l.height
}

func (l Layout) TileAt(point Point) (Tile, bool) {
	index, ok := l.index(point)
	if !ok {
		return Tile{}, false
	}
	return l.tiles[index], true
}

func (l Layout) IsWalkable(point Point) bool {
	tile, exists := l.TileAt(point)
	if !exists {
		return false
	}
	return tile.Walkable()
}

func (l Layout) RoomAt(point Point) (model.RoomID, bool) {
	tile, exists := l.TileAt(point)
	if !exists || tile.RoomID == "" {
		return "", false
	}
	return tile.RoomID, true
}

func (l Layout) HasRoom(roomID model.RoomID) bool {
	_, exists := l.rooms[roomID]
	return exists
}

func (l Layout) Room(roomID model.RoomID) (Room, bool) {
	room, exists := l.rooms[roomID]
	return room, exists
}

func (l Layout) Rooms() []Room {
	out := make([]Room, 0, len(l.roomOrder))
	for _, roomID := range l.roomOrder {
		out = append(out, l.rooms[roomID])
	}
	return out
}

func (l Layout) IsCorridorRoom(roomID model.RoomID) bool {
	_, exists := l.corridorRooms[roomID]
	return exists
}

func (l Layout) DoorLinks() []DoorLink {
	out := make([]DoorLink, len(l.doors))
	copy(out, l.doors)
	return out
}

func (l Layout) DoorLinkAt(point Point) (DoorLink, bool) {
	door, exists := l.doorByPoint[point]
	return door, exists
}

func (l Layout) Cells() []Cell {
	out := make([]Cell, len(l.cells))
	copy(out, l.cells)
	return out
}

func (l Layout) RestrictedZones() []RestrictedZone {
	out := make([]RestrictedZone, len(l.restrictedZones))
	copy(out, l.restrictedZones)
	return out
}

func (l Layout) IsRoomRestricted(roomID model.RoomID) bool {
	_, exists := l.restrictedByRoom[roomID]
	return exists
}

func (l Layout) FindRoomPath(fromRoom model.RoomID, toRoom model.RoomID) ([]model.RoomID, bool) {
	if !l.HasRoom(fromRoom) || !l.HasRoom(toRoom) {
		return nil, false
	}
	if fromRoom == toRoom {
		return []model.RoomID{fromRoom}, true
	}

	parent := make(map[model.RoomID]model.RoomID, len(l.roomGraph))
	visited := make(map[model.RoomID]struct{}, len(l.roomGraph))
	queue := []model.RoomID{fromRoom}
	visited[fromRoom] = struct{}{}

	found := false
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		neighbors := l.roomGraph[current]
		for _, neighbor := range neighbors {
			if _, seen := visited[neighbor]; seen {
				continue
			}
			visited[neighbor] = struct{}{}
			parent[neighbor] = current

			if neighbor == toRoom {
				found = true
				queue = nil
				break
			}

			queue = append(queue, neighbor)
		}
	}

	if !found {
		return nil, false
	}

	path := []model.RoomID{toRoom}
	for current := toRoom; current != fromRoom; {
		current = parent[current]
		path = append(path, current)
	}
	reverseRoomPath(path)
	return path, true
}

func (l Layout) AreRoomsConnected(fromRoom model.RoomID, toRoom model.RoomID) bool {
	_, connected := l.FindRoomPath(fromRoom, toRoom)
	return connected
}

func (l Layout) CheckRoomAccess(fromRoom model.RoomID, toRoom model.RoomID) (RoomAccessCheck, error) {
	if !l.HasRoom(fromRoom) || !l.HasRoom(toRoom) {
		return RoomAccessCheck{}, ErrRoomNotFound
	}

	roomPath, reachable := l.FindRoomPath(fromRoom, toRoom)
	return RoomAccessCheck{
		FromRoom:         fromRoom,
		ToRoom:           toRoom,
		Reachable:        reachable,
		TargetRestricted: l.IsRoomRestricted(toRoom),
		RoomPath:         roomPath,
	}, nil
}

func (l Layout) FindPath(start Point, end Point) ([]Point, error) {
	startIndex, ok := l.index(start)
	if !ok || !l.tiles[startIndex].Walkable() {
		return nil, ErrNoTilePathFound
	}
	endIndex, ok := l.index(end)
	if !ok || !l.tiles[endIndex].Walkable() {
		return nil, ErrNoTilePathFound
	}

	if startIndex == endIndex {
		return []Point{start}, nil
	}

	queue := []int{startIndex}
	visited := make(map[int]struct{}, len(l.tiles)/2)
	parent := make(map[int]int, len(l.tiles)/2)
	visited[startIndex] = struct{}{}

	directions := []Point{
		{X: 0, Y: -1},
		{X: -1, Y: 0},
		{X: 1, Y: 0},
		{X: 0, Y: 1},
	}

	found := false
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		currentPoint := l.pointForIndex(current)

		for _, direction := range directions {
			nextPoint := Point{
				X: currentPoint.X + direction.X,
				Y: currentPoint.Y + direction.Y,
			}
			nextIndex, inBounds := l.index(nextPoint)
			if !inBounds {
				continue
			}
			if !l.tiles[nextIndex].Walkable() {
				continue
			}
			if _, seen := visited[nextIndex]; seen {
				continue
			}

			visited[nextIndex] = struct{}{}
			parent[nextIndex] = current

			if nextIndex == endIndex {
				found = true
				queue = nil
				break
			}
			queue = append(queue, nextIndex)
		}
	}

	if !found {
		return nil, ErrNoTilePathFound
	}

	path := []Point{end}
	for current := endIndex; current != startIndex; {
		current = parent[current]
		path = append(path, l.pointForIndex(current))
	}
	reversePointPath(path)
	return path, nil
}

func (l Layout) ToMapState() model.MapState {
	doors := make([]model.DoorState, 0, len(l.doors))
	for _, door := range l.doors {
		doors = append(doors, model.DoorState{
			ID:       door.ID,
			RoomA:    door.RoomA,
			RoomB:    door.RoomB,
			Open:     true,
			Locked:   door.Locked,
			CanClose: true,
		})
	}

	for _, cell := range l.cells {
		doors = append(doors, model.DoorState{
			ID:       cell.DoorID,
			RoomA:    RoomCellBlockA,
			RoomB:    cellInteriorRoomID(cell.ID),
			Open:     false,
			Locked:   false,
			CanClose: true,
		})
	}

	sort.Slice(doors, func(i int, j int) bool {
		return doors[i].ID < doors[j].ID
	})

	cells := make([]model.CellState, 0, len(l.cells))
	for _, cell := range l.cells {
		cells = append(cells, model.CellState{
			ID:     cell.ID,
			DoorID: cell.DoorID,
		})
	}
	sort.Slice(cells, func(i int, j int) bool {
		return cells[i].ID < cells[j].ID
	})

	zones := make([]model.ZoneState, 0, len(l.restrictedZones))
	for _, zone := range l.restrictedZones {
		zones = append(zones, model.ZoneState{
			ID:         zone.ID,
			RoomID:     zone.RoomID,
			Restricted: true,
			Name:       zone.Name,
		})
	}
	sort.Slice(zones, func(i int, j int) bool {
		return zones[i].ID < zones[j].ID
	})

	return model.MapState{
		PowerOn: true,
		Alarm: model.AlarmState{
			Active: false,
		},
		BlackMarketRoomID: l.blackMarketRoom,
		Doors:             doors,
		Cells:             cells,
		RestrictedZones:   zones,
	}
}

func cellInteriorRoomID(cellID model.CellID) model.RoomID {
	return model.RoomID(fmt.Sprintf("cell_a_%03d", cellID))
}

func (l Layout) index(point Point) (int, bool) {
	if !l.InBounds(point) {
		return 0, false
	}
	return point.Y*l.width + point.X, true
}

func (l Layout) pointForIndex(index int) Point {
	return Point{
		X: index % l.width,
		Y: index / l.width,
	}
}

func reverseRoomPath(path []model.RoomID) {
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
}

func reversePointPath(path []Point) {
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
}

type layoutBuilder struct {
	width  int
	height int

	tiles []Tile

	rooms         map[model.RoomID]Room
	corridorRooms map[model.RoomID]struct{}

	doors []DoorLink
	cells []Cell

	restrictedByID   map[model.ZoneID]RestrictedZone
	restrictedByRoom map[model.RoomID]RestrictedZone

	roomGraph map[model.RoomID]map[model.RoomID]struct{}

	blackMarketRoom model.RoomID
}

func newLayoutBuilder(width int, height int) *layoutBuilder {
	tiles := make([]Tile, width*height)
	for index := range tiles {
		tiles[index] = Tile{Kind: TileWall}
	}

	return &layoutBuilder{
		width:            width,
		height:           height,
		tiles:            tiles,
		rooms:            make(map[model.RoomID]Room),
		corridorRooms:    make(map[model.RoomID]struct{}),
		restrictedByID:   make(map[model.ZoneID]RestrictedZone),
		restrictedByRoom: make(map[model.RoomID]RestrictedZone),
		roomGraph:        make(map[model.RoomID]map[model.RoomID]struct{}),
	}
}

func (b *layoutBuilder) carveRoom(
	roomID model.RoomID,
	name string,
	minX int,
	minY int,
	maxX int,
	maxY int,
	isCorridor bool,
) {
	room := Room{
		ID:         roomID,
		Name:       name,
		Min:        Point{X: minX, Y: minY},
		Max:        Point{X: maxX, Y: maxY},
		IsCorridor: isCorridor,
	}
	b.rooms[roomID] = room
	if isCorridor {
		b.corridorRooms[roomID] = struct{}{}
	}
	if _, exists := b.roomGraph[roomID]; !exists {
		b.roomGraph[roomID] = make(map[model.RoomID]struct{})
	}

	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			index := y*b.width + x
			b.tiles[index] = Tile{
				Kind:   TileFloor,
				RoomID: roomID,
			}
		}
	}
}

func (b *layoutBuilder) addDoor(
	doorID model.DoorID,
	roomA model.RoomID,
	roomB model.RoomID,
	position Point,
	locked bool,
) {
	assignedRoomID := roomA
	if _, isCorridor := b.corridorRooms[roomB]; isCorridor {
		assignedRoomID = roomB
	} else if _, isCorridor := b.corridorRooms[roomA]; isCorridor {
		assignedRoomID = roomA
	}

	index := position.Y*b.width + position.X
	b.tiles[index] = Tile{
		Kind:   TileDoor,
		RoomID: assignedRoomID,
	}

	b.doors = append(b.doors, DoorLink{
		ID:       doorID,
		RoomA:    roomA,
		RoomB:    roomB,
		Position: position,
		Locked:   locked,
	})

	if _, exists := b.roomGraph[roomA]; !exists {
		b.roomGraph[roomA] = make(map[model.RoomID]struct{})
	}
	if _, exists := b.roomGraph[roomB]; !exists {
		b.roomGraph[roomB] = make(map[model.RoomID]struct{})
	}

	b.roomGraph[roomA][roomB] = struct{}{}
	b.roomGraph[roomB][roomA] = struct{}{}
}

func (b *layoutBuilder) addRestrictedZone(zoneID model.ZoneID, roomID model.RoomID, name string) {
	zone := RestrictedZone{
		ID:     zoneID,
		Name:   name,
		RoomID: roomID,
	}
	b.restrictedByID[zoneID] = zone
	b.restrictedByRoom[roomID] = zone
}

func (b *layoutBuilder) addCell(cellID model.CellID, roomID model.RoomID, doorID model.DoorID) {
	b.cells = append(b.cells, Cell{
		ID:     cellID,
		RoomID: roomID,
		DoorID: doorID,
	})
}

func (b *layoutBuilder) build() Layout {
	roomOrder := make([]model.RoomID, 0, len(b.rooms))
	for roomID := range b.rooms {
		roomOrder = append(roomOrder, roomID)
	}
	sort.Slice(roomOrder, func(i int, j int) bool {
		return roomOrder[i] < roomOrder[j]
	})

	doors := make([]DoorLink, len(b.doors))
	copy(doors, b.doors)
	sort.Slice(doors, func(i int, j int) bool {
		return doors[i].ID < doors[j].ID
	})
	doorByPoint := make(map[Point]DoorLink, len(doors))
	for _, door := range doors {
		doorByPoint[door.Position] = door
	}

	cells := make([]Cell, len(b.cells))
	copy(cells, b.cells)
	sort.Slice(cells, func(i int, j int) bool {
		return cells[i].ID < cells[j].ID
	})

	restrictedIDs := make([]model.ZoneID, 0, len(b.restrictedByID))
	for zoneID := range b.restrictedByID {
		restrictedIDs = append(restrictedIDs, zoneID)
	}
	sort.Slice(restrictedIDs, func(i int, j int) bool {
		return restrictedIDs[i] < restrictedIDs[j]
	})

	restrictedZones := make([]RestrictedZone, 0, len(restrictedIDs))
	for _, zoneID := range restrictedIDs {
		restrictedZones = append(restrictedZones, b.restrictedByID[zoneID])
	}

	roomGraph := make(map[model.RoomID][]model.RoomID, len(b.roomGraph))
	for roomID, neighbors := range b.roomGraph {
		list := make([]model.RoomID, 0, len(neighbors))
		for neighbor := range neighbors {
			list = append(list, neighbor)
		}
		sort.Slice(list, func(i int, j int) bool {
			return list[i] < list[j]
		})
		roomGraph[roomID] = list
	}

	tiles := make([]Tile, len(b.tiles))
	copy(tiles, b.tiles)

	rooms := make(map[model.RoomID]Room, len(b.rooms))
	for roomID, room := range b.rooms {
		rooms[roomID] = room
	}

	corridorRooms := make(map[model.RoomID]struct{}, len(b.corridorRooms))
	for roomID := range b.corridorRooms {
		corridorRooms[roomID] = struct{}{}
	}

	restrictedByRoom := make(map[model.RoomID]RestrictedZone, len(b.restrictedByRoom))
	for roomID, zone := range b.restrictedByRoom {
		restrictedByRoom[roomID] = zone
	}

	return Layout{
		width:            b.width,
		height:           b.height,
		tiles:            tiles,
		rooms:            rooms,
		roomOrder:        roomOrder,
		corridorRooms:    corridorRooms,
		doors:            doors,
		doorByPoint:      doorByPoint,
		cells:            cells,
		restrictedZones:  restrictedZones,
		restrictedByRoom: restrictedByRoom,
		roomGraph:        roomGraph,
		blackMarketRoom:  b.blackMarketRoom,
	}
}
