package main

type Coord struct{ X, Y int64 }

func (c Coord) Add(o Coord) Coord {
	return Coord{c.X + o.X, c.Y + o.Y}
}

func (c Coord) Sub(o Coord) Coord {
	return Coord{c.X - o.X, c.Y - o.Y}
}

func (c Coord) Hull() (min, max Coord) {
	// avoid rounding off odd coordinates
	max = Coord{c.X / 2, 0}
	min = Coord{max.X - c.X, max.Y - c.Y}
	return
}

func (c Coord) Floor(i int64) Coord {
	x := (c.X%i + i) % i
	y := (c.Y%i + i) % i
	return Coord{c.X - x, c.Y - y}
}

func (c Coord) Zero() bool {
	return c == Coord{}
}

func (c Coord) LengthSquared() int64 {
	return c.X*c.X + c.Y*c.Y
}

func (c Coord) Unit() Coord {
	var x, y int64
	if c.X < 0 {
		x = -1
	} else if c.X > 0 {
		x = 1
	}
	if c.Y < 0 {
		y = -1
	} else if c.Y > 0 {
		y = 1
	}
	return Coord{x, y}
}
