package cdp

type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Height float64 `json:"height"`
	Width  float64 `json:"width"`
}

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type BoxData struct {
	Point  Point
	Point2 Point
	Center Point
	Width  float64
	Height float64
}
