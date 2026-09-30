package world

import ()

type StatsLogger struct{}

func (s *StatsLogger) LogEvent(name string) {}

func (s *StatsLogger) LogMaxValue(name string, v float64) {}
