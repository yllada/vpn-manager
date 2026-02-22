// Package ui provides the graphical user interface for VPN Manager.
// This file contains icon generation utilities for the system tray.
package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// IconConfig defines the configuration for icon generation.
type IconConfig struct {
	Size          int
	FillColor     color.RGBA
	BorderColor   color.RGBA
	AccentColor   color.RGBA
	SymbolColor   color.RGBA
	ShowCheckmark bool
}

// DefaultConnectedIconConfig returns the default config for connected state.
func DefaultConnectedIconConfig() IconConfig {
	return IconConfig{
		Size:          22,
		FillColor:     color.RGBA{56, 142, 60, 255},   // Dark green
		BorderColor:   color.RGBA{76, 175, 80, 255},   // Green
		AccentColor:   color.RGBA{200, 230, 201, 255}, // Light green
		SymbolColor:   color.RGBA{255, 255, 255, 255}, // White
		ShowCheckmark: true,
	}
}

// DefaultDisconnectedIconConfig returns the default config for disconnected state.
func DefaultDisconnectedIconConfig() IconConfig {
	return IconConfig{
		Size:          22,
		FillColor:     color.RGBA{117, 117, 117, 255}, // Dark gray
		BorderColor:   color.RGBA{158, 158, 158, 255}, // Gray
		AccentColor:   color.RGBA{189, 189, 189, 255}, // Light gray
		SymbolColor:   color.RGBA{255, 255, 255, 255}, // White
		ShowCheckmark: false,
	}
}

// IconGenerator generates PNG icons for the system tray.
type IconGenerator struct {
	config IconConfig
}

// NewIconGenerator creates a new icon generator with the given config.
func NewIconGenerator(config IconConfig) *IconGenerator {
	return &IconGenerator{config: config}
}

// Generate creates a PNG icon and returns the bytes.
func (g *IconGenerator) Generate() []byte {
	size := g.config.Size
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Draw shield
	g.drawShield(img)

	// Draw symbol (checkmark or lock)
	if g.config.ShowCheckmark {
		g.drawCheckmark(img)
	} else {
		g.drawLock(img)
	}

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

// drawShield draws the shield shape on the image.
func (g *IconGenerator) drawShield(img *image.RGBA) {
	size := g.config.Size
	centerX := float64(size) / 2
	topY := 1.0
	bottomY := float64(size) - 2
	shieldWidth := float64(size) - 4

	isInShield := func(x, y float64) bool {
		relY := (y - topY) / (bottomY - topY)
		if relY < 0 || relY > 1 {
			return false
		}

		var halfWidth float64
		if relY < 0.5 {
			halfWidth = shieldWidth/2 - relY*0.5
		} else {
			progress := (relY - 0.5) * 2
			halfWidth = (shieldWidth/2 - 0.25) * (1 - progress*progress)
		}

		return x >= centerX-halfWidth && x <= centerX+halfWidth
	}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5

			if isInShield(fx, fy) {
				isBorder := !isInShield(fx-1, fy) || !isInShield(fx+1, fy) ||
					!isInShield(fx, fy-1) || !isInShield(fx, fy+1)

				if isBorder {
					img.Set(x, y, g.config.BorderColor)
				} else {
					relY := float64(y) / float64(size)
					if relY < 0.3 {
						img.Set(x, y, g.config.AccentColor)
					} else {
						img.Set(x, y, g.config.FillColor)
					}
				}
			}
		}
	}
}

// drawCheckmark draws a checkmark symbol on the image.
func (g *IconGenerator) drawCheckmark(img *image.RGBA) {
	// Checkmark points
	points := []struct{ x, y int }{
		{6, 11}, {7, 11}, {7, 12}, {8, 12}, {8, 13}, {9, 13},
		{9, 12}, {10, 12}, {10, 11}, {11, 11}, {11, 10}, {12, 10},
		{12, 9}, {13, 9}, {13, 8}, {14, 8},
	}
	for _, p := range points {
		if p.x >= 0 && p.x < g.config.Size && p.y >= 0 && p.y < g.config.Size {
			img.Set(p.x, p.y, g.config.SymbolColor)
		}
	}
}

// drawLock draws a lock symbol on the image.
func (g *IconGenerator) drawLock(img *image.RGBA) {
	c := g.config.SymbolColor

	// Lock body
	for y := 10; y <= 15; y++ {
		for x := 8; x <= 14; x++ {
			if y == 10 || y == 15 || x == 8 || x == 14 {
				img.Set(x, y, c)
			}
		}
	}

	// Lock shackle
	for y := 6; y <= 10; y++ {
		if y <= 8 {
			img.Set(9, y, c)
			img.Set(13, y, c)
		}
		if y == 6 {
			for x := 9; x <= 13; x++ {
				img.Set(x, y, c)
			}
		}
	}
}

// GenerateConnectedIcon generates the connected state icon.
func GenerateConnectedIcon() []byte {
	gen := NewIconGenerator(DefaultConnectedIconConfig())
	return gen.Generate()
}

// GenerateDisconnectedIcon generates the disconnected state icon.
func GenerateDisconnectedIcon() []byte {
	gen := NewIconGenerator(DefaultDisconnectedIconConfig())
	return gen.Generate()
}
