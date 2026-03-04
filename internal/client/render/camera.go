package render

import (
	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

const (
	defaultTilePixels = 28.0
	defaultCameraZoom = 1.0
)

type Camera struct {
	CenterX float64
	CenterY float64

	TilePixels float64
	Zoom       float64

	ViewportWidth  int
	ViewportHeight int
}

func NewCamera(viewportWidth int, viewportHeight int) Camera {
	return Camera{
		CenterX:        0,
		CenterY:        0,
		TilePixels:     defaultTilePixels,
		Zoom:           defaultCameraZoom,
		ViewportWidth:  viewportWidth,
		ViewportHeight: viewportHeight,
	}
}

func (c *Camera) FocusOn(position model.Vector2) {
	if c == nil {
		return
	}
	c.CenterX = float64(position.X)
	c.CenterY = float64(position.Y)
}

func (c *Camera) ClampToLayout(layout gamemap.Layout) {
	if c == nil || c.TilePixels <= 0 || c.Zoom <= 0 {
		return
	}

	viewHalfWidthTiles := (float64(c.ViewportWidth) / (c.TilePixels * c.Zoom)) / 2
	viewHalfHeightTiles := (float64(c.ViewportHeight) / (c.TilePixels * c.Zoom)) / 2

	mapWidthTiles := float64(layout.Width())
	mapHeightTiles := float64(layout.Height())

	minCenterX := viewHalfWidthTiles
	maxCenterX := mapWidthTiles - viewHalfWidthTiles
	minCenterY := viewHalfHeightTiles
	maxCenterY := mapHeightTiles - viewHalfHeightTiles

	if minCenterX > maxCenterX {
		c.CenterX = mapWidthTiles / 2
	} else {
		if c.CenterX < minCenterX {
			c.CenterX = minCenterX
		}
		if c.CenterX > maxCenterX {
			c.CenterX = maxCenterX
		}
	}

	if minCenterY > maxCenterY {
		c.CenterY = mapHeightTiles / 2
	} else {
		if c.CenterY < minCenterY {
			c.CenterY = minCenterY
		}
		if c.CenterY > maxCenterY {
			c.CenterY = maxCenterY
		}
	}
}

func (c Camera) WorldToScreen(position model.Vector2) (float64, float64) {
	scale := c.TilePixels * c.Zoom
	x := (float64(position.X)-c.CenterX)*scale + (float64(c.ViewportWidth) / 2)
	y := (float64(position.Y)-c.CenterY)*scale + (float64(c.ViewportHeight) / 2)
	return x, y
}

func (c Camera) ScreenToWorld(screenX int, screenY int) (float32, float32) {
	scale := c.TilePixels * c.Zoom
	if scale <= 0 {
		return 0, 0
	}

	worldX := ((float64(screenX) - (float64(c.ViewportWidth) / 2)) / scale) + c.CenterX
	worldY := ((float64(screenY) - (float64(c.ViewportHeight) / 2)) / scale) + c.CenterY
	return float32(worldX), float32(worldY)
}

func (c Camera) TileRectToScreen(x float64, y float64, widthTiles float64, heightTiles float64) (float64, float64, float64, float64) {
	topLeftX := (x-c.CenterX)*(c.TilePixels*c.Zoom) + (float64(c.ViewportWidth) / 2)
	topLeftY := (y-c.CenterY)*(c.TilePixels*c.Zoom) + (float64(c.ViewportHeight) / 2)
	return topLeftX, topLeftY, widthTiles * c.TilePixels * c.Zoom, heightTiles * c.TilePixels * c.Zoom
}
