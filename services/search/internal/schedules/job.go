package schedules

import "time"

type Job struct {
	RequestID        string
	Token            string
	SourceFinishedAt time.Time
	Payload          []byte
	Graph            []byte
}
