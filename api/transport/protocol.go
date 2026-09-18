// Package transport defines the versioned JSON contract between manual Search and Collector commands.
package transport

type Point struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}
type Request struct {
	Date    string  `json:"date,omitempty"`
	Adults  int     `json:"adults,omitempty"`
	Limit   int     `json:"limit,omitempty"`
	Version int     `json:"version"`
	Method  string  `json:"method"`
	Code    string  `json:"code,omitempty"`
	UID     string  `json:"uid,omitempty"`
	From    string  `json:"from,omitempty"`
	To      string  `json:"to,omitempty"`
	System  string  `json:"system,omitempty"`
	Mode    string  `json:"mode,omitempty"`
	Center  *Point  `json:"center,omitempty"`
	Radius  float64 `json:"radius,omitempty"`
	Offset  int     `json:"offset,omitempty"`
}
type Station struct {
	Code  string `json:"code"`
	Title string `json:"title"`
	IATA  string `json:"iata,omitempty"`
}
type Thread struct {
	UID    string `json:"uid"`
	Number string `json:"number"`
}
type Connection struct {
	From   Station `json:"from"`
	To     Station `json:"to"`
	Mode   string  `json:"mode"`
	Number string  `json:"number"`
}
type FlightPath struct {
	Legs []Connection `json:"legs"`
}
type Response struct {
	FlightPaths []FlightPath `json:"flight_paths,omitempty"`
	Version     int          `json:"version"`
	Error       string       `json:"error,omitempty"`
	Total       int          `json:"total"`
	Count       int          `json:"count"`
	Incomplete  bool         `json:"incomplete"`
	Stations    []Station    `json:"stations,omitempty"`
	Threads     []Thread     `json:"threads,omitempty"`
	Connections []Connection `json:"connections,omitempty"`
}
