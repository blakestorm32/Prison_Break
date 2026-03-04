package render

import (
	"math"
	"testing"

	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

func TestCameraWorldToScreenCentersFocusedPosition(t *testing.T) {
	camera := NewCamera(800, 600)
	camera.TilePixels = 20
	camera.Zoom = 1
	camera.FocusOn(model.Vector2{X: 10, Y: 8})

	x, y := camera.WorldToScreen(model.Vector2{X: 10, Y: 8})
	if x != 400 || y != 300 {
		t.Fatalf("expected focused world position to map to screen center, got x=%.2f y=%.2f", x, y)
	}
}

func TestCameraScreenToWorldInvertsWorldToScreen(t *testing.T) {
	camera := NewCamera(1024, 768)
	camera.TilePixels = 32
	camera.Zoom = 1.25
	camera.FocusOn(model.Vector2{X: 15.5, Y: 9.75})

	world := model.Vector2{X: 18.2, Y: 7.4}
	screenX, screenY := camera.WorldToScreen(world)
	convertedX, convertedY := camera.ScreenToWorld(int(screenX), int(screenY))

	if math.Abs(float64(convertedX-world.X)) > 0.05 || math.Abs(float64(convertedY-world.Y)) > 0.05 {
		t.Fatalf("expected screen->world conversion to approximate original world point, got (%f,%f) want (%f,%f)", convertedX, convertedY, world.X, world.Y)
	}
}

func TestCameraClampToLayoutBounds(t *testing.T) {
	layout := gamemap.DefaultPrisonLayout()
	camera := NewCamera(960, 640)
	camera.TilePixels = 24
	camera.Zoom = 1

	camera.CenterX = -100
	camera.CenterY = -50
	camera.ClampToLayout(layout)
	if camera.CenterX < 0 || camera.CenterY < 0 {
		t.Fatalf("expected camera clamp to keep center in positive bounds, got x=%.2f y=%.2f", camera.CenterX, camera.CenterY)
	}

	camera.CenterX = 9999
	camera.CenterY = 9999
	camera.ClampToLayout(layout)
	if camera.CenterX > float64(layout.Width()) || camera.CenterY > float64(layout.Height()) {
		t.Fatalf("expected camera clamp to keep center within map bounds, got x=%.2f y=%.2f", camera.CenterX, camera.CenterY)
	}
}
