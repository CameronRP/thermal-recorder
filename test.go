package main

import "log"

var activeArea [24][32]bool

func main() {
	makeActiveArea()
}

type Point struct {
	x float32
	y float32
}

var p1 = Point{
	x: 5,
	y: 5,
}

var p2 = Point{
	x: 24,
	y: 5,
}

var p3 = Point{
	x: 8,
	y: 20,
}

var p4 = Point{
	x: 21,
	y: 20,
}

func makeActiveArea() {
	for y, row := range activeArea {
		for x := range row {
			activeArea[y][x] = pointInTriangle2(p1, p2, p3, float32(x), float32(y)) || pointInTriangle2(p2, p3, p4, float32(x), float32(y))
		}
	}

	for _, row := range activeArea {
		line := ""
		for _, val := range row {
			if val {
				line = line + "###"
			} else {
				line = line + "___"
			}
		}
		log.Println(line)
	}
}

func pointInTriangle2(p1, p2, p3 Point, x, y float32) bool {
	a := ((p2.y-p3.y)*(x-p3.x) + (p3.x-p2.x)*(y-p3.y)) / ((p2.y-p3.y)*(p1.x-p3.x) + (p3.x-p2.x)*(p1.y-p3.y))
	b := ((p3.y-p1.y)*(x-p3.x) + (p1.x-p3.x)*(y-p3.y)) / ((p2.y-p3.y)*(p1.x-p3.x) + (p3.x-p2.x)*(p1.y-p3.y))
	c := 1 - a - b
	return 0 <= a && a <= 1 && 0 <= b && b <= 1 && 0 <= c && c <= 1
}
